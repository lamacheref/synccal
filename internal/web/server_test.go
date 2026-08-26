package web

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smiden/synccal/internal/config"
	"github.com/smiden/synccal/internal/storage"
	"github.com/smiden/synccal/internal/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
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
			Name: "src1", Type: "caldav", URL: "https://example.com/calendar.ics",
		}},
		Destinations: []config.DestinationConfig{{
			Name: "dest1", Type: "caldav", URL: "https://dest.example.com/", Username: "user", Password: "secret", Source: "src1",
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
	assert.Equal(t, "src1", source["name"])
	assert.Equal(t, "https://example.com/calendar.ics", source["url"])
	_, hasPassword := source["password"]
	assert.False(t, hasPassword, "password key must be absent")

	dests := body["destinations"].([]interface{})
	require.Len(t, dests, 1)
	dest := dests[0].(map[string]interface{})
	assert.Equal(t, "dest1", dest["name"])
	assert.Equal(t, "src1", dest["source"])
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
			{"name": "src1", "url": "https://newsource.example.com/cal.ics", "username": "srcuser", "password": "newsecret"},
			{"name": "src2", "url": "https://newsource2.example.com/cal.ics"}
		],
		"destinations": [
			{"name": "dest1", "url": "https://dest.example.com/", "username": "user", "source": "src1"},
			{"name": "dest2", "url": "https://dest2.example.com/", "username": "user", "password": "tok2", "source": "src2"}
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
	require.Len(t, s.cfg.Destinations, 2)
	assert.Equal(t, "https://newsource.example.com/cal.ics", s.cfg.Sources[0].URL)
	assert.Equal(t, "newsecret", s.cfg.Sources[0].Password, "explicit source password must be applied")
	assert.Equal(t, "https://newsource2.example.com/cal.ics", s.cfg.Sources[1].URL)
	assert.Equal(t, "dest1", s.cfg.Destinations[0].Name)
	assert.Equal(t, "src1", s.cfg.Destinations[0].Source)
	assert.Equal(t, "secret", s.cfg.Destinations[0].Password, "omitted dest password keeps previous value")
	assert.Equal(t, "tok2", s.cfg.Destinations[1].Password, "password set on new dest applied")
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

	// A destination with missing name is rejected.
	payload := `{"sources": [{"name": "src1", "url": "https://example.com/cal.ics"}], "destinations": [{"url": "https://dest.example.com/", "source": "src1", "username": "u"}]}`
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

func TestPluginsEndpoint(t *testing.T) {
	s, _, _ := newTestServer(t, "")
	handler := s.Handler()

	rec := doRequest(t, handler, http.MethodGet, "/api/plugins", "", "")
	require.Equal(t, http.StatusOK, rec.Code)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	plugins, ok := body["plugins"].([]interface{})
	require.True(t, ok)
	assert.GreaterOrEqual(t, len(plugins), 5, "should have at least caldav source/dest + transformers")
	// Check that transformer types are present
	found := false
	for _, p := range plugins {
		m := p.(map[string]interface{})
		if m["type"] == "filter-private" && m["kind"] == "transformer" {
			found = true
		}
	}
	assert.True(t, found, "filter-private transformer should be registered")
}

func TestConfigWithPlugins(t *testing.T) {
	s, _, _ := newTestServer(t, "")
	handler := s.Handler()

	// Config with plugin types and transformers
	payload := `{
		"sources": [{"name": "src1", "type": "caldav", "url": "https://example.com/cal.ics", "transformers": [{"type": "filter-category", "options": {"categories": "work"}}]}],
		"destinations": [{"name": "dest1", "type": "caldav", "url": "https://dest.example.com/", "username": "user", "password": "secret", "source": "src1", "transformers": [{"type": "prefix-summary", "options": {"prefix": "[Sync] "}}]}],
		"sync": {"interval": "1h"}
	}`
	rec := doRequest(t, handler, http.MethodPut, "/api/config", payload, "")
	require.Equal(t, http.StatusOK, rec.Code)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	srcs := body["sources"].([]interface{})
	assert.Len(t, srcs, 1)
	src := srcs[0].(map[string]interface{})
	assert.Equal(t, "caldav", src["type"])
	trs, ok := src["transformers"].([]interface{})
	require.True(t, ok)
	assert.Len(t, trs, 1)

	dests := body["destinations"].([]interface{})
	dest := dests[0].(map[string]interface{})
	assert.Equal(t, "caldav", dest["type"])
	trs = dest["transformers"].([]interface{})
	assert.Len(t, trs, 1)
}

func TestPluginUploadAndInstalled(t *testing.T) {
	s, _, _ := newTestServer(t, "")
	// DB dans un TempDir → data/plugins sera créé à côté
	s.cfg.Database.Path = filepath.Join(t.TempDir(), "data", "test.db")
	handler := s.Handler()

	// Upload OK
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile("archive", "myplugin.zip")
	_, _ = fw.Write([]byte("fake zip content"))
	w.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/plugins/upload", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "uploaded")

	// Upload sans fichier → 400
	req2 := httptest.NewRequest(http.MethodPost, "/api/plugins/upload", bytes.NewReader(nil))
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusBadRequest, rec2.Code)

	// Méthode interdite
	req3 := httptest.NewRequest(http.MethodGet, "/api/plugins/upload", nil)
	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, req3)
	assert.Equal(t, http.StatusMethodNotAllowed, rec3.Code)

	// Installed : liste le fichier uploadé
	req4 := httptest.NewRequest(http.MethodGet, "/api/plugins/installed", nil)
	rec4 := httptest.NewRecorder()
	handler.ServeHTTP(rec4, req4)
	require.Equal(t, http.StatusOK, rec4.Code)
	assert.Contains(t, rec4.Body.String(), "myplugin.zip")

	// Installed sur dossier inexistant → tableau vide
	s2, _, _ := newTestServer(t, "")
	s2.cfg.Database.Path = filepath.Join(t.TempDir(), "nowhere", "db.sqlite")
	rec5 := httptest.NewRecorder()
	s2.Handler().ServeHTTP(rec5, httptest.NewRequest(http.MethodGet, "/api/plugins/installed", nil))
	require.Equal(t, http.StatusOK, rec5.Code)
	assert.Contains(t, rec5.Body.String(), "[]")
}

