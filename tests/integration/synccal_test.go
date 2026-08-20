package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/smiden/synccal/internal/caldav"
	"github.com/smiden/synccal/internal/config"
	"github.com/smiden/synccal/internal/storage"
	"github.com/smiden/synccal/internal/sync"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	testTimeout = 5 * time.Minute
)

func TestMain(m *testing.M) {
	if os.Getenv("INTEGRATION_TESTS") != "1" {
		fmt.Println("Skipping integration tests (set INTEGRATION_TESTS=1 to run)")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func newTestLogger() *zap.SugaredLogger {
	cfg := zap.NewDevelopmentConfig()
	cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	logger, _ := cfg.Build()
	return logger.Sugar()
}

func TestSyncCal_NextcloudToNextcloud(t *testing.T) {
	ctx := context.Background()
	log := newTestLogger()

	srcContainer, err := startNextcloud(ctx, "source")
	require.NoError(t, err)
	defer func() {
		if err := srcContainer.Terminate(ctx); err != nil {
			log.Errorw("Failed to terminate source container", "error", err)
		}
	}()
	require.NoError(t, ensurePersonalCalendar(ctx, srcContainer))

	destContainer, err := startNextcloud(ctx, "dest")
	require.NoError(t, err)
	defer func() {
		if err := destContainer.Terminate(ctx); err != nil {
			log.Errorw("Failed to terminate dest container", "error", err)
		}
	}()
	require.NoError(t, ensurePersonalCalendar(ctx, destContainer))

	srcURL, err := getCalDAVURL(ctx, srcContainer, "source")
	require.NoError(t, err)

	destURL, err := getCalDAVURL(ctx, destContainer, "dest")
	require.NoError(t, err)

	dbPath := filepath.Join(t.TempDir(), "synccal.db")
	store, err := storage.New(dbPath)
	require.NoError(t, err)
	defer store.Close()

	cfg := &config.Config{
		Source: config.SourceConfig{
			URL:      srcURL,
			Username: "admin",
			Password: "admin",
		},
		Destinations: []config.DestinationConfig{
			{
				Name:     "nextcloud-dest",
				URL:      destURL,
				Username: "admin",
				Password: "admin",
			},
		},
		Database: config.DatabaseConfig{Path: dbPath},
		Sync: config.SyncConfig{
			Interval:      "0",
			Timeout:       "2m",
			BatchSize:     100,
			DeleteMode:    "soft",
			FilterPrivate: true,
		},
		Metrics: config.MetricsConfig{Addr: ":0"},
		Logging: config.LoggingConfig{Level: "debug", Format: "console"},
	}

	sourceClient, err := caldav.NewClient(cfg.Source.URL, cfg.Source.Username, cfg.Source.Password)
	require.NoError(t, err)

	destClient, err := caldav.NewClient(cfg.Destinations[0].URL, cfg.Destinations[0].Username, cfg.Destinations[0].Password)
	require.NoError(t, err)

	syncer := sync.New(cfg, sourceClient, []*caldav.Client{destClient}, store, log)

	syncCtx, cancel := context.WithTimeout(ctx, testTimeout)
	defer cancel()

	_, err = sourceClient.CreateEvent(syncCtx, []byte(`BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//SyncCal Test//EN
BEGIN:VEVENT
UID:first-event@example.com
DTSTAMP:20240101T000000Z
DTSTART:20240115T100000Z
DTEND:20240115T110000Z
SUMMARY:First event
END:VEVENT
END:VCALENDAR`))
	require.NoError(t, err)

	err = syncer.Sync(syncCtx)
	require.NoError(t, err)

	mappings, err := store.ListMappings("nextcloud-dest")
	require.NoError(t, err)
	require.Greater(t, len(mappings), 0, "should have synced events")

	sourceState, err := store.GetSourceState(cfg.Source.URL)
	require.NoError(t, err)
	require.NotEmpty(t, sourceState.CTag)

	_, err = sourceClient.CreateEvent(syncCtx, []byte(`BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//SyncCal Test//EN
BEGIN:VEVENT
UID:second-event@example.com
DTSTAMP:20240101T000000Z
DTSTART:20240116T100000Z
DTEND:20240116T110000Z
SUMMARY:Second event
END:VEVENT
END:VCALENDAR`))
	require.NoError(t, err)

	err = syncer.Sync(syncCtx)
	require.NoError(t, err)

	mappings2, err := store.ListMappings("nextcloud-dest")
	require.NoError(t, err)
	require.Greater(t, len(mappings2), len(mappings), "second sync should pick up new events")
	require.Equal(t, len(mappings)+1, len(mappings2), "second sync should not create duplicates")
}

func startNextcloud(ctx context.Context, name string) (testcontainers.Container, error) {
	return testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "nextcloud:28-apache",
			ExposedPorts: []string{"80/tcp"},
			Env: map[string]string{
				"NEXTCLOUD_ADMIN_USER":     "admin",
				"NEXTCLOUD_ADMIN_PASSWORD": "admin",
				"SQLITE_DATABASE":          "synccal",
			},
			WaitingFor: wait.ForHTTP("/status.php").
				WithStartupTimeout(120 * time.Second).
				WithPollInterval(2 * time.Second),
		},
		Started: true,
	})
}

