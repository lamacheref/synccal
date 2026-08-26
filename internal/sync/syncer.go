package sync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-ical"
	"github.com/robfig/cron/v3"
	"github.com/smiden/synccal/internal/config"
	"github.com/smiden/synccal/internal/metrics"
	"github.com/smiden/synccal/internal/plugin"
	"github.com/smiden/synccal/internal/storage"
	"go.uber.org/zap"
)

// ConnectionStats aggregates the state of one source→destination pairing.
type ConnectionStats struct {
	SourceName   string     `json:"source_name"`
	SourceURL    string     `json:"source_url"`
	Destination  string     `json:"destination"`
	Events       int64      `json:"events"`
	Created      int64      `json:"created"`
	Updated      int64      `json:"updated"`
	Deleted      int64      `json:"deleted"`
	Errors       int64      `json:"errors"`
	LastSync     *time.Time `json:"last_sync,omitempty"`
	LastError    string     `json:"last_error,omitempty"`
	LastDuration float64    `json:"last_duration_sec,omitempty"`
}

type Status struct {
	Running      bool              `json:"running"`
	Interval     string            `json:"interval"`
	LastSync     *time.Time        `json:"last_sync,omitempty"`
	LastError    string            `json:"last_error,omitempty"`
	LastDuration float64           `json:"last_duration_sec,omitempty"`
	Connections  []ConnectionStats `json:"connections"`
}

type Syncer struct {
	cfg          *config.Config
	sources      []plugin.SourceConnector
	destinations []plugin.DestinationConnector
	pipelines    []*plugin.Pipeline
	sourceIndex  map[string]int
	store        *storage.Store
	log          *zap.SugaredLogger
	scheduler    *cron.Cron
	mu           sync.Mutex
	running      bool

	statsMu    sync.Mutex
	connStats  map[string]*ConnectionStats
	lastSync   *time.Time
	lastError  string
	lastDurSec float64
}

func New(cfg *config.Config, sources []plugin.SourceConnector, dests []plugin.DestinationConnector, store *storage.Store, log *zap.SugaredLogger) *Syncer {
	s := &Syncer{
		cfg:          cfg,
		sources:      sources,
		destinations: dests,
		store:        store,
		log:          log,
		connStats:    make(map[string]*ConnectionStats),
		sourceIndex:  make(map[string]int),
	}
	for i, src := range cfg.Sources {
		s.sourceIndex[src.Name] = i
	}
	for _, d := range cfg.Destinations {
		srcURL := ""
		if idx, ok := s.sourceIndex[d.Source]; ok {
			srcURL = cfg.Sources[idx].URL
		}
		s.connStats[d.Name] = &ConnectionStats{SourceName: d.Source, SourceURL: srcURL, Destination: d.Name}
	}
	// Build pipelines per destination
	s.pipelines = make([]*plugin.Pipeline, len(dests))
	for i, destCfg := range cfg.Destinations {
		srcIdx, ok := s.sourceIndex[destCfg.Source]
		if !ok {
			continue
		}
		sourceCfg := cfg.Sources[srcIdx]
		pipeline := plugin.NewPipeline()
		// Legacy filter_private -> filter-private transformer if not already present
		hasFilterPrivate := false
		for _, tr := range destCfg.Transformers {
			if tr.Type == "filter-private" || tr.Type == "mask-private" {
				hasFilterPrivate = true
				break
			}
		}
		for _, tr := range sourceCfg.Transformers {
			if tr.Type == "filter-private" || tr.Type == "mask-private" {
				hasFilterPrivate = true
				break
			}
		}
		if cfg.Sync.FilterPrivate && !hasFilterPrivate {
			if t, err := plugin.NewTransformer("filter-private", nil); err == nil {
				pipeline.Add(t)
			}
		}
		// Prefix UID transformer (always, per source hash) — ensures UID collision avoidance
		prefix := sourcePrefix(sourceCfg.URL)
		if t, err := plugin.NewTransformer("prefix-uid", map[string]string{"prefix": prefix}); err == nil {
			pipeline.Add(t)
		}
		// Source transformers
		for _, tr := range sourceCfg.Transformers {
			opts := tr.Options
			if opts == nil {
				opts = make(map[string]string)
			}
			// inject source_url for prefix transformer if needed
			if tr.Type == "prefix-uid" && opts["prefix"] == "" {
				opts["prefix"] = prefix
			}
			if t, err := plugin.NewTransformer(tr.Type, opts); err == nil {
				pipeline.Add(t)
			} else {
				log.Warnw("Unknown source transformer", "source", sourceCfg.Name, "type", tr.Type, "error", err)
			}
		}
		// Destination transformers
		for _, tr := range destCfg.Transformers {
			opts := tr.Options
			if opts == nil {
				opts = make(map[string]string)
			}
			if tr.Type == "prefix-uid" && opts["prefix"] == "" {
				opts["prefix"] = prefix
			}
			if t, err := plugin.NewTransformer(tr.Type, opts); err == nil {
				pipeline.Add(t)
			} else {
				log.Warnw("Unknown destination transformer", "dest", destCfg.Name, "type", tr.Type, "error", err)
			}
		}
		s.pipelines[i] = pipeline
	}
	return s
}

