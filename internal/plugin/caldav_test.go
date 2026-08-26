package plugin

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/emersion/go-ical"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	const ics = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Test//EN
BEGIN:VEVENT
UID:wrap-1@example.com
DTSTAMP:20240101T000000Z
DTSTART:20240115T100000Z
END:VEVENT
END:VCALENDAR`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead:
			w.Header().Set("ETag", `"etag-h"`)
			w.WriteHeader(200)
		case r.Method == "PROPFIND" && r.Header.Get("Depth") == "0":
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:" xmlns:cs="http://calendarserver.org/ns/"><d:response><d:href>/dav/</d:href><d:propstat><d:prop><cs:getctag>ctag-w</cs:getctag><d:sync-token>tok-w</d:sync-token></d:prop></d:propstat></d:response></d:multistatus>`)
		case r.Method == "PROPFIND":
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:"><d:response><d:href>/dav/wrap-1.ics</d:href><d:propstat><d:prop><d:getetag>"etag-1"</d:getetag></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response></d:multistatus>`)
		case r.Method == "REPORT":
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:"><d:response><d:href>/dav/wrap-1.ics</d:href><d:propstat><d:prop><d:getetag>"etag-1"</d:getetag></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response><d:sync-token>tok-w2</d:sync-token></d:multistatus>`)
		case strings.HasSuffix(r.URL.Path, ".ics"):
			w.Header().Set("Content-Type", "text/calendar")
			w.Header().Set("ETag", `"etag-g"`)
			fmt.Fprint(w, ics)
		case r.Method == http.MethodPut:
			w.WriteHeader(201)
		case r.Method == http.MethodDelete:
			w.WriteHeader(204)
		default:
			w.WriteHeader(200)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCaldavSourceWrapper(t *testing.T) {
	srv := newMockServer(t)

	sc, err := NewSource(SourceConfig{Type: "caldav", URL: srv.URL + "/dav/", Username: "u", Password: "p"})
	require.NoError(t, err)
	assert.Equal(t, "caldav", sc.Type())

	ctx := context.Background()

	// HasChanged nil → true
	changed, st, err := sc.HasChanged(ctx, nil)
	require.NoError(t, err)
	assert.True(t, changed)
	require.NotNil(t, st)
	assert.Equal(t, "ctag-w", st.CTag)

	// HasChanged same state → false
	changed, _, err = sc.HasChanged(ctx, &CalendarState{CTag: "ctag-w", SyncToken: "tok-w"})
	require.NoError(t, err)
	assert.False(t, changed)

	// Fetch sans token
	data, fst, err := sc.Fetch(ctx, "")
	require.NoError(t, err)
	assert.Contains(t, string(data), "BEGIN:VCALENDAR")
	assert.Equal(t, "ctag-w", fst.CTag)

	// Fetch avec token
	data, _, err = sc.Fetch(ctx, "tok-w")
	require.NoError(t, err)
	assert.Contains(t, string(data), "BEGIN:VCALENDAR")

	// Alias ics
	sc2, err := NewSource(SourceConfig{Type: "ics", URL: srv.URL + "/cal.ics"})
	require.NoError(t, err)
	assert.Equal(t, "caldav", sc2.Type(), "le wrapper ics réutilise caldavSource qui expose caldav")

	// Erreur serveur injoignable
	bad, err := NewSource(SourceConfig{Type: "caldav", URL: "http://127.0.0.1:1/dav/"})
	require.NoError(t, err)
	_, _, err = bad.HasChanged(ctx, nil)
	assert.Error(t, err)
}

func TestCaldavDestinationWrapper(t *testing.T) {
	srv := newMockServer(t)

	dc, err := NewDestination(DestinationConfig{Type: "caldav", URL: srv.URL + "/dav/", Username: "u", Password: "p", Source: "s"})
	require.NoError(t, err)
	assert.Equal(t, "caldav", dc.Type())

	ctx := context.Background()
	ics := []byte(`BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Test//EN
BEGIN:VEVENT
UID:wrap-new@example.com
DTSTAMP:20240101T000000Z
DTSTART:20240120T100000Z
END:VEVENT
END:VCALENDAR`)

	href, err := dc.CreateEvent(ctx, ics)
	require.NoError(t, err)
	assert.NotEmpty(t, href)

	require.NoError(t, dc.UpdateEvent(ctx, href, ics, "etag"))
	require.NoError(t, dc.DeleteEvent(ctx, href, "etag"))

	refs, err := dc.ListEvents(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, refs)
}

func TestStateConverters(t *testing.T) {
	in := &CalendarState{CTag: "c", SyncToken: "t", ETag: "e"}

	// toPluginState / fromStorageState / toStorageState / toCaldavState round trips
	nilP := toCaldavState(nil)
	require.NotNil(t, nilP)
	nilS := fromStorageState(nil)
	require.NotNil(t, nilS)
	nilSt := toStorageState(nil)
	require.NotNil(t, nilSt)

	st := fromStorageState(toStorageState(in))
	assert.Equal(t, in.CTag, st.CTag)
	assert.Equal(t, in.SyncToken, st.SyncToken)
	assert.Equal(t, in.ETag, st.ETag)
}

func TestListAllAndRegistry(t *testing.T) {
	all := ListAll()
	require.NotEmpty(t, all)
	kinds := map[string]int{}
	for _, p := range all {
		kinds[p.Kind]++
	}
	assert.GreaterOrEqual(t, kinds["source"], 2)
	assert.GreaterOrEqual(t, kinds["destination"], 1)
	assert.GreaterOrEqual(t, kinds["transformer"], 5)

	// Tri : kind puis type
	for i := 1; i < len(all); i++ {
		if all[i-1].Kind == all[i].Kind {
			assert.LessOrEqual(t, all[i-1].Type, all[i].Type, "liste triée par type dans un même kind")
		}
	}

	assert.NotEmpty(t, ListSources())
	assert.NotEmpty(t, ListDestinations())
	assert.NotEmpty(t, ListTransformers())
}

