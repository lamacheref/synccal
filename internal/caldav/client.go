package caldav

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav/caldav"
	"golang.org/x/net/webdav"

	"github.com/smiden/synccal/internal/retry"
)

type Client struct {
	URL           string
	Username      string
	Password      string
	httpClient    *http.Client
	caldavClient  *caldav.Client
	retryConfig   retry.Config
}

func NewClient(rawURL, username, password string) (*Client, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
		},
	}

	if username != "" && password != "" {
		httpClient.Transport = &basicAuthTransport{
			transport: http.DefaultTransport,
			username:  username,
			password:  password,
		}
	}

	webdavClient := &webdav.Client{
		HTTPClient: httpClient,
		BaseURL:    parsedURL,
	}

	caldavClient := caldav.NewClient(webdavClient)

	return &Client{
		URL:          rawURL,
		Username:     username,
		Password:     password,
		httpClient:   httpClient,
		caldavClient: caldavClient,
		retryConfig:  retry.DefaultConfig(),
	}, nil
}

func (c *Client) WithRetryConfig(cfg retry.Config) *Client {
	c.retryConfig = cfg
	return c
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
	if c.Username == "" && c.Password == "" {
		return c.fetchPublicICSState(ctx)
	}
	return c.fetchCalDAVState(ctx)
}

func (c *Client) fetchPublicICSState(ctx context.Context) (*CalendarState, error) {
	var state CalendarState

	err := retry.Do(ctx, c.retryConfig, func() error {
		req, err := http.NewRequestWithContext(ctx, "HEAD", c.URL, nil)
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

		state.ETag = strings.Trim(resp.Header.Get("ETag"), "\"")
		return nil
	})

	return &state, err
}

