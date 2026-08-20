package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smiden/synccal/internal/config"
	"github.com/smiden/synccal/internal/storage"
	"github.com/smiden/synccal/internal/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeSyncer struct {
	running atomic.Bool
	calls   atomic.Int32
}

func (f *fakeSyncer) Sync(ctx context.Context) error {
	f.calls.Add(1)
	return nil
}

func (f *fakeSyncer) IsRunning() bool { return f.running.Load() }

func (f *fakeSyncer) Status() sync.Status {
	return sync.Status{
		Running:  f.running.Load(),
		Interval: "1h",
		Connections: []sync.ConnectionStats{{
			SourceURL:   "https://example.com/calendar.ics",
			Destination: "dest1",
		}},
	}
}

func (f *fakeSyncer) StartScheduler() {}
func (f *fakeSyncer) Stop()           {}

func newTestServer(t *testing.T, token string) (*Server, *fakeSyncer, *storage.Store) {
	t.Helper()

	cfg := &config.Config{
		Sources: []config.SourceConfig{{
			URL:         "https://example.com/calendar.ics",
			Destination: config.DestinationConfig{Name: "dest1", URL: "https://dest.example.com/", Username: "user", Password: "secret"},
		}},
		Database: config.DatabaseConfig{Path: filepath.Join(t.TempDir(), "test.db")},
		Sync: config.SyncConfig{
			Interval: "1h", Timeout: "2m", BatchSize: 100,
			DeleteMode: "soft", FilterPrivate: true,
		},
		Metrics: config.MetricsConfig{Addr: ":0"},
		Web:     config.WebConfig{Addr: ":0", Token: token},
		Logging: config.LoggingConfig{Level: "info", Format: "json"},
	}

	store, err := storage.New(cfg.Database.Path)
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })

	holder := NewSyncerHolder(&fakeSyncer{})
	logStore := NewLogStore(100)
	log := zap.NewNop().Sugar()

	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	rebuild := func(c *config.Config) (Syncer, error) {
		return &fakeSyncer{}, nil
	}

	s := New(cfg, cfgPath, store, logStore, holder, rebuild, log)
	return s, holder.Get().(*fakeSyncer), store
}

func doRequest(t *testing.T, h http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != "" {
		reader = bytes.NewReader([]byte(body))
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAuthRequired(t *testing.T) {
	s, _, _ := newTestServer(t, "s3cret")
	handler := s.Handler()

	rec := doRequest(t, handler, http.MethodGet, "/api/status", "", "")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	// Static assets are public so the SPA can prompt for the token.
	rec = doRequest(t, handler, http.MethodGet, "/", "", "")
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, handler, http.MethodGet, "/api/status", "", "wrong-token")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	rec = doRequest(t, handler, http.MethodGet, "/api/status", "", "s3cret")
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestNoAuthWhenTokenEmpty(t *testing.T) {
	s, _, _ := newTestServer(t, "")
	handler := s.Handler()

	rec := doRequest(t, handler, http.MethodGet, "/api/status", "", "")
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestStatusEndpoint(t *testing.T) {
	s, fake, _ := newTestServer(t, "")
	handler := s.Handler()

	rec := doRequest(t, handler, http.MethodGet, "/api/status", "", "")
	require.Equal(t, http.StatusOK, rec.Code)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "1h", body["interval"])
	assert.Equal(t, false, body["running"])

	conns, ok := body["connections"].([]interface{})
	require.True(t, ok)
	require.Len(t, conns, 1)
	conn := conns[0].(map[string]interface{})
	assert.Equal(t, "https://example.com/calendar.ics", conn["source_url"])
	assert.Equal(t, "dest1", conn["destination"])

	fake.running.Store(true)
	rec = doRequest(t, handler, http.MethodGet, "/api/status", "", "")
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, true, body["running"])
}

func TestConfigGetSanitized(t *testing.T) {
	s, _, _ := newTestServer(t, "")
	handler := s.Handler()

	rec := doRequest(t, handler, http.MethodGet, "/api/config", "", "")
	require.Equal(t, http.StatusOK, rec.Code)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	raw, err := json.Marshal(body)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "secret", "password must never be exposed")

	sources := body["sources"].([]interface{})
	require.Len(t, sources, 1)
	source := sources[0].(map[string]interface{})
	assert.Equal(t, "https://example.com/calendar.ics", source["url"])
	_, hasPassword := source["password"]
	assert.False(t, hasPassword, "password key must be absent")

	dest := source["destination"].(map[string]interface{})
	assert.Equal(t, "dest1", dest["name"])
	_, hasDestPassword := dest["password"]
	assert.False(t, hasDestPassword, "destination password key must be absent")

	web := body["web"].(map[string]interface{})
	assert.Equal(t, false, web["token_set"])
}

