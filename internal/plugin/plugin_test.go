package plugin

import (
	"context"
	"testing"

	"github.com/emersion/go-ical"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilterPrivateTransformer(t *testing.T) {
	tr := &FilterPrivateTransformer{}
	comp := ical.NewComponent("VEVENT")
	comp.Props.SetText("UID", "test-1")
	comp.Props.SetText("CLASS", "PRIVATE")
	_, keep, err := tr.Transform(context.Background(), comp)
	require.NoError(t, err)
	assert.False(t, keep, "PRIVATE should be filtered")

	comp2 := ical.NewComponent("VEVENT")
	comp2.Props.SetText("UID", "test-2")
	comp2.Props.SetText("CLASS", "PUBLIC")
	_, keep, err = tr.Transform(context.Background(), comp2)
	require.NoError(t, err)
	assert.True(t, keep)

	comp3 := ical.NewComponent("VEVENT")
	comp3.Props.SetText("UID", "test-3")
	_, keep, err = tr.Transform(context.Background(), comp3)
	require.NoError(t, err)
	assert.True(t, keep, "no CLASS should be kept")
}

func TestMaskPrivateTransformer(t *testing.T) {
	tr := &MaskPrivateTransformer{MaskSummary: "Busy"}
	comp := ical.NewComponent("VEVENT")
	comp.Props.SetText("UID", "test-1")
	comp.Props.SetText("CLASS", "PRIVATE")
	comp.Props.SetText("SUMMARY", "Secret Meeting")
	comp.Props.SetText("DESCRIPTION", "Very secret")
	comp.Props.SetText("LOCATION", "Room 101")

	out, keep, err := tr.Transform(context.Background(), comp)
	require.NoError(t, err)
	assert.True(t, keep)
	assert.Equal(t, "Busy", out.Props.Get("SUMMARY").Value)
	desc, _ := out.Props.Text("DESCRIPTION")
	assert.Empty(t, desc, "DESCRIPTION should be removed")
	loc, _ := out.Props.Text("LOCATION")
	assert.Empty(t, loc)
}

func TestPrefixTransformer(t *testing.T) {
	tr := &PrefixTransformer{Prefix: "abc12345"}
	comp := ical.NewComponent("VEVENT")
	comp.Props.SetText("UID", "myuid@example.com")
	out, keep, err := tr.Transform(context.Background(), comp)
	require.NoError(t, err)
	assert.True(t, keep)
	uid, _ := out.Props.Text("UID")
	assert.Equal(t, "abc12345-myuid@example.com", uid)

	// Already prefixed should not double prefix
	comp2 := ical.NewComponent("VEVENT")
	comp2.Props.SetText("UID", "abc12345-myuid@example.com")
	out2, keep, _ := tr.Transform(context.Background(), comp2)
	assert.True(t, keep)
	uid2, _ := out2.Props.Text("UID")
	assert.Equal(t, "abc12345-myuid@example.com", uid2)
}

func TestCategoryFilterTransformer(t *testing.T) {
	tr := &CategoryFilterTransformer{Allowed: map[string]bool{"work": true, "perso": true}}
	comp := ical.NewComponent("VEVENT")
	comp.Props.SetText("UID", "1")
	comp.Props.SetText("CATEGORIES", "work")
	_, keep, _ := tr.Transform(context.Background(), comp)
	assert.True(t, keep)

	comp2 := ical.NewComponent("VEVENT")
	comp2.Props.SetText("UID", "2")
	comp2.Props.SetText("CATEGORIES", "spam")
	_, keep, _ = tr.Transform(context.Background(), comp2)
	assert.False(t, keep)

	comp3 := ical.NewComponent("VEVENT")
	comp3.Props.SetText("UID", "3")
	_, keep, _ = tr.Transform(context.Background(), comp3)
	assert.False(t, keep, "no categories should be filtered when filter list non-empty")
}

func TestPipeline(t *testing.T) {
	p := NewPipeline(
		&FilterPrivateTransformer{},
		&PrefixTransformer{Prefix: "deadbeef"},
	)
	comp := ical.NewComponent("VEVENT")
	comp.Props.SetText("UID", "u1")
	comp.Props.SetText("CLASS", "PRIVATE")
	_, keep, _ := p.Apply(context.Background(), comp)
	assert.False(t, keep, "private should be filtered before prefix")

	comp2 := ical.NewComponent("VEVENT")
	comp2.Props.SetText("UID", "u2")
	out, keep, _ := p.Apply(context.Background(), comp2)
	assert.True(t, keep)
	uid, _ := out.Props.Text("UID")
	assert.Equal(t, "deadbeef-u2", uid)
}

func TestRegistry(t *testing.T) {
	// Built-in plugins should be registered via init()
	srcs := ListSources()
	assert.GreaterOrEqual(t, len(srcs), 1)
	found := false
	for _, p := range srcs {
		if p.Type == "caldav" {
			found = true
		}
	}
	assert.True(t, found, "caldav source should be registered")

	dests := ListDestinations()
	assert.GreaterOrEqual(t, len(dests), 1)

	trs := ListTransformers()
	assert.GreaterOrEqual(t, len(trs), 3)
	// Check filter-private exists
	found = false
	for _, p := range trs {
		if p.Type == "filter-private" {
			found = true
		}
	}
	assert.True(t, found)

	// Creating unknown plugin should error
	_, err := NewSource(SourceConfig{Type: "unknown", URL: "http://example.com"})
	assert.Error(t, err)

	_, err = NewTransformer("unknown", nil)
	assert.Error(t, err)

	// Creating known should succeed
	sc, err := NewSource(SourceConfig{Type: "caldav", URL: "http://example.com/calendar.ics"})
	require.NoError(t, err)
	assert.Equal(t, "caldav", sc.Type())

	dc, err := NewDestination(DestinationConfig{Type: "caldav", URL: "http://example.com/dav/", Username: "u", Password: "p", Source: "src1"})
	require.NoError(t, err)
	assert.Equal(t, "caldav", dc.Type())

	tr, err := NewTransformer("filter-private", nil)
	require.NoError(t, err)
	assert.Equal(t, "filter-private", tr.Name())
}

func TestNewPrefixForSource(t *testing.T) {
	p1 := NewPrefixForSource("http://example.com/a.ics")
	p2 := NewPrefixForSource("http://example.com/a.ics")
	assert.Equal(t, p1, p2)
	assert.Len(t, p1, 8)
	p3 := NewPrefixForSource("http://example.com/b.ics")
	assert.NotEqual(t, p1, p3)
}
