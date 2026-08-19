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
	"github.com/smiden/synccal/internal/caldav"
	"github.com/smiden/synccal/internal/config"
	"github.com/smiden/synccal/internal/metrics"
	"github.com/smiden/synccal/internal/storage"
	"go.uber.org/zap"
)

type Syncer struct {
	cfg          *config.Config
	source       *caldav.Client
	destinations []*caldav.Client
	store        *storage.Store
	log          *zap.SugaredLogger
	scheduler    *cron.Cron
	mu           sync.Mutex
	running      bool
}

func New(cfg *config.Config, source *caldav.Client, dests []*caldav.Client, store *storage.Store, log *zap.SugaredLogger) *Syncer {
	return &Syncer{
		cfg:          cfg,
		source:       source,
		destinations: dests,
		store:        store,
		log:          log,
	}
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

	sourceState, err := s.store.GetSourceState(s.cfg.Source.URL)
	if err != nil {
		s.log.Warnw("Failed to get source state", "error", err)
		sourceState = &storage.CalendarState{}
	}

	changed, newSourceState, err := s.source.HasChanged(ctx, sourceState)
	if err != nil {
		metrics.SyncErrors.WithLabelValues("source", "check_change").Inc()
		return fmt.Errorf("check source changes: %w", err)
	}

	if !changed {
		s.log.Info("Source unchanged, skipping sync")
		return nil
	}

	s.log.Infow("Source changed, fetching events",
		"ctag_changed", sourceState.CTag != newSourceState.CTag,
		"sync_token_changed", sourceState.SyncToken != newSourceState.SyncToken,
	)

	syncToken := newSourceState.SyncToken
	icsData, fetchedState, err := s.source.FetchCalendar(ctx, syncToken)
	if err != nil {
		metrics.SyncErrors.WithLabelValues("source", "fetch").Inc()
		return fmt.Errorf("fetch source: %w", err)
	}

	if err := s.store.SetSourceState(s.cfg.Source.URL, fetchedState); err != nil {
		s.log.Warnw("Failed to save source state", "error", err)
	}

	events, err := parseEvents(icsData, s.cfg.Sync.FilterPrivate)
	if err != nil {
		metrics.SyncErrors.WithLabelValues("source", "parse").Inc()
		return fmt.Errorf("parse events: %w", err)
	}

	s.log.Infow("Fetched source events", "count", len(events), "filter_private", s.cfg.Sync.FilterPrivate)

	var totalCreated, totalUpdated, totalDeleted int

	for i, dest := range s.destinations {
		destName := s.cfg.Destinations[i].Name
		destStart := time.Now()

		created, updated, deleted, err := s.syncDestination(ctx, dest, destName, events)
		if err != nil {
			s.log.Errorw("Destination sync failed", "dest", destName, "error", err)
			metrics.SyncErrors.WithLabelValues(destName, "sync").Inc()
			continue
		}

		totalCreated += created
		totalUpdated += updated
		totalDeleted += deleted

		duration := time.Since(destStart).Seconds()
		metrics.SyncDuration.WithLabelValues(destName, "success").Observe(duration)
		metrics.EventsSynced.WithLabelValues(destName, "created").Add(float64(created))
		metrics.EventsSynced.WithLabelValues(destName, "updated").Add(float64(updated))
		metrics.EventsSynced.WithLabelValues(destName, "deleted").Add(float64(deleted))
		metrics.LastSyncTimestamp.WithLabelValues(destName).Set(float64(time.Now().Unix()))
	}

	totalDuration := time.Since(start).Seconds()
	s.log.Infow("Sync completed",
		"duration_sec", totalDuration,
		"created", totalCreated,
		"updated", totalUpdated,
		"deleted", totalDeleted,
	)

	return nil
}

func (s *Syncer) syncDestination(ctx context.Context, dest *caldav.Client, destName string, events map[string][]byte) (created, updated, deleted int, err error) {
	existing, err := s.store.ListMappings(destName)
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

func parseEvents(data []byte, filterPrivate bool) (map[string][]byte, error) {
	cal, err := ical.NewDecoder(strings.NewReader(string(data))).Decode()
	if err != nil {
		return nil, err
	}

	events := make(map[string][]byte)
	for _, comp := range cal.Children {
		if comp.Name != "VEVENT" {
			continue
		}

		uid := comp.Props.Get("UID")
		if uid == "" {
			continue
		}

		if filterPrivate {
			class := comp.Props.Get("CLASS")
			if class == "PRIVATE" || class == "CONFIDENTIAL" {
				continue
			}
		}

		eventCal := ical.NewCalendar()
		eventCal.Children = []*ical.Component{comp}
		var buf strings.Builder
		ical.NewEncoder(&buf).Encode(eventCal)
		events[uid] = []byte(buf.String())
	}
	return events, nil
}

func hashContent(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}