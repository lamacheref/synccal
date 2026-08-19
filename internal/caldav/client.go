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
)

type Client struct {
	URL      string
	Username string
	Password string
	httpClient *http.Client
	caldavClient *caldav.Client
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
		URL:           rawURL,
		Username:      username,
		Password:      password,
		httpClient:    httpClient,
		caldavClient:  caldavClient,
	}, nil
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

func (c *Client) FetchCalendar(ctx context.Context) ([]byte, string, error) {
	if c.Username == "" && c.Password == "" {
		return c.fetchPublicICS(ctx)
	}
	return c.fetchCalDAVCalendar(ctx)
}

func (c *Client) fetchPublicICS(ctx context.Context) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.URL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetch public ICS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read response: %w", err)
	}

	etag := resp.Header.Get("ETag")
	return data, strings.Trim(etag, "\""), nil
}

func (c *Client) fetchCalDAVCalendar(ctx context.Context) ([]byte, string, error) {
	calendars, err := c.caldavClient.FindCalendars(ctx, c.URL)
	if err != nil {
		return nil, "", fmt.Errorf("find calendars: %w", err)
	}

	if len(calendars) == 0 {
		return nil, "", fmt.Errorf("no calendars found at %s", c.URL)
	}

	cal := calendars[0]
	objects, err := cal.QueryEvents(ctx, caldav.EventQuery{
		TimeRange: &caldav.TimeRange{Start: time.Now().Add(-365 * 24 * time.Hour), End: time.Now().Add(365 * 24 * time.Hour)},
	})
	if err != nil {
		return nil, "", fmt.Errorf("query events: %w", err)
	}

	var buf bytes.Buffer
	encoder := ical.NewEncoder(&buf)
	calObj := ical.NewCalendar()
	for _, obj := range objects {
		calObj.Children = append(calObj.Children, obj.Data)
	}
	if err := encoder.Encode(calObj); err != nil {
		return nil, "", fmt.Errorf("encode calendar: %w", err)
	}

	return buf.Bytes(), "", nil
}

func (c *Client) CreateEvent(ctx context.Context, icsData []byte) (string, error) {
	if c.Username == "" || c.Password == "" {
		return "", fmt.Errorf("authentication required for create")
	}

	calendars, err := c.caldavClient.FindCalendars(ctx, c.URL)
	if err != nil {
		return "", fmt.Errorf("find calendars: %w", err)
	}
	if len(calendars) == 0 {
		return "", fmt.Errorf("no calendars found")
	}

	cal := calendars[0]
	decoder := ical.NewDecoder(bytes.NewReader(icsData))
	comp, err := decoder.Decode()
	if err != nil {
		return "", fmt.Errorf("decode ics: %w", err)
	}

	for _, child := range comp.Children {
		if child.Name == "VEVENT" {
			uid := child.Props.Get("UID")
			if uid == "" {
				return "", fmt.Errorf("event missing UID")
			}

			var eventBuf bytes.Buffer
			eventCal := ical.NewCalendar()
			eventCal.Children = []*ical.Component{child}
			ical.NewEncoder(&eventBuf).Encode(eventCal)

			href, err := cal.CreateEvent(ctx, eventBuf.Bytes())
			if err != nil {
				return "", fmt.Errorf("create event: %w", err)
			}
			return href, nil
		}
	}

	return "", fmt.Errorf("no VEVENT found in ics data")
}

func (c *Client) UpdateEvent(ctx context.Context, href string, icsData []byte, etag string) error {
	if c.Username == "" || c.Password == "" {
		return fmt.Errorf("authentication required for update")
	}

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
}

func (c *Client) DeleteEvent(ctx context.Context, href string, etag string) error {
	if c.Username == "" || c.Password == "" {
		return fmt.Errorf("authentication required for delete")
	}

	calendars, err := c.caldavClient.FindCalendars(ctx, c.URL)
	if err != nil {
		return fmt.Errorf("find calendars: %w", err)
	}
	if len(calendars) == 0 {
		return fmt.Errorf("no calendars found")
	}

	return calendars[0].DeleteEvent(ctx, href, etag)
}

func (c *Client) ListEvents(ctx context.Context) ([]EventRef, error) {
	if c.Username == "" && c.Password == "" {
		return nil, fmt.Errorf("authentication required for list")
	}

	calendars, err := c.caldavClient.FindCalendars(ctx, c.URL)
	if err != nil {
		return nil, fmt.Errorf("find calendars: %w", err)
	}
	if len(calendars) == 0 {
		return nil, fmt.Errorf("no calendars found")
	}

	cal := calendars[0]
	objects, err := cal.QueryEvents(ctx, caldav.EventQuery{
		TimeRange: &caldav.TimeRange{Start: time.Now().Add(-365 * 24 * time.Hour), End: time.Now().Add(365 * 24 * time.Hour)},
	})
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}

	var refs []EventRef
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

	return refs, nil
}

type EventRef struct {
	Href string
	ETag string
	UID  string
}