func (s *Syncer) Sync(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		s.log.Warn("Sync already running, skipping")
		return nil
	}
	s.running = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	start := time.Now()
	s.log.Info("Starting sync")

	var totalCreated, totalUpdated, totalDeleted int
	var firstErr error

	for di, destClient := range s.destinations {
		destCfg := s.cfg.Destinations[di]
		srcIdx, ok := s.sourceIndex[destCfg.Source]
		if !ok {
			err := fmt.Errorf("destination %q references unknown source %q", destCfg.Name, destCfg.Source)
			s.log.Errorw("Invalid destination source reference", "dest", destCfg.Name, "source", destCfg.Source, "error", err)
			s.recordConnError(destCfg.Name, destCfg.Source, "", err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		sourceCfg := s.cfg.Sources[srcIdx]
		sourceClient := s.sources[srcIdx]
		sourceURL := sourceCfg.URL
		sourceName := sourceCfg.Name
		prefix := sourcePrefix(sourceURL)
		pipeline := s.pipelines[di]

		s.log.Infow("Syncing connection",
			"source", sourceName,
			"source_url", sourceURL,
			"dest", destCfg.Name,
			"dest_type", destCfg.Type,
			"source_type", sourceCfg.Type,
		)

		sourceState, err := s.store.GetSourceState(sourceURL)
		if err != nil {
			s.log.Warnw("Failed to get source state", "source", sourceURL, "error", err)
			sourceState = &storage.CalendarState{}
		}

		changed, newSourceState, err := sourceClient.HasChanged(ctx, toPluginState(sourceState))
		if err != nil {
			metrics.SourceErrors.WithLabelValues(sourceURL, "check_change").Inc()
			s.recordConnError(destCfg.Name, sourceName, sourceURL, err)
			if firstErr == nil {
				firstErr = fmt.Errorf("check source %s changes: %w", sourceName, err)
			}
			continue
		}

		if !changed {
			s.log.Infow("Source unchanged, skipping", "source", sourceName, "source_url", sourceURL)
			continue
		}

		s.log.Infow("Source changed, fetching events",
			"source", sourceName,
			"ctag_changed", sourceState.CTag != newSourceState.CTag,
			"sync_token_changed", sourceState.SyncToken != newSourceState.SyncToken,
		)

		syncToken := newSourceState.SyncToken
		icsData, fetchedState, err := sourceClient.Fetch(ctx, syncToken)
		if err != nil {
			metrics.SourceErrors.WithLabelValues(sourceURL, "fetch").Inc()
			s.recordConnError(destCfg.Name, sourceName, sourceURL, err)
			if firstErr == nil {
				firstErr = fmt.Errorf("fetch source %s: %w", sourceName, err)
			}
			continue
		}

		if err := s.store.SetSourceState(sourceURL, toStorageState(fetchedState)); err != nil {
			s.log.Warnw("Failed to save source state", "source", sourceURL, "error", err)
		}

		events, err := parseEventsWithPipeline(ctx, icsData, pipeline)
		if err != nil {
			metrics.SourceErrors.WithLabelValues(sourceURL, "parse").Inc()
			s.recordConnError(destCfg.Name, sourceName, sourceURL, err)
			if firstErr == nil {
				firstErr = fmt.Errorf("parse events from %s: %w", sourceName, err)
			}
			continue
		}

		s.log.Infow("Fetched source events", "source", sourceName, "count", len(events), "filter_private", s.cfg.Sync.FilterPrivate)

		connStart := time.Now()
		created, updated, deleted, err := s.syncDestination(ctx, destClient, destCfg.Name, events, prefix)
		if err != nil {
			s.log.Errorw("Connection sync failed", "source", sourceName, "dest", destCfg.Name, "error", err)
			metrics.SyncErrors.WithLabelValues(destCfg.Name, "sync").Inc()
			metrics.SourceErrors.WithLabelValues(sourceURL, "sync").Inc()
			s.recordConnError(destCfg.Name, sourceName, sourceURL, err)
			continue
		}

		totalCreated += created
		totalUpdated += updated
		totalDeleted += deleted

		duration := time.Since(connStart).Seconds()
		metrics.SyncDuration.WithLabelValues(destCfg.Name, "success").Observe(duration)
		metrics.EventsSynced.WithLabelValues(destCfg.Name, "created").Add(float64(created))
		metrics.EventsSynced.WithLabelValues(destCfg.Name, "updated").Add(float64(updated))
		metrics.EventsSynced.WithLabelValues(destCfg.Name, "deleted").Add(float64(deleted))
		metrics.LastSyncTimestamp.WithLabelValues(destCfg.Name).Set(float64(time.Now().Unix()))
		metrics.SourceEvents.WithLabelValues(sourceURL).Add(float64(len(events)))
		metrics.SourceSyncDuration.WithLabelValues(sourceURL, "success").Observe(duration)
		metrics.SourceLastSyncTimestamp.WithLabelValues(sourceURL).Set(float64(time.Now().Unix()))
		s.recordConnSuccess(destCfg.Name, sourceName, sourceURL, len(events), created, updated, deleted, duration)
	}

	totalDuration := time.Since(start).Seconds()

	if firstErr != nil {
		s.recordFailure(firstErr)
		s.log.Errorw("Sync completed with errors",
			"duration_sec", totalDuration,
			"created", totalCreated,
			"updated", totalUpdated,
			"deleted", totalDeleted,
			"error", firstErr,
		)
		return firstErr
	}

	s.recordSuccess(totalDuration)
	s.log.Infow("Sync completed",
		"duration_sec", totalDuration,
		"created", totalCreated,
		"updated", totalUpdated,
		"deleted", totalDeleted,
	)

	return nil
}

func (s *Syncer) syncDestination(ctx context.Context, dest plugin.DestinationConnector, destName string, events map[string][]byte, prefix string) (created, updated, deleted int, err error) {
	existing, err := s.store.ListMappings(destName, prefix)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("list mappings: %w", err)
	}

	seen := make(map[string]bool)

	for uid, icsData := range events {
		seen[uid] = true
		hash := hashContent(icsData)

		destUID, destHref, destETag, isDeleted, err := s.store.GetMapping(uid, destName)
		if err != nil {
			return 0, 0, 0, err
		}

		if existingHash, ok := existing[uid]; ok && existingHash == hash && !isDeleted {
			s.log.Debugw("Event unchanged", "uid", uid, "dest", destName)
			continue
		}

		if destUID == "" {
			newHref, err := dest.CreateEvent(ctx, icsData)
			if err != nil {
				return 0, 0, 0, fmt.Errorf("create event: %w", err)
			}
			if err := s.store.SetMapping(uid, destName, uid, newHref, "", hash, false); err != nil {
				return 0, 0, 0, err
			}
			s.log.Infow("Created event", "uid", uid, "dest", destName)
			created++
		} else {
			if err := dest.UpdateEvent(ctx, destHref, icsData, destETag); err != nil {
				return 0, 0, 0, fmt.Errorf("update event: %w", err)
			}
			if err := s.store.SetMapping(uid, destName, destUID, destHref, "", hash, false); err != nil {
				return 0, 0, 0, err
			}
			s.log.Infow("Updated event", "uid", uid, "dest", destName)
			updated++
		}
	}

	for sourceUID, existingHash := range existing {
		if !seen[sourceUID] {
			destUID, destHref, destETag, isDeleted, err := s.store.GetMapping(sourceUID, destName)
			if err != nil || isDeleted {
				continue
			}
			if s.cfg.Sync.DeleteMode == "hard" {
				if err := dest.DeleteEvent(ctx, destHref, destETag); err != nil {
					s.log.Errorw("Failed to delete event", "uid", sourceUID, "error", err)
				}
			}
			if err := s.store.SetMapping(sourceUID, destName, destUID, destHref, destETag, existingHash, true); err != nil {
				return 0, 0, 0, err
			}
			s.log.Infow("Marked event deleted", "uid", sourceUID, "dest", destName)
			deleted++
		}
	}

	return created, updated, deleted, nil
}

