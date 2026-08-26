package plugin

import (
	"context"

	"github.com/emersion/go-ical"
)

// CalendarState mirrors storage/caldav state for change detection.
type CalendarState struct {
	CTag      string
	SyncToken string
	ETag      string
}

// SourceConnector fetches calendar data from a source (public ICS or CalDAV).
type SourceConnector interface {
	// Type returns the plugin type identifier (e.g. "caldav", "ics").
	Type() string
	// HasChanged checks if the remote calendar changed since lastState.
	HasChanged(ctx context.Context, last *CalendarState) (bool, *CalendarState, error)
	// Fetch retrieves the full calendar as raw iCal bytes and its new state.
	Fetch(ctx context.Context, syncToken string) ([]byte, *CalendarState, error)
}

// DestinationConnector writes events to a destination calendar.
type DestinationConnector interface {
	Type() string
	CreateEvent(ctx context.Context, icsData []byte) (string, error)
	UpdateEvent(ctx context.Context, href string, icsData []byte, etag string) error
	DeleteEvent(ctx context.Context, href string, etag string) error
	ListEvents(ctx context.Context) ([]EventRef, error)
}

// EventRef is a lightweight reference to a remote VEVENT.
type EventRef struct {
	Href string
	UID  string
	ETag string
}

// EventTransformer transforms a single VEVENT component.
// Returning (nil, false, nil) means the event should be filtered out.
type EventTransformer interface {
	// Name returns the transformer identifier.
	Name() string
	// Transform receives a VEVENT component and returns the transformed one.
	// If keep==false, the event is dropped (filtered). If comp==nil and keep==true,
	// the original is kept unchanged.
	Transform(ctx context.Context, comp *ical.Component) (*ical.Component, bool, error)
}

// PluginInfo describes a registered plugin for UI discovery.
type PluginInfo struct {
	Type        string `json:"type"`
	Kind        string `json:"kind"` // "source", "destination", "transformer"
	Name        string `json:"name"`
	Description string `json:"description"`
}