func TestConfigPutUpdatesAndRebuilds(t *testing.T) {
	s, fake, _ := newTestServer(t, "")
	handler := s.Handler()

	payload := `{
		"sources": [
			{
				"url": "https://newsource.example.com/cal.ics",
				"username": "srcuser",
				"password": "newsecret",
				"destination": {"name": "dest1", "url": "https://dest.example.com/", "username": "user"}
			},
			{
				"url": "https://newsource2.example.com/cal.ics",
				"destination": {"name": "dest2", "url": "https://dest2.example.com/", "username": "user", "password": "tok2"}
			}
		],
		"sync": {"interval": "30m", "timeout": "3m", "batch_size": 50, "delete_mode": "hard", "filter_private": false},
		"web": {"addr": ":9090"},
		"logging": {"level": "debug", "format": "console"}
	}`

	oldCalls := fake.calls.Load()
	rec := doRequest(t, handler, http.MethodPut, "/api/config", payload, "")
	require.Equal(t, http.StatusOK, rec.Code)

	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	require.Len(t, s.cfg.Sources, 2)
	assert.Equal(t, "https://newsource.example.com/cal.ics", s.cfg.Sources[0].URL)
	assert.Equal(t, "newsecret", s.cfg.Sources[0].Password, "explicit source password must be applied")
	assert.Equal(t, "https://newsource2.example.com/cal.ics", s.cfg.Sources[1].URL)
	assert.Equal(t, "dest1", s.cfg.Sources[0].Destination.Name)
	assert.Equal(t, "secret", s.cfg.Sources[0].Destination.Password, "omitted dest password keeps previous value")
	assert.Equal(t, "tok2", s.cfg.Sources[1].Destination.Password, "password set on new dest applied")
	assert.Equal(t, "30m", s.cfg.Sync.Interval)
	assert.Equal(t, false, s.cfg.Sync.FilterPrivate)
	assert.Equal(t, "debug", s.cfg.Logging.Level)

	// Config file must be persisted
	data, err := os.ReadFile(s.cfgPath)
	require.NoError(t, err)
	require.Contains(t, string(data), "newsource.example.com")

	// Rebuild must have been triggered by calling the rebuild closure
	assert.Equal(t, oldCalls, fake.calls.Load(), "fake syncer should be replaced, not mutated")
}

func TestConfigPutValidationFails(t *testing.T) {
	s, _, _ := newTestServer(t, "")
	handler := s.Handler()

	// A source with an invalid destination (missing name) is rejected.
	payload := `{"sources": [{"url": "https://example.com/cal.ics", "destination": {}}]}`
	rec := doRequest(t, handler, http.MethodPut, "/api/config", payload, "")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestConfigPutRejectedWhileRunning(t *testing.T) {
	s, fake, _ := newTestServer(t, "")
	fake.running.Store(true)
	handler := s.Handler()

	rec := doRequest(t, handler, http.MethodPut, "/api/config", `{"sync": {"interval": "30m"}}`, "")
	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestEventsEndpoint(t *testing.T) {
	s, _, store := newTestServer(t, "")
	handler := s.Handler()

	require.NoError(t, store.SetMapping("uid-1", "dest1", "uid-1", "/dest1/uid-1.ics", "etag1", "hash1", false))
	require.NoError(t, store.SetMapping("uid-2", "dest1", "uid-2", "/dest1/uid-2.ics", "etag2", "hash2", true))
	require.NoError(t, store.SetMapping("uid-3", "dest2", "uid-3", "/dest2/uid-3.ics", "etag3", "hash3", false))

	rec := doRequest(t, handler, http.MethodGet, "/api/events?destination=dest1", "", "")
	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Events []storage.EventRecord `json:"events"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Events, 2)
	assert.Equal(t, "uid-1", body.Events[0].SourceUID)
	assert.Equal(t, "hash1", body.Events[0].ContentHash)

	// No filter returns everything
	rec = doRequest(t, handler, http.MethodGet, "/api/events", "", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Len(t, body.Events, 3)
}

func TestLogsEndpoint(t *testing.T) {
	s, _, _ := newTestServer(t, "")
	handler := s.Handler()

	s.logs.Append(time.Now().Add(-time.Minute), "info", "first", map[string]interface{}{"k": "v"})
	s.logs.Append(time.Now(), "error", "second", map[string]interface{}{"k2": "v2"})

	rec := doRequest(t, handler, http.MethodGet, "/api/logs?level=info&limit=10", "", "")
	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Logs []LogEntry `json:"logs"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Logs, 2)
	assert.Equal(t, "second", body.Logs[0].Message)
	assert.Equal(t, "first", body.Logs[1].Message)

	// Filter by level drops info
	rec = doRequest(t, handler, http.MethodGet, "/api/logs?level=error&limit=10", "", "")
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Logs, 1)
	assert.Equal(t, "second", body.Logs[0].Message)
}

func TestSyncEndpoint(t *testing.T) {
	s, fake, _ := newTestServer(t, "")
	handler := s.Handler()

	rec := doRequest(t, handler, http.MethodPost, "/api/sync", "", "")
	require.Equal(t, http.StatusAccepted, rec.Code)
	assert.Eventually(t, func() bool { return fake.calls.Load() == 1 }, time.Second, 10*time.Millisecond, "sync should be triggered")

	// Second sync while running is rejected
	fake.running.Store(true)
	rec = doRequest(t, handler, http.MethodPost, "/api/sync", "", "")
	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestIndexPageSmoke(t *testing.T) {
	s, _, _ := newTestServer(t, "")
	handler := s.Handler()

	rec := doRequest(t, handler, http.MethodGet, "/", "", "")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "<title>SyncCal</title>")
	assert.Contains(t, rec.Body.String(), "Synchroniser maintenant")

	rec = doRequest(t, handler, http.MethodGet, "/app.js", "", "")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "loadDashboard")

	rec = doRequest(t, handler, http.MethodGet, "/style.css", "", "")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "--primary")
}

func TestIndexPagePublicWithoutTokenLeak(t *testing.T) {
	s, _, _ := newTestServer(t, "tok123")
	handler := s.Handler()

	// The index page must load without auth and never embed the access token.
	rec := doRequest(t, handler, http.MethodGet, "/", "", "")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Authentification requise")
	assert.NotContains(t, rec.Body.String(), "tok123", "access token must never be embedded in the page")
	assert.NotContains(t, rec.Body.String(), "__SYNCCAL_TOKEN__")
}
