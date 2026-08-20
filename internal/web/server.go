package web

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strconv"
	"sync"

	"github.com/smiden/synccal/internal/config"
	"github.com/smiden/synccal/internal/storage"
	syncpkg "github.com/smiden/synccal/internal/sync"
	"go.uber.org/zap"
)

//go:embed assets/*
var assetsFS embed.FS

// Syncer is the subset of sync.Syncer consumed by the web server.
type Syncer interface {
	Sync(ctx context.Context) error
	IsRunning() bool
	Status() syncpkg.Status
	StartScheduler()
	Stop()
}

// SyncerHolder keeps a mutable reference to the active syncer so it can be
// replaced after a config update without rebuilding the HTTP stack.
type SyncerHolder struct {
	mu sync.RWMutex
	s  Syncer
}

func NewSyncerHolder(s Syncer) *SyncerHolder {
	return &SyncerHolder{s: s}
}

func (h *SyncerHolder) Get() Syncer {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.s
}

func (h *SyncerHolder) Set(s Syncer) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.s = s
}

type Server struct {
	cfgPath string
	cfgMu   sync.RWMutex
	cfg     *config.Config
	store   *storage.Store
	logs    *LogStore
	log     *zap.SugaredLogger
	holder  *SyncerHolder
	rebuild func(*config.Config) (Syncer, error)
}

func New(cfg *config.Config, cfgPath string, store *storage.Store, logs *LogStore, holder *SyncerHolder, rebuild func(*config.Config) (Syncer, error), log *zap.SugaredLogger) *Server {
	return &Server{
		cfgPath: cfgPath,
		cfg:     cfg,
		store:   store,
		logs:    logs,
		log:     log,
		holder:  holder,
		rebuild: rebuild,
	}
}

// Handler returns the full HTTP handler for the web UI and REST API. The
// static assets are served publicly so the SPA can prompt for the token; only
// the /api/* endpoints require authentication.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api/status", s.auth(http.HandlerFunc(s.handleStatus)))
	mux.Handle("/api/config", s.auth(http.HandlerFunc(s.handleConfig)))
	mux.Handle("/api/events", s.auth(http.HandlerFunc(s.handleEvents)))
	mux.Handle("/api/logs", s.auth(http.HandlerFunc(s.handleLogs)))
	mux.Handle("/api/sync", s.auth(http.HandlerFunc(s.handleSync)))
	mux.Handle("/", s.assetHandler())
	return mux
}

// IsRunning reports whether a sync is currently in progress (used by healthz).
func (s *Server) IsRunning() bool {
	return s.holder.Get().IsRunning()
}

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.cfgMu.RLock()
		token := s.cfg.Web.Token
		s.cfgMu.RUnlock()

		if token == "" {
			next.ServeHTTP(w, r)
			return
		}

		if r.Header.Get("X-API-Key") == token || r.Header.Get("Authorization") == "Bearer "+token {
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Set("WWW-Authenticate", `Bearer realm="synccal"`)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	})
}

// ---------------------------------------------------------------------------
// API handlers
// ---------------------------------------------------------------------------

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	writeJSON(w, http.StatusOK, s.holder.Get().Status())
}

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if s.holder.Get().IsRunning() {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "sync already running"})
		return
	}

	s.cfgMu.RLock()
	timeout := s.cfg.Sync.TimeoutDuration()
	s.cfgMu.RUnlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if err := s.holder.Get().Sync(ctx); err != nil {
			s.log.Errorw("Manual sync failed", "error", err)
		}
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	q := r.URL.Query()
	dest := q.Get("destination")
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	events, err := s.store.ListEvents(dest, limit, offset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"events": events})
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	q := r.URL.Query()
	level := q.Get("level")
	limit, _ := strconv.Atoi(q.Get("limit"))

	entries := s.logs.List(level, limit)
	writeJSON(w, http.StatusOK, map[string]interface{}{"logs": entries})
}

// ---------------------------------------------------------------------------
// Config handler (GET = sanitized view, PUT = apply + rebuild)
// ---------------------------------------------------------------------------

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.configView())
	case http.MethodPut:
		s.applyConfig(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (s *Server) applyConfig(w http.ResponseWriter, r *http.Request) {
	var upd configUpdate
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&upd); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	if s.holder.Get().IsRunning() {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "cannot update config while sync is running"})
		return
	}

	s.cfgMu.Lock()
	newCfg := s.cfg
	mergeConfigUpdate(newCfg, &upd)
	if err := config.Validate(newCfg); err != nil {
		s.cfgMu.Unlock()
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.cfgMu.Unlock()

	data, err := marshalConfig(newCfg)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to serialize config"})
		return
	}
	if err := writeConfigFile(s.cfgPath, data); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to write config file"})
		return
	}

	newSyncer, err := s.rebuild(newCfg)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("failed to apply config: %v", err)})
		return
	}

	old := s.holder.Get()
	old.Stop()
	s.holder.Set(newSyncer)
	if newCfg.Sync.IntervalDuration() > 0 {
		newSyncer.StartScheduler()
	}

	s.cfgMu.Lock()
	s.cfg = newCfg
	s.cfgMu.Unlock()

	s.log.Infow("Config updated via web UI", "connections", len(newCfg.Sources))
	writeJSON(w, http.StatusOK, s.configView())
}

// ---------------------------------------------------------------------------
// Static assets
// ---------------------------------------------------------------------------

func (s *Server) assetHandler() http.Handler {
	sub, err := fs.Sub(assetsFS, "assets")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			s.serveIndex(w)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

func (s *Server) serveIndex(w http.ResponseWriter) {
	data, err := assetsFS.ReadFile("assets/index.html")
	if err != nil {
		http.Error(w, "index not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method != method {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		_ = err
	}
}