func (s *Syncer) StartScheduler() {
	if s.scheduler != nil {
		return
	}
	s.scheduler = cron.New()
	interval := s.cfg.Sync.IntervalDuration()
	if interval > 0 {
		s.scheduler.AddFunc("@every "+interval.String(), func() {
			ctx, cancel := context.WithTimeout(context.Background(), s.cfg.Sync.TimeoutDuration())
			defer cancel()
			if err := s.Sync(ctx); err != nil {
				s.log.Errorw("Scheduled sync failed", "error", err)
			}
		})
		s.scheduler.Start()
		s.log.Infow("Scheduler started", "interval", interval)
	}
}

func (s *Syncer) Stop() {
	if s.scheduler != nil {
		s.scheduler.Stop()
	}
}

func (s *Syncer) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func (s *Syncer) recordConnError(destName, sourceName, sourceURL string, err error) {
	s.statsMu.Lock()
	defer s.statsMu.Unlock()
	st := s.connStats[destName]
	if st == nil {
		st = &ConnectionStats{SourceName: sourceName, SourceURL: sourceURL, Destination: destName}
		s.connStats[destName] = st
	}
	st.Errors++
	st.LastError = err.Error()
}

func (s *Syncer) recordConnSuccess(destName, sourceName, sourceURL string, events, created, updated, deleted int, duration float64) {
	now := time.Now()
	s.statsMu.Lock()
	defer s.statsMu.Unlock()
	st := s.connStats[destName]
	if st == nil {
		st = &ConnectionStats{SourceName: sourceName, SourceURL: sourceURL, Destination: destName}
		s.connStats[destName] = st
	}
	st.SourceName = sourceName
	st.SourceURL = sourceURL
	st.Events += int64(events)
	st.Created += int64(created)
	st.Updated += int64(updated)
	st.Deleted += int64(deleted)
	st.LastError = ""
	st.LastSync = &now
	st.LastDuration = duration
}

