package caldav

type Client struct {
	URL      string
	Username string
	Password string
}

func NewClient(url, username, password string) *Client {
	return &Client{
		URL:      url,
		Username: username,
		Password: password,
	}
}

func (c *Client) FetchCalendar(ctx context.Context) ([]byte, string, error) {
	// TODO: Implement CalDAV GET with ETag
	return nil, "", nil
}

func (c *Client) CreateEvent(ctx context.Context, icsData []byte) (string, error) {
	// TODO: Implement CalDAV PUT for new event
	return "", nil
}

func (c *Client) UpdateEvent(ctx context.Context, href string, icsData []byte, etag string) error {
	// TODO: Implement CalDAV PUT with If-Match
	return nil
}

func (c *Client) DeleteEvent(ctx context.Context, href string, etag string) error {
	// TODO: Implement CalDAV DELETE
	return nil
}

func (c *Client) ListEvents(ctx context.Context) ([]EventRef, error) {
	// TODO: Implement CalDAV REPORT calendar-query
	return nil, nil
}

type EventRef struct {
	Href string
	ETag string
	UID  string
}