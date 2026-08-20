package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/smiden/synccal/internal/caldav"
	"github.com/smiden/synccal/internal/config"
	"github.com/smiden/synccal/internal/metrics"
	"github.com/smiden/synccal/internal/storage"
	"github.com/smiden/synccal/internal/sync"
	"github.com/smiden/synccal/internal/web"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to config file")
	flag.Parse()

	logStore := web.NewLogStore(1000)
	logger := newLogger(&config.Config{}) // bootstrap logger, recreated after config load
	defer logger.Sync()
	log := logger.Sugar()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalw("Failed to load config", "error", err)
	}

	logger = newLogger(cfg).WithOptions(zap.WrapCore(func(core zapcore.Core) zapcore.Core {
		return logStore.Core(core)
	}))
	log = logger.Sugar()

	log.Infow("Starting SyncCal",
		"source", cfg.Source.URL,
		"destinations", len(cfg.Destinations),
		"interval", cfg.Sync.Interval,
	)

	store, err := storage.New(cfg.Database.Path)
	if err != nil {
		log.Fatalw("Failed to init storage", "error", err)
	}
	defer store.Close()

	newSyncer := func(c *config.Config) (*sync.Syncer, error) {
		sourceClient, err := caldav.NewClient(c.Source.URL, c.Source.Username, c.Source.Password)
		if err != nil {
			return nil, fmt.Errorf("source client: %w", err)
		}

		destClients := make([]*caldav.Client, len(c.Destinations))
		for i, d := range c.Destinations {
			client, err := caldav.NewClient(d.URL, d.Username, d.Password)
			if err != nil {
				return nil, fmt.Errorf("destination client %q: %w", d.Name, err)
			}
			destClients[i] = client
		}
		return sync.New(c, sourceClient, destClients, store, log), nil
	}

	syncer, err := newSyncer(cfg)
	if err != nil {
		log.Fatalw("Failed to create syncer", "error", err)
	}

	holder := web.NewSyncerHolder(syncer)
	rebuild := func(c *config.Config) (web.Syncer, error) { return newSyncer(c) }
	webServer := web.New(cfg, *configPath, store, logStore, holder, rebuild, log)

	mux := http.NewServeMux()
	mux.Handle("/metrics", metrics.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		if webServer.IsRunning() {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, "sync in progress")
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if webServer.IsRunning() {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, "sync in progress")
			return
		}
		if err := store.Ping(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, "db unavailable")
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ready")
	})
	mux.Handle("/", webServer.Handler())

	server := &http.Server{
		Addr:         cfg.Web.Addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	serverErrCh := make(chan error, 1)
	go func() {
		log.Infow("HTTP server starting", "addr", cfg.Web.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrCh <- err
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Sync.TimeoutDuration())
	if err := syncer.Sync(ctx); err != nil {
		log.Errorw("Initial sync failed", "error", err)
	}
	cancel()

	if cfg.Sync.IntervalDuration() > 0 {
		syncer.StartScheduler()
		log.Infow("Scheduler started", "interval", cfg.Sync.Interval)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		log.Infow("Signal received, initiating graceful shutdown", "signal", sig)
	case err := <-serverErrCh:
		log.Errorw("Server error, initiating shutdown", "error", err)
	}

	log.Info("Shutting down...")
	holder.Get().Stop()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Errorw("HTTP server shutdown error", "error", err)
	} else {
		log.Info("HTTP server stopped")
	}

	waitForSyncCompletion(shutdownCtx, holder, log)

	log.Info("Graceful shutdown complete")
}

func waitForSyncCompletion(ctx context.Context, holder *web.SyncerHolder, log *zap.SugaredLogger) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Warn("Shutdown timeout exceeded, forcing exit")
			return
		case <-ticker.C:
			if !holder.Get().IsRunning() {
				log.Info("Sync completed, safe to exit")
				return
			}
			log.Debug("Waiting for sync to complete...")
		}
	}
}

func newLogger(cfg *config.Config) *zap.Logger {
	zapCfg := zap.NewProductionConfig()
	if cfg.Logging.Format == "console" {
		zapCfg = zap.NewDevelopmentConfig()
		zapCfg.EncoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	}
	zapCfg.EncoderConfig.TimeKey = "timestamp"
	zapCfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	zapCfg.EncoderConfig.LevelKey = "level"
	zapCfg.EncoderConfig.MessageKey = "message"
	zapCfg.DisableStacktrace = true

	level := zapcore.InfoLevel
	if cfg.Logging.Level != "" {
		if lvl, err := zapcore.ParseLevel(cfg.Logging.Level); err == nil {
			level = lvl
		}
	}
	zapCfg.Level = zap.NewAtomicLevelAt(level)

	logger, _ := zapCfg.Build()
	return logger
}
