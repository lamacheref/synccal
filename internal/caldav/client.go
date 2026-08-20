package caldav

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/emersion/go-ical"
	"github.com/smiden/synccal/internal/retry"
)

// Client is a minimal CalDAV client implemented over raw HTTP. It supports
// public ICS feeds (GET/HEAD + ETag) and authenticated CalDAV calendars
// (PROPFIND CTag/sync-token, REPORT sync-collection, PUT/DELETE).
type Client struct {
	URL         string
	Username    string
	Password    string
	httpClient  *http.Client
	retryConfig retry.Config
}

func NewClient(rawURL, username, password string) (*Client, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("invalid URL scheme %q", parsedURL.Scheme)
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
	}

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		// Never follow redirects: a redirect (e.g. to a login page) should be
		// surfaced as an error instead of silently succeeding.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: transport,
	}

	if username != "" && password != "" {
		httpClient.Transport = &basicAuthTransport{
			transport: transport,
			username:  username,
			password:  password,
		}
	}

	return &Client{
		URL:         rawURL,
		Username:    username,
		Password:    password,
		httpClient:  httpClient,
		retryConfig: retry.DefaultConfig(),
	}, nil
}

func (c *Client) WithRetryConfig(cfg retry.Config) *Client {
	c.retryConfig = cfg
	return c
}

func (c *Client) authenticated() bool {
	return c.Username != "" && c.Password != ""
}

type basicAuthTransport struct {
	transport http.RoundTripper
	username  string
	password  string
}

func (t *basicAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.SetBasicAuth(t.username, t.password)
	return t.transport.RoundTrip(req)
}

type CalendarState struct {
	CTag      string
	SyncToken string
	ETag      string
}

func (c *Client) FetchCalendarState(ctx context.Context) (*CalendarState, error) {
	if !c.authenticated() {
		return c.fetchPublicICSState(ctx)
	}
	return c.fetchCalDAVState(ctx)
}

func (c *Client) fetchPublicICSState(ctx context.Context) (*CalendarState, error) {
	var state CalendarState

	err := retry.Do(ctx, c.retryConfig, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, c.URL, nil)
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("fetch public ICS HEAD: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotModified {
			return &statusError{status: resp.StatusCode}
		}

		state.ETag = strings.Trim(resp.Header.Get("ETag"), `"`)
		return nil
	})

	return &state, err
}

func (c *Client) fetchCalDAVState(ctx context.Context) (*CalendarState, error) {
	var state CalendarState

	err := retry.Do(ctx, c.retryConfig, func() error {
		body := `<?xml version="1.0" encoding="utf-8" ?>
<d:propfind xmlns:d="DAV:" xmlns:cs="http://calendarserver.org/ns/">
  <d:prop>
    <cs:getctag/>
    <d:sync-token/>
  </d:prop>
</d:propfind>`

		resp, err := c.request(ctx, "PROPFIND", c.URL, []byte(body), "application/xml; charset=utf-8", "0")
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read propfind response: %w", err)
		}

		state.CTag = xmlPropValue(data, "getctag")
		state.SyncToken = xmlPropValue(data, "sync-token")
		return nil
	})

	return &state, err
}

func (c *Client) HasChanged(ctx context.Context, lastState *CalendarState) (bool, *CalendarState, error) {
	currentState, err := c.FetchCalendarState(ctx)
	if err != nil {
		return false, nil, err
	}

	if lastState == nil {
		return true, currentState, nil
	}

	if currentState.CTag != "" && currentState.CTag != lastState.CTag {
		return true, currentState, nil
	}
	if currentState.SyncToken != "" && currentState.SyncToken != lastState.SyncToken {
		return true, currentState, nil
	}
	if currentState.ETag != "" && currentState.ETag != lastState.ETag {
		return true, currentState, nil
	}

	// Fallback: if the server provides no change token at all (no CTag,
	// sync-token or ETag, e.g. a plain public .ics), always treat the source
	// as changed. Redundant work is avoided downstream by content hashing.
	if currentState.CTag == "" && currentState.SyncToken == "" && currentState.ETag == "" &&
		lastState.CTag == "" && lastState.SyncToken == "" && lastState.ETag == "" {
		return true, currentState, nil
	}

	return false, currentState, nil
}