func TestAllTransformersFactories(t *testing.T) {
	ctx := context.Background()

	// mask-private via factory
	mt, err := NewTransformer("mask-private", map[string]string{"summary": "X"})
	require.NoError(t, err)
	assert.Equal(t, "mask-private", mt.Name())
	comp := makeEvent("PRIVATE", "")
	out, keep, err := mt.Transform(ctx, comp)
	require.NoError(t, err)
	assert.True(t, keep)
	sum, _ := out.Props.Text("SUMMARY")
	assert.Equal(t, "X", sum)

	// mask-private sans option → défaut "Busy"
	mt2, _ := NewTransformer("mask-private", nil)
	out2, keep, _ := mt2.Transform(ctx, makeEvent("CONFIDENTIAL", ""))
	assert.True(t, keep)
	sum2, _ := out2.Props.Text("SUMMARY")
	assert.Equal(t, "Busy", sum2)

	// mask-private sur event non privé → inchangé
	orig := makeEvent("", "")
	out3, keep, _ := mt2.Transform(ctx, orig)
	assert.True(t, keep)
	assert.Equal(t, comp3UID(orig), comp3UID(out3))

	// prefix-uid via factory avec prefix direct
	pt, _ := NewTransformer("prefix-uid", map[string]string{"prefix": "abcd1234"})
	assert.Equal(t, "prefix-uid", pt.Name())
	out4, keep, _ := pt.Transform(ctx, makeEvent("", ""))
	assert.True(t, keep)
	u4, _ := out4.Props.Text("UID")
	assert.True(t, strings.HasPrefix(u4, "abcd1234-"))

	// prefix-uid avec source_url → hash
	pt2, _ := NewTransformer("prefix-uid", map[string]string{"source_url": "https://x.example.com/a.ics"})
	out5, keep, _ := pt2.Transform(ctx, makeEvent("", ""))
	assert.True(t, keep)
	u5, _ := out5.Props.Text("UID")
	assert.Len(t, u5, 8+1+len(comp3UID(makeEvent("", ""))))

	// filter-category via factory
	ct, _ := NewTransformer("filter-category", map[string]string{"categories": "work,home"})
	assert.Equal(t, "filter-category", ct.Name())
	ev := makeEvent("", "")
	ev.Props.SetText("CATEGORIES", "work")
	_, keep, _ = ct.Transform(ctx, ev)
	assert.True(t, keep)
	evNoCat := makeEvent("", "")
	_, keep, _ = ct.Transform(ctx, evNoCat)
	assert.False(t, keep, "sans CATEGORIES → filtré si liste non vide")

	// filter-category sans options → tout passe
	ct2, _ := NewTransformer("filter-category", nil)
	_, keep, _ = ct2.Transform(ctx, evNoCat)
	assert.True(t, keep)

	// prefix-summary via factory
	st_, _ := NewTransformer("prefix-summary", map[string]string{"prefix": "[S] "})
	assert.Equal(t, "prefix-summary", st_.Name())
	evSum := makeEvent("", "")
	evSum.Props.SetText("SUMMARY", "Hello")
	out6, keep, _ := st_.Transform(ctx, evSum)
	assert.True(t, keep)
	sum6, _ := out6.Props.Text("SUMMARY")
	assert.Equal(t, "[S] Hello", sum6)

	// prefix-summary vide → inchangé
	st2_, _ := NewTransformer("prefix-summary", nil)
	out7, keep, _ := st2_.Transform(ctx, evSum)
	assert.True(t, keep)
	assert.Equal(t, comp3UID(evSum), comp3UID(out7))

	// filter-private via factory
	ft, err := NewTransformer("filter-private", nil)
	require.NoError(t, err)
	assert.Equal(t, "filter-private", ft.Name())

	// Pipeline.Add
	pl := NewPipeline(ft)
	pl.Add(pt)
	_, keep, err = pl.Apply(ctx, makeEvent("", ""))
	require.NoError(t, err)
	assert.True(t, keep)

	// Apply avec erreur du transformer → propagée
	plErr := NewPipeline(&errTransformer{})
	_, _, err = plErr.Apply(ctx, makeEvent("", ""))
	assert.Error(t, err)

	// Apply avec composante filtrée en milieu de chaîne
	plF := NewPipeline(&FilterPrivateTransformer{}, pt)
	_, keep, err = plF.Apply(ctx, makeEvent("PRIVATE", ""))
	require.NoError(t, err)
	assert.False(t, keep)
}

type errTransformer struct{}

func (e *errTransformer) Name() string { return "err" }
func (e *errTransformer) Transform(_ context.Context, _ *ical.Component) (*ical.Component, bool, error) {
	return nil, false, assert.AnError
}

// makeEvent crée un VEVENT avec UID fixe et CLASS optionnelle.
func makeEvent(class, _ string) *ical.Component {
	c := ical.NewComponent("VEVENT")
	c.Props.SetText("UID", "fixed-uid@example.com")
	if class != "" {
		c.Props.SetText("CLASS", class)
	}
	return c
}

func comp3UID(c *ical.Component) string {
	u, _ := c.Props.Text("UID")
	return u
}

func TestToPluginStateNil(t *testing.T) {
	out := toPluginState(nil)
	require.NotNil(t, out)
	assert.Empty(t, out.CTag)
}
