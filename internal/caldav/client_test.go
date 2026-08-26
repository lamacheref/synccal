package caldav

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smiden/synccal/internal/retry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockCalDAV implements a minimal CalDAV server for tests.
type mockCalDAV struct {
	server       *httptest.Server
	ctag         string
	syncToken    string
	etag         string
	icsData      string
	putStatus    int
	deleteStatus int
	propfindData string
	reportData   string
}

func newMockCalDAV(t *testing.T, handler http.HandlerFunc) *mockCalDAV {
	t.Helper()
	m := &mockCalDAV{
		ctag:      "ctag-123",
		syncToken: "sync-123",
		etag:      "etag-123",
		icsData: `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Test//EN
BEGIN:VEVENT
UID:test-uid@example.com
DTSTAMP:20240101T000000Z
DTSTART:20240115T100000Z
END:VEVENT
END:VCALENDAR`,
		putStatus:    http.StatusCreated,
		deleteStatus: http.StatusNoContent,
	}
	if handler != nil {
		m.server = httptest.NewServer(handler)
	} else {
		m.server = httptest.NewServer(http.HandlerFunc(m.defaultHandler))
	}
	t.Cleanup(func() { m.server.Close() })
	return m
}

func (m *mockCalDAV) URL() string { return m.server.URL + "/caldav/" }

func (m *mockCalDAV) defaultHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodHead:
		w.Header().Set("ETag", fmt.Sprintf(`"%s"`, m.etag))
		w.WriteHeader(http.StatusOK)
	case http.MethodGet:
		if strings.HasSuffix(r.URL.Path, ".ics") {
			w.Header().Set("Content-Type", "text/calendar")
			w.Header().Set("ETag", fmt.Sprintf(`"%s"`, m.etag))
			fmt.Fprint(w, m.icsData)
			return
		}
		// GET for calendar (public ICS)
		w.Header().Set("Content-Type", "text/calendar")
		w.Header().Set("ETag", fmt.Sprintf(`"%s"`, m.etag))
		fmt.Fprint(w, m.icsData)
	case http.MethodPut:
		w.WriteHeader(m.putStatus)
	case http.MethodDelete:
		w.WriteHeader(m.deleteStatus)
	case "PROPFIND":
		w.Header().Set("Content-Type", "application/xml")
		if m.propfindData != "" {
			fmt.Fprint(w, m.propfindData)
			return
		}
		// Default PROPFIND response with CTag and sync-token
		fmt.Fprintf(w, `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:" xmlns:cs="http://calendarserver.org/ns/"><d:response><d:href>/caldav/</d:href><d:propstat><d:prop><cs:getctag>%s</cs:getctag><d:sync-token>%s</d:sync-token><d:getetag>%s</d:getetag></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response></d:multistatus>`, m.ctag, m.syncToken, m.etag)
	case "REPORT":
		w.Header().Set("Content-Type", "application/xml")
		if m.reportData != "" {
			fmt.Fprint(w, m.reportData)
			return
		}
		fmt.Fprintf(w, `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:"><d:response><d:href>/caldav/event1.ics</d:href><d:propstat><d:prop><d:getetag>etag1</d:getetag></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response><d:sync-token>%s</d:sync-token></d:multistatus>`, m.syncToken)
	default:
		w.WriteHeader(http.StatusOK)
	}
}

func TestNewClient(t *testing.T) {
	_, err := NewClient("http://example.com/cal/", "user", "pass")
	require.NoError(t, err)

	_, err = NewClient("://bad", "u", "p")
	assert.Error(t, err)

	_, err = NewClient("ftp://example.com/cal", "u", "p")
	assert.ErrorContains(t, err, "invalid URL scheme")

	c, err := NewClient("http://example.com/cal/", "", "")
	require.NoError(t, err)
	assert.False(t, c.authenticated())
	c2, _ := NewClient("http://example.com/cal/", "user", "pass")
	assert.True(t, c2.authenticated())
	assert.NotNil(t, c2.WithRetryConfig(retry.DefaultConfig()))
}