func (c *Client) FetchCalendar(ctx context.Context, syncToken string) ([]byte, *CalendarState, error) {
	if !c.authenticated() {
		data, etag, err := c.fetchPublicICSWithRetry(ctx)
		return data, &CalendarState{ETag: etag}, err
	}
	return c.fetchCalDAVEvents(ctx, syncToken)
}

func (c *Client) fetchPublicICSWithRetry(ctx context.Context) ([]byte, string, error) {
	var data []byte
	var etag string

	err := retry.Do(ctx, c.retryConfig, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL, nil)
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("fetch public ICS: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return &statusError{status: resp.StatusCode}
		}

		d, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read response: %w", err)
		}

		data = d
		etag = strings.Trim(resp.Header.Get("ETag"), `"`)
		return nil
	})

	return data, etag, err
}

func (c *Client) fetchCalDAVEvents(ctx context.Context, syncToken string) ([]byte, *CalendarState, error) {
	var state CalendarState
	var merged []byte

	err := retry.Do(ctx, c.retryConfig, func() error {
		hrefs := make([]string, 0)

		if syncToken != "" {
			// RFC 6578 sync-collection: only fetch the changed resources.
			body := `<?xml version="1.0" encoding="utf-8" ?>
<d:sync-collection xmlns:d="DAV:">
  <d:sync-token>` + xmlEscape(syncToken) + `</d:sync-token>
  <d:sync-level>1</d:sync-level>
  <d:prop><d:getetag/></d:prop>
</d:sync-collection>`

			resp, err := c.request(ctx, "REPORT", c.URL, []byte(body), "application/xml; charset=utf-8", "1")
			if err != nil {
				return err
			}
			data, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr != nil {
				return fmt.Errorf("read sync-collection response: %w", readErr)
			}

			state.SyncToken = xmlPropValue(data, "sync-token")
			hrefs = xmlHrefs(data, true)
		} else {
			// Initial fetch: list all resources with PROPFIND, then GET each.
			body := `<?xml version="1.0" encoding="utf-8" ?>
<d:propfind xmlns:d="DAV:">
  <d:prop><d:getetag/></d:prop>
</d:propfind>`

			resp, err := c.request(ctx, "PROPFIND", c.URL, []byte(body), "application/xml; charset=utf-8", "1")
			if err != nil {
				return err
			}
			data, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr != nil {
				return fmt.Errorf("read propfind response: %w", readErr)
			}

			hrefs = xmlHrefs(data, false)
		}

		var err error
		merged, err = c.mergeCalendar(ctx, hrefs)
		return err
	})

	if err != nil {
		return nil, &state, err
	}

	// Populate the returned state with the authoritative CTag and sync-token
	// so the next HasChanged comparison works.
	if st, err := c.fetchCalDAVState(ctx); err == nil {
		state.CTag = st.CTag
		state.SyncToken = st.SyncToken
	}

	return merged, &state, nil
}

