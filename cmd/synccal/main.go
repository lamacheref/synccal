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
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to config file")
	flag.Parse()

	logger := newLogger()
	defer logger.Sync()
	log := logger.Sugar()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalw("Failed to load config", "error", err)
	}

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

	sourceClient, err := caldav.NewClient(cfg.Source.URL, cfg.Source.Username, cfg.Source.Password)
	if err != nil {
		log.Fatalw("Failed to create source client", "error", err)
	}

	destClients := make([]*caldav.Client, len(cfg.Destinations))
	for i, d := range cfg.Destinations {
		client, err := caldav.NewClient(d.URL, d.Username, d.Password)
		if err != nil {
			log.Fatalw("Failed to create destination client", "dest", d.Name, "error", err)
		}
		destClients[i] = client
	}

	syncer := sync.New(cfg, sourceClient, destClients, store, log)

	mux := http.NewServeMux()
	mux.Handle("/metrics", metrics.Handler())
	mux.HandleFunc("/healthz", healthHandler(syncer))
	mux.HandleFunc("/readyz", readyHandler(syncer, store))

	go func() {
		log.Infow("Metrics/Health server starting", "addr", cfg.Metrics.Addr)
		if err := http.ListenAndServe(cfg.Metrics.Addr, mux); err != nil {
			log.Errorw("Metrics server failed", "error", err)
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
	<-sigCh

	log.Info("Shutting down...")
	syncer.Stop()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	select {
	case <-shutdownCtx.Done():
		log.Warn("Shutdown timeout exceeded")
	case <-time.After(100 * time.Millisecond):
		log.Info("Graceful shutdown complete")
	}
}

func newLogger() *zap.Logger {
	cfg := zap.NewProductionConfig()
	cfg.EncoderConfig.TimeKey = "timestamp"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.EncoderConfig.LevelKey = "level"
	cfg.EncoderConfig.MessageKey = "message"
	cfg.DisableStacktrace = true
	logger, _ := cfg.Build()
	return logger
}

func healthHandler(syncer *sync.Syncer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if syncer.IsRunning() {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, "sync in progress")
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	}
}

func readyHandler(syncer *sync.Syncer, store *storage.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if syncer.IsRunning() {
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
	}
}