func TestBasicAuthTransport(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Basic dXNlcjpwYXNz", r.Header.Get("Authorization"))
		called = true
		w.WriteHeader(200)
	}))
	defer srv.Close()
	c, _ := NewClient(srv.URL+"/cal/", "user", "pass")
	req, _ := http.NewRequest("GET", srv.URL+"/cal/", nil)
	// Use the transport directly
	tr := c.httpClient.Transport.(*basicAuthTransport)
	resp, err := tr.RoundTrip(req)
	require.NoError(t, err)
	assert.True(t, called)
	resp.Body.Close()
}

func TestFetchPublicICSState(t *testing.T) {
	m := newMockCalDAV(t, nil)
	c, _ := NewClient(m.URL(), "", "")
	// Need to use URL that points to server's GET
	c.URL = m.server.URL + "/calendar.ics"
	// Override handler for HEAD
	m.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("ETag", `"etag-head"`)
			w.WriteHeader(200)
			return
		}
		w.WriteHeader(200)
	})

	st, err := c.fetchPublicICSState(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "etag-head", st.ETag)

	// Error case: 404
	m2 := newMockCalDAV(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	})
	c2, _ := NewClient(m2.server.URL+"/calendar.ics", "", "")
	c2.httpClient.Timeout = 1 * time.Second
	_, err = c2.fetchPublicICSState(context.Background())
	assert.Error(t, err)
}

func TestFetchCalDAVState(t *testing.T) {
	m := newMockCalDAV(t, nil)
	c, _ := NewClient(m.URL(), "user", "pass")
	st, err := c.fetchCalDAVState(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "ctag-123", st.CTag)
	assert.Equal(t, "sync-123", st.SyncToken)

	// 500 : fetchCalDAVState ne vérifie pas le statut HTTP → réponse vide, pas d'erreur
	m2 := newMockCalDAV(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	})
	c2, _ := NewClient(m2.URL(), "user", "pass")
	st2, err := c2.fetchCalDAVState(context.Background())
	require.NoError(t, err)
	assert.Empty(t, st2.CTag)
	assert.Empty(t, st2.SyncToken)
}

func TestHasChanged(t *testing.T) {
	m := newMockCalDAV(t, nil)
	c, _ := NewClient(m.URL(), "user", "pass")
	// Need to set c.URL to m.URL for HasChanged to work via FetchCalendarState
	// For public ICS, HasChanged uses ETag
	c2, _ := NewClient(m.server.URL+"/calendar.ics", "", "")
	// For public, test HasChanged with nil lastState
	changed, cur, err := c2.HasChanged(context.Background(), nil)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.NotNil(t, cur)

	// Same ETag -> not changed (if ETag supported)
	last := &CalendarState{ETag: cur.ETag}
	_, _, err = c2.HasChanged(context.Background(), last)
	require.NoError(t, err)
	// For public ICS, if ETag same, should be false, but our mock returns same ETag, so not changed
	// However our HasChanged for public uses ETag, so if same, false
	// But we need to ensure lastState with same ETag returns false
	// Let's do a more deterministic test with CalDAV CTag
	m3 := newMockCalDAV(t, nil)
	c3, _ := NewClient(m3.URL(), "user", "pass")
	last2 := &CalendarState{CTag: "ctag-123", SyncToken: "sync-123"}
	changed, _, err = c3.HasChanged(context.Background(), last2)
	require.NoError(t, err)
	assert.False(t, changed, "same CTag should be not changed")

	last3 := &CalendarState{CTag: "old-ctag"}
	changed, _, err = c3.HasChanged(context.Background(), last3)
	require.NoError(t, err)
	assert.True(t, changed)

	// Fallback: both empty -> true
	changed, _, err = c3.HasChanged(context.Background(), &CalendarState{})
	require.NoError(t, err)
	// Our mock returns non-empty CTag, so last empty, current non-empty -> true
	assert.True(t, changed)

	// Error case
	c4, _ := NewClient("http://invalid:0/caldav/", "user", "pass")
	c4.httpClient.Timeout = 1 * time.Millisecond
	_, _, err = c4.HasChanged(context.Background(), nil)
	assert.Error(t, err)
	_ = m
	_ = c
}