// mergeCalendar fetches the given resources and merges all their VEVENT
// components into a single VCALENDAR. When hrefs is nil, the resource list is
// discovered with a PROPFIND first.
func (c *Client) mergeCalendar(ctx context.Context, hrefs []string) ([]byte, error) {
	if hrefs == nil {
		body := `<?xml version="1.0" encoding="utf-8" ?>
<d:propfind xmlns:d="DAV:">
  <d:prop><d:getetag/></d:prop>
</d:propfind>`

		resp, err := c.request(ctx, "PROPFIND", c.URL, []byte(body), "application/xml; charset=utf-8", "1")
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read propfind response: %w", readErr)
		}
		hrefs = xmlHrefs(data, false)
	}

	cal := ical.NewCalendar()
	cal.Props.SetText(ical.PropProductID, "-//SyncCal//EN")
	cal.Props.SetText(ical.PropVersion, "2.0")

	seen := make(map[string]bool)
	for _, href := range hrefs {
		ics, err := c.getResource(ctx, href)
		if err != nil {
			return nil, err
		}
		decoded, err := ical.NewDecoder(bytes.NewReader(ics)).Decode()
		if err != nil {
			continue
		}
		for _, child := range decoded.Children {
			if child.Name != "VEVENT" {
				continue
			}
			uid, err := child.Props.Text("UID")
			if err != nil || seen[uid] {
				continue
			}
			seen[uid] = true
			cal.Children = append(cal.Children, child)
		}
	}

	var buf bytes.Buffer
	if err := ical.NewEncoder(&buf).Encode(cal); err != nil {
		return nil, fmt.Errorf("encode merged calendar: %w", err)
	}
	return buf.Bytes(), nil
}

func (c *Client) getResource(ctx context.Context, href string) ([]byte, error) {
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, href, nil)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("get resource: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, &statusError{status: resp.StatusCode}
		}
		return io.ReadAll(resp.Body)
	}

	resp, err := c.request(ctx, http.MethodGet, href, nil, "", "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &statusError{status: resp.StatusCode}
	}
	return io.ReadAll(resp.Body)
}

func (c *Client) CreateEvent(ctx context.Context, icsData []byte) (string, error) {
	if !c.authenticated() {
		return "", fmt.Errorf("authentication required for create")
	}

	uid, err := eventUID(icsData)
	if err != nil {
		return "", err
	}

	href := c.URL
	if !strings.HasSuffix(href, "/") {
		href += "/"
	}
	href += url.PathEscape(uid) + ".ics"

	err = retry.Do(ctx, c.retryConfig, func() error {
		resp, err := c.request(ctx, http.MethodPut, href, icsData, ical.MIMEType, "")
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			return &statusError{status: resp.StatusCode}
		}
		return nil
	})

	return href, err
}

func (c *Client) UpdateEvent(ctx context.Context, href string, icsData []byte, etag string) error {
	if !c.authenticated() {
		return fmt.Errorf("authentication required for update")
	}

	return retry.Do(ctx, c.retryConfig, func() error {
		headers := map[string]string{}
		if etag != "" {
			headers["If-Match"] = `"` + etag + `"`
		}
		resp, err := c.request(ctx, http.MethodPut, href, icsData, ical.MIMEType, "", headers)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			return &statusError{status: resp.StatusCode}
		}
		return nil
	})
}

func (c *Client) DeleteEvent(ctx context.Context, href string, etag string) error {
	if !c.authenticated() {
		return fmt.Errorf("authentication required for delete")
	}

	return retry.Do(ctx, c.retryConfig, func() error {
		headers := map[string]string{}
		if etag != "" {
			headers["If-Match"] = `"` + etag + `"`
		}
		resp, err := c.request(ctx, http.MethodDelete, href, nil, "", "", headers)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			return &statusError{status: resp.StatusCode}
		}
		return nil
	})
}

func (c *Client) ListEvents(ctx context.Context) ([]EventRef, error) {
	if !c.authenticated() {
		return nil, fmt.Errorf("authentication required for list")
	}

	var refs []EventRef

	err := retry.Do(ctx, c.retryConfig, func() error {
		body := `<?xml version="1.0" encoding="utf-8" ?>
<d:propfind xmlns:d="DAV:">
  <d:prop><d:getetag/></d:prop>
</d:propfind>`

		resp, err := c.request(ctx, "PROPFIND", c.URL, []byte(body), "application/xml; charset=utf-8", "1")
		if err != nil {
			return err
		}
		data, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read propfind response: %w", readErr)
		}

		refs = nil
		for _, href := range xmlHrefs(data, false) {
			ics, err := c.getResource(ctx, href)
			if err != nil {
				return err
			}
			uid, err := eventUID(ics)
			if err != nil {
				return err
			}
			refs = append(refs, EventRef{Href: href, UID: uid})
		}
		return nil
	})

	return refs, err
}

