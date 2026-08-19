package main

import (
	"context"
	"flag"
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
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to config file")
	flag.Parse()

	logger, _ := zap.NewProduction()
	defer logger.Sync()
	log := logger.Sugar()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalw("Failed to load config", "error", err)
	}

	// Initialize storage
	store, err := storage.New(cfg.Database.Path)
	if err != nil {
		log.Fatalw("Failed to init storage", "error", err)
	}
	defer store.Close()

	// Initialize CalDAV clients
	sourceClient := caldav.NewClient(cfg.Source.URL, cfg.Source.Username, cfg.Source.Password)
	destClients := make([]*caldav.Client, len(cfg.Destinations))
	for i, d := range cfg.Destinations {
		destClients[i] = caldav.NewClient(d.URL, d.Username, d.Password)
	}

	// Initialize syncer
	syncer := sync.New(cfg, sourceClient, destClients, store, log)

	// Start metrics server
	go func() {
		http.Handle("/metrics", metrics.Handler())
		http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		})
		http.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ready"))
		})
		log.Infow("Metrics server starting", "addr", cfg.Metrics.Addr)
		if err := http.ListenAndServe(cfg.Metrics.Addr, nil); err != nil {
			log.Errorw("Metrics server failed", "error", err)
		}
	}()

	// Run initial sync
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	if err := syncer.Sync(ctx); err != nil {
		log.Errorw("Initial sync failed", "error", err)
	}
	cancel()

	// Schedule periodic sync
	if cfg.Sync.Interval > 0 {
		syncer.StartScheduler()
		log.Infow("Scheduler started", "interval", cfg.Sync.Interval)
	}

	// Wait for shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Info("Shutting down...")
	syncer.Stop()
}