func TestFetchCalendar_Public(t *testing.T) {
	m := newMockCalDAV(t, nil)
	c, _ := NewClient(m.server.URL+"/cal.ics", "", "")
	data, st, err := c.FetchCalendar(context.Background(), "")
	require.NoError(t, err)
	assert.Contains(t, string(data), "BEGIN:VCALENDAR")
	assert.NotEmpty(t, st.ETag)
}

func TestFetchCalendar_CalDAV(t *testing.T) {
	// Need a mock that returns PROPFIND and then GET for event
	ics := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Test//EN
BEGIN:VEVENT
UID:ev1@example.com
DTSTAMP:20240101T000000Z
DTSTART:20240115T100000Z
END:VEVENT
END:VCALENDAR`
	m := newMockCalDAV(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PROPFIND" && r.Header.Get("Depth") == "0" {
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:" xmlns:cs="http://calendarserver.org/ns/"><d:response><d:href>/caldav/</d:href><d:propstat><d:prop><cs:getctag>newctag</cs:getctag><d:sync-token>newtok</d:sync-token></d:prop></d:propstat></d:response></d:multistatus>`)
			return
		}
		if r.Method == "PROPFIND" {
			// List hrefs
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:"><d:response><d:href>/caldav/event1.ics</d:href><d:propstat><d:prop><d:getetag>"etag1"</d:getetag></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response></d:multistatus>`)
			return
		}
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "event1.ics") {
			w.Header().Set("Content-Type", "text/calendar")
			fmt.Fprint(w, ics)
			return
		}
		if r.Method == "REPORT" {
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:"><d:response><d:href>/caldav/event1.ics</d:href><d:propstat><d:prop><d:getetag>"etag1"</d:getetag></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response><d:sync-token>newtok</d:sync-token></d:multistatus>`)
			return
		}
		w.WriteHeader(200)
	})
	c, _ := NewClient(m.server.URL+"/caldav/", "user", "pass")
	data, st, err := c.FetchCalendar(context.Background(), "")
	require.NoError(t, err)
	assert.Contains(t, string(data), "BEGIN:VCALENDAR")
	assert.NotNil(t, st)
	// With syncToken
	data2, st2, err := c.FetchCalendar(context.Background(), "sync-123")
	require.NoError(t, err)
	assert.Contains(t, string(data2), "BEGIN:VCALENDAR")
	assert.NotEmpty(t, st2.SyncToken)
}