// request issues an authenticated HTTP request to the CalDAV endpoint.
func (c *Client) request(ctx context.Context, method, href string, body []byte, contentType, depth string, extraHeaders ...map[string]string) (*http.Response, error) {
	target := c.resolveURL(href)
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if depth != "" {
		req.Header.Set("Depth", depth)
	}
	req.Header.Set("Accept", ical.MIMEType)
	for _, h := range extraHeaders {
		for k, v := range h {
			req.Header.Set(k, v)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request %s %s: %w", method, href, err)
	}
	return resp, nil
}

// resolveURL returns href as an absolute URL, resolving relative paths
// (e.g. "/remote.php/dav/...") against the client's base URL.
func (c *Client) resolveURL(href string) string {
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}
	base, err := url.Parse(c.URL)
	if err != nil {
		return href
	}
	ref, err := url.Parse(href)
	if err != nil {
		return href
	}
	return base.ResolveReference(ref).String()
}

func eventUID(icsData []byte) (string, error) {
	cal, err := ical.NewDecoder(bytes.NewReader(icsData)).Decode()
	if err != nil {
		return "", fmt.Errorf("decode ics: %w", err)
	}
	for _, child := range cal.Children {
		if child.Name == "VEVENT" {
			uid, err := child.Props.Text("UID")
			if err != nil {
				return "", fmt.Errorf("event missing UID")
			}
			return uid, nil
		}
	}
	return "", fmt.Errorf("no VEVENT found in ics data")
}

type statusError struct {
	status int
}

func (e *statusError) Error() string {
	return fmt.Sprintf("HTTP status %d", e.status)
}

type EventRef struct {
	Href string
	ETag string
	UID  string
}

// xmlPropValue extracts the text content of the first element with the given
// local name anywhere inside the XML document.
func xmlPropValue(data []byte, localName string) string {
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err != nil {
			return ""
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != localName {
			continue
		}
		var sb strings.Builder
		for {
			t, err := dec.Token()
			if err != nil {
				return strings.TrimSpace(sb.String())
			}
			switch tt := t.(type) {
			case xml.CharData:
				sb.Write(tt)
			case xml.EndElement:
				if tt.Name.Local == localName {
					return strings.TrimSpace(sb.String())
				}
			}
		}
	}
}

// xmlHrefs extracts the hrefs of all response entries in a multistatus
// document. When onlyExisting is true, entries with a non-2xx status (i.e.
// deleted resources in a sync-collection) are skipped.
func xmlHrefs(data []byte, onlyExisting bool) []string {
	var hrefs []string

	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err != nil {
			return hrefs
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if se.Name.Local != "response" {
			continue
		}

		var href, status string
		for {
			t, err := dec.Token()
			if err != nil {
				break
			}
			switch tt := t.(type) {
			case xml.StartElement:
				switch tt.Name.Local {
				case "href":
					var sb strings.Builder
					for {
						it, err := dec.Token()
						if err != nil {
							break
						}
						if c, ok := it.(xml.CharData); ok {
							sb.Write(c)
						}
						if e, ok := it.(xml.EndElement); ok && e.Name.Local == "href" {
							break
						}
					}
					href = strings.TrimSpace(sb.String())
				case "status":
					var sb strings.Builder
					for {
						it, err := dec.Token()
						if err != nil {
							break
						}
						if c, ok := it.(xml.CharData); ok {
							sb.Write(c)
						}
						if e, ok := it.(xml.EndElement); ok && e.Name.Local == "status" {
							break
						}
					}
					status = strings.TrimSpace(sb.String())
				}
			case xml.EndElement:
				if tt.Name.Local == "response" {
					if href != "" && (!onlyExisting || status == "" || strings.HasPrefix(status, "HTTP/1.1 2")) {
						hrefs = append(hrefs, href)
					}
					goto next
				}
			}
		}
	next:
	}
}

func xmlEscape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(s, "&", "&amp;"), "<", "&lt;"), ">", "&gt;")
}
