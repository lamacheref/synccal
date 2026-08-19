package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/smiden/synccal/internal/config"
	"github.com/smiden/synccal/internal/storage"
	"github.com/smiden/synccal/internal/sync"
	"go.uber.org/zap"
)

type Server struct {
	cfg    *config.Config
	store  *storage.Store
	syncer *sync.Syncer
	log    *zap.SugaredLogger
}

func NewServer(cfg *config.Config, store *storage.Store, syncer *sync.Syncer, log *zap.SugaredLogger) *Server {
	return &Server{cfg: cfg, store: store, syncer: syncer, log: log}
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/sync", s.handleSync)
	mux.HandleFunc("/api/sync/trigger", s.handleTriggerSync)
	mux.HandleFunc("/api/events", s.handleEvents)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, http.StatusOK, map[string]string{"status": "ok", "time": time.Now().Format(time.RFC3339)})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	// Get basic stats from storage
	stats := map[string]interface{}{
		"source": map[string]string{
			"url": s.cfg.Source.URL,
		},
		"destinations": make([]map[string]string, len(s.cfg.Destinations)),
		"scheduler_running": s.syncer != nil,
		"sync_interval":     s.cfg.Sync.Interval,
	}

	for i, d := range s.cfg.Destinations {
		stats["destinations"].([]map[string]string)[i] = map[string]string{
			"name": d.Name,
			"url":  d.URL,
		}
	}

	jsonResponse(w, http.StatusOK, stats)
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		// Return config without passwords
		type safeConfig struct {
			Source struct {
				URL      string `json:"url"`
				HasAuth  bool   `json:"has_auth"`
			} `json:"source"`
			Destinations []struct {
				Name     string `json:"name"`
				URL      string `json:"url"`
				Username string `json:"username"`
				HasAuth  bool   `json:"has_auth"`
			} `json:"destinations"`
			Sync struct {
				Interval   string `json:"interval"`
				Timeout    string `json:"timeout"`
				BatchSize  int    `json:"batch_size"`
				DeleteMode string `json:"delete_mode"`
			} `json:"sync"`
			Metrics struct {
				Addr string `json:"addr"`
			} `json:"metrics"`
		}

		resp := safeConfig{}
		resp.Source.URL = s.cfg.Source.URL
		resp.Source.HasAuth = s.cfg.Source.Username != "" || s.cfg.Source.Password != ""
		resp.Sync.Interval = s.cfg.Sync.Interval
		resp.Sync.Timeout = s.cfg.Sync.Timeout
		resp.Sync.BatchSize = s.cfg.Sync.BatchSize
		resp.Sync.DeleteMode = s.cfg.Sync.DeleteMode
		resp.Metrics.Addr = s.cfg.Metrics.Addr

		for _, d := range s.cfg.Destinations {
			resp.Destinations = append(resp.Destinations, struct {
				Name     string `json:"name"`
				URL      string `json:"url"`
				Username string `json:"username"`
				HasAuth  bool   `json:"has_auth"`
			}{
				Name:     d.Name,
				URL:      d.URL,
				Username: d.Username,
				HasAuth:  d.Password != "",
			})
		}

		jsonResponse(w, http.StatusOK, resp)
		return
	}

	// POST - update config (would need file write, omitted for safety)
	jsonResponse(w, http.StatusMethodNotAllowed, map[string]string{"error": "Config update not implemented"})
}

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	// Return last sync status per destination
	type SyncStatus struct {
		Destination string `json:"destination"`
		LastSync    string `json:"last_sync,omitempty"`
		EventCount  int    `json:"event_count"`
		Error       string `json:"error,omitempty"`
	}

	statuses := []SyncStatus{}
	for _, d := range s.cfg.Destinations {
		mappings, _ := s.store.ListMappings(d.Name)
		statuses = append(statuses, SyncStatus{
			Destination: d.Name,
			EventCount:  len(mappings),
		})
	}

	jsonResponse(w, http.StatusOK, statuses)
}

func (s *Server) handleTriggerSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonResponse(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
		return
	}

	if s.syncer == nil {
		jsonResponse(w, http.StatusServiceUnavailable, map[string]string{"error": "Syncer not initialized"})
		return
	}

	go func() {
		ctx := r.Context()
		if err := s.syncer.Sync(ctx); err != nil {
			s.log.Errorw("Manual sync failed", "error", err)
		}
	}()

	jsonResponse(w, http.StatusAccepted, map[string]string{"status": "sync triggered"})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	destName := r.URL.Query().Get("destination")
	if destName == "" {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"error": "destination query param required"})
		return
	}

	mappings, err := s.store.ListMappings(destName)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	type EventInfo struct {
		SourceUID  string `json:"source_uid"`
		ContentHash string `json:"content_hash"`
	}

	events := make([]EventInfo, 0, len(mappings))
	for uid, hash := range mappings {
		events = append(events, EventInfo{SourceUID: uid, ContentHash: hash})
	}

	jsonResponse(w, http.StatusOK, events)
}

func jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}