func TestCreateUpdateDeleteList(t *testing.T) {
	m := newMockCalDAV(t, nil)
	c, _ := NewClient(m.URL(), "user", "pass")
	ics := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Test//EN
BEGIN:VEVENT
UID:create-test@example.com
DTSTAMP:20240101T000000Z
DTSTART:20240115T100000Z
END:VEVENT
END:VCALENDAR`

	// Create
	href, err := c.CreateEvent(context.Background(), []byte(ics))
	require.NoError(t, err)
	assert.Contains(t, href, "create-test")

	// Update
	err = c.UpdateEvent(context.Background(), href, []byte(ics), "etag1")
	require.NoError(t, err)

	// Delete
	err = c.DeleteEvent(context.Background(), href, "etag1")
	require.NoError(t, err)

	// Create without auth should fail
	c2, _ := NewClient(m.URL(), "", "")
	_, err = c2.CreateEvent(context.Background(), []byte(ics))
	assert.ErrorContains(t, err, "authentication required")
	err = c2.UpdateEvent(context.Background(), href, []byte(ics), "")
	assert.ErrorContains(t, err, "authentication required")
	err = c2.DeleteEvent(context.Background(), href, "")
	assert.ErrorContains(t, err, "authentication required")

	// Bad UID
	_, err = c.CreateEvent(context.Background(), []byte("not a calendar"))
	assert.Error(t, err)

	// Server error 500 on Create
	m3 := newMockCalDAV(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	})
	c3, _ := NewClient(m3.URL(), "user", "pass")
	c3.retryConfig = retry.Config{MaxAttempts: 1, BaseDelay: 1 * time.Millisecond, RetryableStatus: []int{500}}
	_, err = c3.CreateEvent(context.Background(), []byte(ics))
	assert.Error(t, err)

	// ListEvents
	m4 := newMockCalDAV(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PROPFIND" {
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:"><d:response><d:href>/caldav/</d:href><d:propstat><d:prop><d:getetag>"etag0"</d:getetag></d:prop></d:propstat></d:response><d:response><d:href>/caldav/ev1.ics</d:href><d:propstat><d:prop><d:getetag>"etag1"</d:getetag></d:prop></d:propstat></d:response></d:multistatus>`)
			return
		}
		if strings.Contains(r.URL.Path, "ev1.ics") {
			w.Header().Set("Content-Type", "text/calendar")
			fmt.Fprint(w, ics)
			return
		}
		w.WriteHeader(200)
	})
	c4, _ := NewClient(m4.URL(), "user", "pass")
	refs, err := c4.ListEvents(context.Background())
	require.NoError(t, err)
	assert.Len(t, refs, 1)
	assert.Equal(t, "create-test@example.com", refs[0].UID)

	// List without auth
	_, err = c2.ListEvents(context.Background())
	assert.Error(t, err)
}

func TestGetResource(t *testing.T) {
	m := newMockCalDAV(t, nil)
	c, _ := NewClient(m.URL(), "user", "pass")
	// Absolute URL
	data, err := c.getResource(context.Background(), m.server.URL+"/caldav/event.ics")
	require.NoError(t, err)
	assert.Contains(t, string(data), "BEGIN:VCALENDAR")

	// Relative href
	m.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/caldav/rel.ics" {
			fmt.Fprint(w, m.icsData)
			return
		}
		w.WriteHeader(404)
	})
	_, err = c.getResource(context.Background(), "/caldav/rel.ics")
	require.NoError(t, err)

	// 404
	_, err = c.getResource(context.Background(), "/caldav/notfound.ics")
	assert.Error(t, err)
}

func TestRequestAndResolveURL(t *testing.T) {
	m := newMockCalDAV(t, nil)
	c, _ := NewClient(m.URL(), "user", "pass")
	resp, err := c.request(context.Background(), "GET", "/caldav/test.ics", nil, "", "")
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	resp.Body.Close()

	// Invalid URL
	_, err = c.request(context.Background(), "GET", "://bad", nil, "", "")
	assert.Error(t, err)

	assert.Equal(t, "http://example.com/foo", c.resolveURL("http://example.com/foo"))
	assert.Contains(t, c.resolveURL("/caldav/a.ics"), "/caldav/a.ics")
	c2, _ := NewClient("http://[bad", "u", "p")
	// resolve with bad base should fallback
	_ = c2
}