func (c *Client) fetchCalDAVState(ctx context.Context) (*CalendarState, error) {
	var state CalendarState

	err := retry.Do(ctx, c.retryConfig, func() error {
		calendars, err := c.caldavClient.FindCalendars(ctx, c.URL)
		if err != nil {
			return fmt.Errorf("find calendars: %w", err)
		}

		if len(calendars) == 0 {
			return fmt.Errorf("no calendars found at %s", c.URL)
		}

		cal := calendars[0]
		props, err := cal.GetProperties(ctx, []string{"{http://calendarserver.org/ns/}ctag", "{DAV:}sync-token"})
		if err == nil {
			for _, p := range props {
				switch p.Name {
				case "{http://calendarserver.org/ns/}ctag":
					state.CTag = p.Value
				case "{DAV:}sync-token":
					state.SyncToken = p.Value
				}
			}
		}

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

	return false, currentState, nil
}

func (c *Client) FetchCalendar(ctx context.Context, syncToken string) ([]byte, *CalendarState, error) {
	if c.Username == "" && c.Password == "" {
		data, etag, err := c.fetchPublicICSWithRetry(ctx)
		return data, &CalendarState{ETag: etag}, err
	}
	return c.fetchCalDAVCalendarWithRetry(ctx, syncToken)
}

func (c *Client) fetchPublicICSWithRetry(ctx context.Context) ([]byte, string, error) {
	var data []byte
	var etag string

	err := retry.Do(ctx, c.retryConfig, func() error {
		req, err := http.NewRequestWithContext(ctx, "GET", c.URL, nil)
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
		etag = strings.Trim(resp.Header.Get("ETag"), "\"")
		return nil
	})

	return data, etag, err
}

func (c *Client) fetchCalDAVCalendarWithRetry(ctx context.Context, syncToken string) ([]byte, *CalendarState, error) {
	var data []byte
	var state CalendarState

	err := retry.Do(ctx, c.retryConfig, func() error {
		calendars, err := c.caldavClient.FindCalendars(ctx, c.URL)
		if err != nil {
			return fmt.Errorf("find calendars: %w", err)
		}

		if len(calendars) == 0 {
			return fmt.Errorf("no calendars found at %s", c.URL)
		}

		cal := calendars[0]

		props, err := cal.GetProperties(ctx, []string{"{http://calendarserver.org/ns/}ctag", "{DAV:}sync-token"})
		if err == nil {
			for _, p := range props {
				switch p.Name {
				case "{http://calendarserver.org/ns/}ctag":
					state.CTag = p.Value
				case "{DAV:}sync-token":
					state.SyncToken = p.Value
				}
			}
		}

		var objects []caldav.EventObject
		if syncToken != "" {
			objects, err = cal.QueryChanges(ctx, caldav.ChangesQuery{
				SyncToken: syncToken,
			})
		} else {
			objects, err = cal.QueryEvents(ctx, caldav.EventQuery{
				TimeRange: &caldav.TimeRange{Start: time.Now().Add(-365 * 24 * time.Hour), End: time.Now().Add(365 * 24 * time.Hour)},
			})
		}
		if err != nil {
			return fmt.Errorf("query events: %w", err)
		}

		var buf bytes.Buffer
		encoder := ical.NewEncoder(&buf)
		calObj := ical.NewCalendar()
		for _, obj := range objects {
			calObj.Children = append(calObj.Children, obj.Data)
		}
		if err := encoder.Encode(calObj); err != nil {
			return fmt.Errorf("encode calendar: %w", err)
		}

		data = buf.Bytes()
		return nil
	})

	return data, &state, err
}

func (c *Client) CreateEvent(ctx context.Context, icsData []byte) (string, error) {
	if c.Username == "" || c.Password == "" {
		return "", fmt.Errorf("authentication required for create")
	}

	var href string

	err := retry.Do(ctx, c.retryConfig, func() error {
		calendars, err := c.caldavClient.FindCalendars(ctx, c.URL)
		if err != nil {
			return fmt.Errorf("find calendars: %w", err)
		}
		if len(calendars) == 0 {
			return fmt.Errorf("no calendars found")
		}

		cal := calendars[0]
		decoder := ical.NewDecoder(bytes.NewReader(icsData))
		comp, err := decoder.Decode()
		if err != nil {
			return fmt.Errorf("decode ics: %w", err)
		}

		for _, child := range comp.Children {
			if child.Name == "VEVENT" {
				uid := child.Props.Get("UID")
				if uid == "" {
					return fmt.Errorf("event missing UID")
				}

				var eventBuf bytes.Buffer
				eventCal := ical.NewCalendar()
				eventCal.Children = []*ical.Component{child}
				ical.NewEncoder(&eventBuf).Encode(eventCal)

				h, err := cal.CreateEvent(ctx, eventBuf.Bytes())
				if err != nil {
					return fmt.Errorf("create event: %w", err)
				}
				href = h
				return nil
			}
		}

		return fmt.Errorf("no VEVENT found in ics data")
	})

	return href, err
}

func (c *Client) UpdateEvent(ctx context.Context, href string, icsData []byte, etag string) error {
	if c.Username == "" || c.Password == "" {
		return fmt.Errorf("authentication required for update")
	}

	return retry.Do(ctx, c.retryConfig, func() error {
		decoder := ical.NewDecoder(bytes.NewReader(icsData))
		comp, err := decoder.Decode()
		if err != nil {
			return fmt.Errorf("decode ics: %w", err)
		}

		for _, child := range comp.Children {
			if child.Name == "VEVENT" {
				var eventBuf bytes.Buffer
				eventCal := ical.NewCalendar()
				eventCal.Children = []*ical.Component{child}
				ical.NewEncoder(&eventBuf).Encode(eventCal)

				return cal.UpdateEvent(ctx, href, eventBuf.Bytes(), etag)
			}
		}

		return fmt.Errorf("no VEVENT found in ics data")
	})
}

func (c *Client) DeleteEvent(ctx context.Context, href string, etag string) error {
	if c.Username == "" || c.Password == "" {
		return fmt.Errorf("authentication required for delete")
	}

	return retry.Do(ctx, c.retryConfig, func() error {
		calendars, err := c.caldavClient.FindCalendars(ctx, c.URL)
		if err != nil {
			return fmt.Errorf("find calendars: %w", err)
		}
		if len(calendars) == 0 {
			return fmt.Errorf("no calendars found")
		}

		return calendars[0].DeleteEvent(ctx, href, etag)
	})
}

func (c *Client) ListEvents(ctx context.Context) ([]EventRef, error) {
	if c.Username == "" && c.Password == "" {
		return nil, fmt.Errorf("authentication required for list")
	}

	var refs []EventRef

	err := retry.Do(ctx, c.retryConfig, func() error {
		calendars, err := c.caldavClient.FindCalendars(ctx, c.URL)
		if err != nil {
			return fmt.Errorf("find calendars: %w", err)
		}
		if len(calendars) == 0 {
			return fmt.Errorf("no calendars found")
		}

		cal := calendars[0]
		objects, err := cal.QueryEvents(ctx, caldav.EventQuery{
			TimeRange: &caldav.TimeRange{Start: time.Now().Add(-365 * 24 * time.Hour), End: time.Now().Add(365 * 24 * time.Hour)},
		})
		if err != nil {
			return fmt.Errorf("query events: %w", err)
		}

		refs = nil
		for _, obj := range objects {
			decoder := ical.NewDecoder(bytes.NewReader(obj.Data))
			comp, _ := decoder.Decode()
			uid := ""
			for _, child := range comp.Children {
				if child.Name == "VEVENT" {
					uid = child.Props.Get("UID")
					break
				}
			}
			refs = append(refs, EventRef{
				Href: obj.Path,
				ETag: obj.ETag,
				UID:  uid,
			})
		}

		return nil
	})

	return refs, err
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