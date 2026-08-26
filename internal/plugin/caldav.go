package plugin

import (
	"context"

	"github.com/smiden/synccal/internal/caldav"
	"github.com/smiden/synccal/internal/storage"
)

// caldavSource is the built-in CalDAV/ICS source connector.
// It wraps the low-level caldav.Client which already handles both
// public .ics (GET) and authenticated CalDAV (PROPFIND/REPORT).
type caldavSource struct {
	client *caldav.Client
}

func (s *caldavSource) Type() string { return "caldav" }

func (s *caldavSource) HasChanged(ctx context.Context, last *CalendarState) (bool, *CalendarState, error) {
	calState := toCaldavState(last)
	changed, cur, err := s.client.HasChanged(ctx, calState)
	if err != nil {
		return false, nil, err
	}
	return changed, toPluginState(cur), nil
}

func (s *caldavSource) Fetch(ctx context.Context, syncToken string) ([]byte, *CalendarState, error) {
	data, st, err := s.client.FetchCalendar(ctx, syncToken)
	if err != nil {
		return nil, nil, err
	}
	return data, toPluginState(st), nil
}

// caldavDestination wraps caldav.Client as DestinationConnector.
type caldavDestination struct {
	client *caldav.Client
}

func (d *caldavDestination) Type() string { return "caldav" }

func (d *caldavDestination) CreateEvent(ctx context.Context, icsData []byte) (string, error) {
	return d.client.CreateEvent(ctx, icsData)
}
func (d *caldavDestination) UpdateEvent(ctx context.Context, href string, icsData []byte, etag string) error {
	return d.client.UpdateEvent(ctx, href, icsData, etag)
}
func (d *caldavDestination) DeleteEvent(ctx context.Context, href string, etag string) error {
	return d.client.DeleteEvent(ctx, href, etag)
}
func (d *caldavDestination) ListEvents(ctx context.Context) ([]EventRef, error) {
	refs, err := d.client.ListEvents(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]EventRef, len(refs))
	for i, r := range refs {
		out[i] = EventRef{Href: r.Href, UID: r.UID, ETag: r.ETag}
	}
	return out, nil
}

func toCaldavState(s *CalendarState) *caldav.CalendarState {
	if s == nil {
		return &caldav.CalendarState{}
	}
	return &caldav.CalendarState{CTag: s.CTag, SyncToken: s.SyncToken, ETag: s.ETag}
}

func toPluginState(s *caldav.CalendarState) *CalendarState {
	if s == nil {
		return &CalendarState{}
	}
	return &CalendarState{CTag: s.CTag, SyncToken: s.SyncToken, ETag: s.ETag}
}

func toStorageState(s *CalendarState) *storage.CalendarState {
	if s == nil {
		return &storage.CalendarState{}
	}
	return &storage.CalendarState{CTag: s.CTag, SyncToken: s.SyncToken, ETag: s.ETag}
}

func fromStorageState(s *storage.CalendarState) *CalendarState {
	if s == nil {
		return &CalendarState{}
	}
	return &CalendarState{CTag: s.CTag, SyncToken: s.SyncToken, ETag: s.ETag}
}

func init() {
	// Source: type "caldav" (default) handles both public ICS and authenticated CalDAV.
	RegisterSource("caldav", PluginInfo{
		Name: "CalDAV / ICS",
		Description: "Connecteur CalDAV natif — gère les sources publiques (.ics) et authentifiées (Nextcloud, Carbonio) via CTag/sync-token/ETag",
	}, func(cfg SourceConfig) (SourceConnector, error) {
		c, err := caldav.NewClient(cfg.URL, cfg.Username, cfg.Password)
		if err != nil {
			return nil, err
		}
		return &caldavSource{client: c}, nil
	})
	// Alias "ics" for explicit public feeds (same implementation)
	RegisterSource("ics", PluginInfo{
		Name: "ICS public",
		Description: "Flux .ics public (GET + ETag) — alias de CalDAV sans auth",
	}, func(cfg SourceConfig) (SourceConnector, error) {
		c, err := caldav.NewClient(cfg.URL, cfg.Username, cfg.Password)
		if err != nil {
			return nil, err
		}
		return &caldavSource{client: c}, nil
	})

	RegisterDestination("caldav", PluginInfo{
		Name: "CalDAV",
		Description: "Connecteur destination CalDAV (Nextcloud, Carbonio) — PUT/DELETE avec gestion ETag",
	}, func(cfg DestinationConfig) (DestinationConnector, error) {
		c, err := caldav.NewClient(cfg.URL, cfg.Username, cfg.Password)
		if err != nil {
			return nil, err
		}
		return &caldavDestination{client: c}, nil
	})
}