func (s *Syncer) recordSuccess(duration float64) {
	now := time.Now()
	s.statsMu.Lock()
	defer s.statsMu.Unlock()
	s.lastSync = &now
	s.lastError = ""
	s.lastDurSec = duration
}

func (s *Syncer) recordFailure(err error) {
	s.statsMu.Lock()
	defer s.statsMu.Unlock()
	s.lastError = err.Error()
}

// Status returns a snapshot of the sync state for the web dashboard.
func (s *Syncer) Status() Status {
	s.statsMu.Lock()
	defer s.statsMu.Unlock()

	status := Status{
		Running:      s.IsRunning(),
		Interval:     s.cfg.Sync.Interval,
		LastSync:     s.lastSync,
		LastError:    s.lastError,
		LastDuration: s.lastDurSec,
	}
	for _, st := range s.connStats {
		status.Connections = append(status.Connections, *st)
	}
	return status
}

// sourcePrefix derives a short deterministic hash of the source URL. It is
// prepended to every UID synced from that source so identical UIDs coming from
// different sources never collide in a shared destination calendar.
func sourcePrefix(rawURL string) string {
	h := sha256.Sum256([]byte(rawURL))
	return hex.EncodeToString(h[:])[:8]
}

func parseEventsWithPipeline(ctx context.Context, data []byte, pipeline *plugin.Pipeline) (map[string][]byte, error) {
	cal, err := ical.NewDecoder(strings.NewReader(string(data))).Decode()
	if err != nil {
		return nil, err
	}

	events := make(map[string][]byte)
	for _, comp := range cal.Children {
		if comp.Name != "VEVENT" {
			continue
		}
		// Apply pipeline (filter-private, prefix, etc.)
		var keep bool
		var transformed *ical.Component
		if pipeline != nil {
			transformed, keep, err = pipeline.Apply(ctx, comp)
			if err != nil {
				continue
			}
			if !keep || transformed == nil {
				continue
			}
			comp = transformed
		}

		uid, err := comp.Props.Text("UID")
		if err != nil || uid == "" {
			continue
		}

		eventCal := ical.NewCalendar()
		eventCal.Props.SetText(ical.PropProductID, "-//SyncCal//EN")
		eventCal.Props.SetText(ical.PropVersion, "2.0")
		eventCal.Children = []*ical.Component{comp}
		var buf strings.Builder
		if err := ical.NewEncoder(&buf).Encode(eventCal); err != nil {
			continue
		}
		events[uid] = []byte(buf.String())
	}
	return events, nil
}

func hashContent(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func toPluginState(s *storage.CalendarState) *plugin.CalendarState {
	if s == nil {
		return &plugin.CalendarState{}
	}
	return &plugin.CalendarState{CTag: s.CTag, SyncToken: s.SyncToken, ETag: s.ETag}
}

func toStorageState(s *plugin.CalendarState) *storage.CalendarState {
	if s == nil {
		return &storage.CalendarState{}
	}
	return &storage.CalendarState{CTag: s.CTag, SyncToken: s.SyncToken, ETag: s.ETag}
}