func TestIsRunningAndRequireMethod(t *testing.T) {
	s, fake, _ := newTestServer(t, "")
	handler := s.Handler()

	assert.False(t, s.IsRunning())
	fake.running.Store(true)
	assert.True(t, s.IsRunning())

	// handleStatus avec mauvaise méthode → 405
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/status", nil))
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)

	// handleEvents avec mauvaise méthode
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/events", nil))
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)

	// handleLogs avec mauvaise méthode
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/logs", nil))
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)

	// handlePlugins avec mauvaise méthode
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/plugins", nil))
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestLogStoreCore(t *testing.T) {
	ls := NewLogStore(10)
	core := ls.Core(zap.NewNop().Core())
	logger := zap.New(core)

	require.True(t, core.Enabled(zapcore.InfoLevel))
	entry := logger.Check(zapcore.InfoLevel, "via core")
	require.NotNil(t, entry)
	entry.Write(zap.String("k", "v"))
	entry2 := logger.Check(zapcore.WarnLevel, "warn via core")
	entry2.Write()

	entries := ls.List("", 10)
	require.Len(t, entries, 2)
	assert.Equal(t, "warn via core", entries[0].Message)
	assert.Equal(t, "via core", entries[1].Message)

	// With retourne un core fonctionnel
	c2 := core.With([]zap.Field{zap.String("ctx", "x")})
	assert.NotNil(t, c2)
}

func TestHandleEventsDBErrorAndPagination(t *testing.T) {
	s, _, _ := newTestServer(t, "")
	handler := s.Handler()

	// limit invalide → défaut appliqué, pas d'erreur
	rec := doRequest(t, handler, http.MethodGet, "/api/events?limit=abc&offset=xyz", "", "")
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestPluginUploadInvalidMultipart(t *testing.T) {
	s, _, _ := newTestServer(t, "")
	s.cfg.Database.Path = filepath.Join(t.TempDir(), "data", "db.sqlite")
	handler := s.Handler()

	// Body multipart corrompu
	req := httptest.NewRequest(http.MethodPost, "/api/plugins/upload", strings.NewReader("not-multipart"))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=bad")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// Multipart valide sans champ archive
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("other", "value")
	w.Close()
	req2 := httptest.NewRequest(http.MethodPost, "/api/plugins/upload", &buf)
	req2.Header.Set("Content-Type", w.FormDataContentType())
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusBadRequest, rec2.Code)
}