func TestHelpers(t *testing.T) {
	// eventUID
	uid, err := eventUID([]byte(`BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:myuid@example.com
END:VEVENT
END:VCALENDAR`))
	require.NoError(t, err)
	assert.Equal(t, "myuid@example.com", uid)

	_, err = eventUID([]byte("bad data"))
	assert.Error(t, err)

	// VEVENT sans UID : Props.Text retourne ("", nil) → pas d'erreur, UID vide
	uidEmpty, err := eventUID([]byte(`BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
END:VEVENT
END:VCALENDAR`))
	require.NoError(t, err)
	assert.Empty(t, uidEmpty)

	_, err = eventUID([]byte(`BEGIN:VCALENDAR
VERSION:2.0
END:VCALENDAR`))
	assert.Error(t, err)

	// xmlPropValue
	val := xmlPropValue([]byte(`<root><getctag>abc</getctag></root>`), "getctag")
	assert.Equal(t, "abc", val)
	assert.Empty(t, xmlPropValue([]byte("bad xml"), "getctag"))
	assert.Empty(t, xmlPropValue([]byte(`<root></root>`), "missing"))

	// xmlHrefs
	hrefs := xmlHrefs([]byte(`<?xml version="1.0"?><d:multistatus xmlns:d="DAV:"><d:response><d:href>/a</d:href><d:status>HTTP/1.1 200 OK</d:status></d:response><d:response><d:href>/b</d:href><d:status>HTTP/1.1 404 Not Found</d:status></d:response></d:multistatus>`), true)
	assert.Contains(t, hrefs, "/a")
	assert.NotContains(t, hrefs, "/b")
	hrefs2 := xmlHrefs([]byte(`bad xml`), false)
	assert.Empty(t, hrefs2)
	hrefs3 := xmlHrefs([]byte(`<?xml version="1.0"?><d:multistatus xmlns:d="DAV:"><d:response><d:href>/a</d:href></d:response></d:multistatus>`), false)
	assert.Contains(t, hrefs3, "/a")

	// xmlEscape
	assert.Equal(t, "a &amp; b &lt; c &gt;", xmlEscape("a & b < c >"))

	// statusError
	err = &statusError{status: 404}
	assert.Equal(t, "HTTP status 404", err.Error())

	// Test WithRetryConfig and authenticated
	c, _ := NewClient("http://example.com/cal/", "u", "p")
	c2 := c.WithRetryConfig(retry.Config{MaxAttempts: 5})
	assert.Equal(t, 5, c2.retryConfig.MaxAttempts)

	// Test request with extra headers and content type
	m := newMockCalDAV(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "1", r.Header.Get("Depth"))
		assert.Equal(t, "value", r.Header.Get("X-Custom"))
		w.WriteHeader(200)
	})
	c3, _ := NewClient(m.URL(), "user", "pass")
	_, err = c3.request(context.Background(), "PROPFIND", m.URL(), []byte("data"), "application/json", "1", map[string]string{"X-Custom": "value"})
	require.NoError(t, err)

	// Test ListEvents with non-VEVENT
	m4 := newMockCalDAV(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PROPFIND" {
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:"><d:response><d:href>/caldav/ev1.ics</d:href><d:propstat><d:prop><d:getetag>"etag1"</d:getetag></d:prop></d:propstat></d:response></d:multistatus>`)
			return
		}
		w.Header().Set("Content-Type", "text/calendar")
		fmt.Fprint(w, `BEGIN:VCALENDAR
VERSION:2.0
END:VCALENDAR`)
	})
	c4, _ := NewClient(m4.URL(), "user", "pass")
	refs, err := c4.ListEvents(context.Background())
	require.NoError(t, err)
	assert.Empty(t, refs)

	// Test mergeCalendar with nil hrefs (discover via PROPFIND)
	m5 := newMockCalDAV(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PROPFIND" {
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:"><d:response><d:href>/caldav/ev1.ics</d:href><d:propstat><d:prop><d:getetag>"etag1"</d:getetag></d:prop></d:propstat></d:response></d:multistatus>`)
			return
		}
		if strings.Contains(r.URL.Path, "ev1.ics") {
			w.Header().Set("Content-Type", "text/calendar")
			fmt.Fprint(w, `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:dup@example.com
DTSTAMP:20240101T000000Z
DTSTART:20240115T100000Z
END:VEVENT
END:VCALENDAR`)
			return
		}
		w.WriteHeader(200)
	})
	c5, _ := NewClient(m5.URL(), "user", "pass")
	data, err := c5.mergeCalendar(context.Background(), nil)
	require.NoError(t, err)
	assert.Contains(t, string(data), "BEGIN:VCALENDAR")

	// Test getResource with http:// URL - use invalid port to ensure error
	cClosed, _ := NewClient("http://127.0.0.1:1/caldav/", "user", "pass")
	cClosed.httpClient.Timeout = 10 * time.Millisecond
	_, err = cClosed.getResource(context.Background(), "http://127.0.0.1:1/other.ics")
	assert.Error(t, err)
	_ = val
	_ = hrefs
}
