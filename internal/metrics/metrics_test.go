package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetricsHandler(t *testing.T) {
	// Les Vec n'exposent une métrique qu'après première utilisation d'un enfant
	SyncDuration.WithLabelValues("dest-handler", "success").Observe(0.1)
	EventsSynced.WithLabelValues("dest-handler", "created").Add(1)
	h := Handler()
	require.NotNil(t, h)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "text/plain")
	body := rec.Body.String()
	assert.Contains(t, body, "synccal_sync_duration_seconds")
	assert.Contains(t, body, "synccal_events_synced_total")
}

func TestMetricUpdates(t *testing.T) {
	SyncDuration.WithLabelValues("dest-test", "success").Observe(0.5)
	EventsSynced.WithLabelValues("dest-test", "created").Add(3)
	SyncErrors.WithLabelValues("dest-test", "sync").Inc()
	LastSyncTimestamp.WithLabelValues("dest-test").SetToCurrentTime()
	SourceEvents.WithLabelValues("https://src.example.com/cal.ics").Add(5)
	SourceErrors.WithLabelValues("https://src.example.com/cal.ics", "fetch").Inc()
	SourceSyncDuration.WithLabelValues("https://src.example.com/cal.ics", "success").Observe(0.2)
	SourceLastSyncTimestamp.WithLabelValues("https://src.example.com/cal.ics").SetToCurrentTime()

	assert.Equal(t, 3.0, testutil.ToFloat64(EventsSynced.WithLabelValues("dest-test", "created")))
	assert.Equal(t, 1.0, testutil.ToFloat64(SyncErrors.WithLabelValues("dest-test", "sync")))
	assert.Equal(t, 5.0, testutil.ToFloat64(SourceEvents.WithLabelValues("https://src.example.com/cal.ics")))
	assert.Equal(t, 1.0, testutil.ToFloat64(SourceErrors.WithLabelValues("https://src.example.com/cal.ics", "fetch")))

	// Verify exposition includes labels
	h := Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := rec.Body.String()
	assert.True(t, strings.Contains(body, `destination="dest-test"`), "metrics should expose destination label")
	assert.True(t, strings.Contains(body, `source="https://src.example.com/cal.ics"`), "metrics should expose source label")
}
