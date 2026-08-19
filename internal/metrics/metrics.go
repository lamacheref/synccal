package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	SyncDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "synccal_sync_duration_seconds",
			Help:    "Duration of sync operations",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"destination", "status"},
	)

	EventsSynced = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "synccal_events_synced_total",
			Help: "Total number of events synced",
		},
		[]string{"destination", "operation"},
	)

	SyncErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "synccal_sync_errors_total",
			Help: "Total number of sync errors",
		},
		[]string{"destination", "type"},
	)

	LastSyncTimestamp = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "synccal_last_sync_timestamp",
			Help: "Unix timestamp of last successful sync",
		},
		[]string{"destination"},
	)
)

func init() {
	prometheus.MustRegister(SyncDuration, EventsSynced, SyncErrors, LastSyncTimestamp)
}

func Handler() http.Handler {
	return promhttp.Handler()
}