// ensurePersonalCalendar creates the default "personal" calendar for the admin
// user, which is not provisioned by the automatic install.
func ensurePersonalCalendar(ctx context.Context, c testcontainers.Container) error {
	_, _, err := c.Exec(ctx,
		[]string{"su", "www-data", "-s", "/bin/sh", "-c", "php occ dav:create-calendar admin personal"},
	)
	return err
}

func getCalDAVURL(ctx context.Context, c testcontainers.Container, name string) (string, error) {
	host, err := c.Host(ctx)
	if err != nil {
		return "", err
	}
	port, err := c.MappedPort(ctx, "80/tcp")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("http://%s:%s/remote.php/dav/calendars/admin/personal/", host, port.Port()), nil
}

func TestSyncCal_PublicICSToNextcloud(t *testing.T) {
	ctx := context.Background()
	log := newTestLogger()

	destContainer, err := startNextcloud(ctx, "dest-public")
	require.NoError(t, err)
	defer func() {
		if err := destContainer.Terminate(ctx); err != nil {
			log.Errorw("Failed to terminate dest container", "error", err)
		}
	}()
	require.NoError(t, ensurePersonalCalendar(ctx, destContainer))

	destURL, err := getCalDAVURL(ctx, destContainer, "dest-public")
	require.NoError(t, err)

	publicICS := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Test//Test//EN
BEGIN:VEVENT
UID:test-event-1@example.com
DTSTAMP:20240101T000000Z
DTSTART:20240115T100000Z
DTEND:20240115T110000Z
SUMMARY:Test Event 1
DESCRIPTION:Public event
END:VEVENT
BEGIN:VEVENT
UID:test-event-2@example.com
DTSTAMP:20240101T000000Z
DTSTART:20240116T100000Z
DTEND:20240116T110000Z
SUMMARY:Private Event
CLASS:PRIVATE
DESCRIPTION:Should be filtered
END:VEVENT
END:VCALENDAR`

	icsContainer, err := startPublicICSServer(ctx, publicICS)
	require.NoError(t, err)
	defer func() {
		if err := icsContainer.Terminate(ctx); err != nil {
			log.Errorw("Failed to terminate ICS container", "error", err)
		}
	}()

	icsURL, err := getICSURL(ctx, icsContainer)
	require.NoError(t, err)

	dbPath := filepath.Join(t.TempDir(), "synccal.db")
	store, err := storage.New(dbPath)
	require.NoError(t, err)
	defer store.Close()

	cfg := &config.Config{
		Source: config.SourceConfig{
			URL: icsURL,
		},
		Destinations: []config.DestinationConfig{
			{
				Name:     "nextcloud-dest",
				URL:      destURL,
				Username: "admin",
				Password: "admin",
			},
		},
		Database: config.DatabaseConfig{Path: dbPath},
		Sync: config.SyncConfig{
			Interval:      "0",
			Timeout:       "2m",
			BatchSize:     100,
			DeleteMode:    "soft",
			FilterPrivate: true,
		},
		Metrics: config.MetricsConfig{Addr: ":0"},
		Logging: config.LoggingConfig{Level: "debug", Format: "console"},
	}

	sourceClient, err := caldav.NewClient(cfg.Source.URL, cfg.Source.Username, cfg.Source.Password)
	require.NoError(t, err)

	destClient, err := caldav.NewClient(cfg.Destinations[0].URL, cfg.Destinations[0].Username, cfg.Destinations[0].Password)
	require.NoError(t, err)

	syncer := sync.New(cfg, sourceClient, []*caldav.Client{destClient}, store, log)

	syncCtx, cancel := context.WithTimeout(ctx, testTimeout)
	defer cancel()

	err = syncer.Sync(syncCtx)
	require.NoError(t, err)

	mappings, err := store.ListMappings("nextcloud-dest")
	require.NoError(t, err)
	require.Equal(t, 1, len(mappings), "should only sync non-private event")

	for uid := range mappings {
		require.NotEqual(t, "test-event-2@example.com", uid, "private event should be filtered")
	}
}

func startPublicICSServer(ctx context.Context, icsContent string) (testcontainers.Container, error) {
	return testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "nginx:alpine",
			ExposedPorts: []string{"80/tcp"},
			Files: []testcontainers.ContainerFile{
				{
					HostFilePath:      createTempICSFile(icsContent),
					ContainerFilePath: "/usr/share/nginx/html/calendar.ics",
					FileMode:          0644,
				},
			},
			WaitingFor: wait.ForHTTP("/calendar.ics").WithStartupTimeout(30 * time.Second),
		},
		Started: true,
	})
}

func createTempICSFile(content string) string {
	tmpFile, err := os.CreateTemp("", "calendar-*.ics")
	if err != nil {
		panic(err)
	}
	defer tmpFile.Close()
	if _, err := tmpFile.WriteString(content); err != nil {
		panic(err)
	}
	return tmpFile.Name()
}

func getICSURL(ctx context.Context, c testcontainers.Container) (string, error) {
	host, err := c.Host(ctx)
	if err != nil {
		return "", err
	}
	port, err := c.MappedPort(ctx, "80/tcp")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("http://%s:%s/calendar.ics", host, port.Port()), nil
}
