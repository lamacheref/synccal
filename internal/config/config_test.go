package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTempYAML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}

func TestLoad_Valid(t *testing.T) {
	yaml := `
sources:
  - name: src1
    type: caldav
    url: https://example.com/cal.ics
destinations:
  - name: dest1
    type: caldav
    url: https://dest.example.com/
    username: user
    password: secret
    source: src1
database:
  path: /tmp/db.sqlite
sync:
  interval: 1h
  timeout: 5m
  delete_mode: soft
  filter_private: true
metrics:
  addr: :9090
web:
  addr: :8080
  token: tok
logging:
  level: debug
  format: json
`
	path := writeTempYAML(t, yaml)
	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "src1", cfg.Sources[0].Name)
	assert.Equal(t, "caldav", cfg.Sources[0].Type)
	assert.Equal(t, "dest1", cfg.Destinations[0].Name)
	assert.Equal(t, "src1", cfg.Destinations[0].Source)
	assert.Equal(t, "/tmp/db.sqlite", cfg.Database.Path)
	assert.Equal(t, "1h", cfg.Sync.Interval)
	assert.Equal(t, ":9090", cfg.Metrics.Addr)
	assert.Equal(t, "tok", cfg.Web.Token)
}

func TestLoad_Defaults(t *testing.T) {
	yaml := `
sources:
  - name: src1
    url: https://example.com/cal.ics
destinations:
  - name: dest1
    url: https://dest.example.com/
    username: u
    password: p
    source: src1
database:
  path: /tmp/db.sqlite
`
	path := writeTempYAML(t, yaml)
	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, 100, cfg.Sync.BatchSize)
	assert.Equal(t, "soft", cfg.Sync.DeleteMode)
	assert.Equal(t, ":8080", cfg.Metrics.Addr)
	assert.Equal(t, ":8080", cfg.Web.Addr)
	assert.Equal(t, "info", cfg.Logging.Level)
	assert.Equal(t, "json", cfg.Logging.Format)
	assert.Equal(t, "caldav", cfg.Sources[0].Type)
	assert.Equal(t, "caldav", cfg.Destinations[0].Type)
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load("/nonexistent/path.yaml")
	assert.Error(t, err)
}

func TestLoad_InvalidYAML(t *testing.T) {
	path := writeTempYAML(t, "::: not yaml :::\n sources: [")
	_, err := Load(path)
	assert.Error(t, err)
}

func TestLoad_MigrateLegacy(t *testing.T) {
	yaml := `
sources:
  - url: https://example.com/cal.ics
    destination:
      name: dest1
      url: https://dest.example.com/
      username: u
      password: p
database:
  path: /tmp/db.sqlite
`
	path := writeTempYAML(t, yaml)
	cfg, err := Load(path)
	require.NoError(t, err)
	require.Len(t, cfg.Sources, 1)
	require.Len(t, cfg.Destinations, 1)
	assert.Equal(t, "source-1", cfg.Sources[0].Name)
	assert.Equal(t, "dest1", cfg.Destinations[0].Name)
	assert.Equal(t, "source-1", cfg.Destinations[0].Source)
	assert.Empty(t, cfg.Sources[0].Destination.Name)
}

func TestValidate_MissingSource(t *testing.T) {
	cfg := &Config{
		Sources: []SourceConfig{},
		Destinations: []DestinationConfig{{Name: "d1", URL: "https://x", Username: "u", Password: "p", Source: "src1", Type: "caldav"}},
		Database: DatabaseConfig{Path: "/tmp/db"},
	}
	assert.ErrorContains(t, Validate(cfg), "at least one source")
}

func TestValidate_DuplicateSourceName(t *testing.T) {
	cfg := &Config{
		Sources: []SourceConfig{{Name: "src1", URL: "https://a", Type: "caldav"}, {Name: "src1", URL: "https://b", Type: "caldav"}},
		Destinations: []DestinationConfig{{Name: "d1", Type: "caldav", URL: "https://x", Username: "u", Password: "p", Source: "src1"}},
		Database: DatabaseConfig{Path: "/tmp/db"},
	}
	assert.ErrorContains(t, Validate(cfg), "duplicated")
}

func TestValidate_MissingSourceURL(t *testing.T) {
	cfg := &Config{
		Sources: []SourceConfig{{Name: "src1", Type: "caldav"}},
		Destinations: []DestinationConfig{{Name: "d1", Type: "caldav", URL: "https://x", Username: "u", Password: "p", Source: "src1"}},
		Database: DatabaseConfig{Path: "/tmp/db"},
	}
	assert.ErrorContains(t, Validate(cfg), "source[0].url")
}

func TestValidate_MissingSourceName(t *testing.T) {
	cfg := &Config{
		Sources: []SourceConfig{{URL: "https://a", Type: "caldav"}},
		Destinations: []DestinationConfig{{Name: "d1", Type: "caldav", URL: "https://x", Username: "u", Password: "p", Source: ""}},
		Database: DatabaseConfig{Path: "/tmp/db"},
	}
	assert.ErrorContains(t, Validate(cfg), "source[0].name")
}

func TestValidate_MissingSourceType(t *testing.T) {
	cfg := &Config{
		Sources: []SourceConfig{{Name: "src1", URL: "https://a"}},
		Destinations: []DestinationConfig{{Name: "d1", Type: "caldav", URL: "https://x", Username: "u", Password: "p", Source: "src1"}},
		Database: DatabaseConfig{Path: "/tmp/db"},
	}
	assert.ErrorContains(t, Validate(cfg), "source[0].type")
}

func TestValidate_TransformerTypeRequired(t *testing.T) {
	cfg := &Config{
		Sources: []SourceConfig{{Name: "src1", Type: "caldav", URL: "https://a", Transformers: []TransformerConfig{{Type: ""}}}},
		Destinations: []DestinationConfig{{Name: "d1", Type: "caldav", URL: "https://x", Username: "u", Password: "p", Source: "src1"}},
		Database: DatabaseConfig{Path: "/tmp/db"},
	}
	assert.ErrorContains(t, Validate(cfg), "transformers[0].type")
}

func TestValidate_MissingDestination(t *testing.T) {
	cfg := &Config{
		Sources: []SourceConfig{{Name: "src1", Type: "caldav", URL: "https://a"}},
		Destinations: []DestinationConfig{},
		Database: DatabaseConfig{Path: "/tmp/db"},
	}
	assert.ErrorContains(t, Validate(cfg), "at least one destination")
}

func TestValidate_DuplicateDest(t *testing.T) {
	cfg := &Config{
		Sources: []SourceConfig{{Name: "src1", Type: "caldav", URL: "https://a"}},
		Destinations: []DestinationConfig{
			{Name: "d1", Type: "caldav", URL: "https://x", Username: "u", Password: "p", Source: "src1"},
			{Name: "d1", Type: "caldav", URL: "https://y", Username: "u", Password: "p", Source: "src1"},
		},
		Database: DatabaseConfig{Path: "/tmp/db"},
	}
	assert.ErrorContains(t, Validate(cfg), "duplicated")
}

func TestValidate_MissingDestURL(t *testing.T) {
	cfg := &Config{
		Sources: []SourceConfig{{Name: "src1", Type: "caldav", URL: "https://a"}},
		Destinations: []DestinationConfig{{Name: "d1", Type: "caldav", Username: "u", Password: "p", Source: "src1"}},
		Database: DatabaseConfig{Path: "/tmp/db"},
	}
	assert.ErrorContains(t, Validate(cfg), "destination[0].url")
}

func TestValidate_MissingDestCreds(t *testing.T) {
	cfg := &Config{
		Sources: []SourceConfig{{Name: "src1", Type: "caldav", URL: "https://a"}},
		Destinations: []DestinationConfig{{Name: "d1", Type: "caldav", URL: "https://x", Source: "src1"}},
		Database: DatabaseConfig{Path: "/tmp/db"},
	}
	assert.ErrorContains(t, Validate(cfg), "requires username and password")
}

func TestValidate_MissingDestSource(t *testing.T) {
	cfg := &Config{
		Sources: []SourceConfig{{Name: "src1", Type: "caldav", URL: "https://a"}},
		Destinations: []DestinationConfig{{Name: "d1", Type: "caldav", URL: "https://x", Username: "u", Password: "p"}},
		Database: DatabaseConfig{Path: "/tmp/db"},
	}
	assert.ErrorContains(t, Validate(cfg), "destination[0].source is required")
}

func TestValidate_InvalidDestSource(t *testing.T) {
	cfg := &Config{
		Sources: []SourceConfig{{Name: "src1", Type: "caldav", URL: "https://a"}},
		Destinations: []DestinationConfig{{Name: "d1", Type: "caldav", URL: "https://x", Username: "u", Password: "p", Source: "unknown"}},
		Database: DatabaseConfig{Path: "/tmp/db"},
	}
	assert.ErrorContains(t, Validate(cfg), "does not match any source name")
}

func TestValidate_DestTransformerType(t *testing.T) {
	cfg := &Config{
		Sources: []SourceConfig{{Name: "src1", Type: "caldav", URL: "https://a"}},
		Destinations: []DestinationConfig{{Name: "d1", Type: "caldav", URL: "https://x", Username: "u", Password: "p", Source: "src1", Transformers: []TransformerConfig{{Type: ""}}}},
		Database: DatabaseConfig{Path: "/tmp/db"},
	}
	assert.ErrorContains(t, Validate(cfg), "transformers[0].type")
}

func TestValidate_MissingDB(t *testing.T) {
	cfg := &Config{
		Sources: []SourceConfig{{Name: "src1", Type: "caldav", URL: "https://a"}},
		Destinations: []DestinationConfig{{Name: "d1", Type: "caldav", URL: "https://x", Username: "u", Password: "p", Source: "src1"}},
	}
	assert.ErrorContains(t, Validate(cfg), "database.path")
}

func TestValidate_InvalidInterval(t *testing.T) {
	cfg := &Config{
		Sources: []SourceConfig{{Name: "src1", Type: "caldav", URL: "https://a"}},
		Destinations: []DestinationConfig{{Name: "d1", Type: "caldav", URL: "https://x", Username: "u", Password: "p", Source: "src1"}},
		Database: DatabaseConfig{Path: "/tmp/db"},
		Sync: SyncConfig{Interval: "notaduration"},
	}
	assert.ErrorContains(t, Validate(cfg), "sync.interval")
}

func TestValidate_InvalidTimeout(t *testing.T) {
	cfg := &Config{
		Sources: []SourceConfig{{Name: "src1", Type: "caldav", URL: "https://a"}},
		Destinations: []DestinationConfig{{Name: "d1", Type: "caldav", URL: "https://x", Username: "u", Password: "p", Source: "src1"}},
		Database: DatabaseConfig{Path: "/tmp/db"},
		Sync: SyncConfig{Timeout: "bad"},
	}
	assert.ErrorContains(t, Validate(cfg), "sync.timeout")
}

func TestValidate_InvalidDeleteMode(t *testing.T) {
	cfg := &Config{
		Sources: []SourceConfig{{Name: "src1", Type: "caldav", URL: "https://a"}},
		Destinations: []DestinationConfig{{Name: "d1", Type: "caldav", URL: "https://x", Username: "u", Password: "p", Source: "src1"}},
		Database: DatabaseConfig{Path: "/tmp/db"},
		Sync: SyncConfig{DeleteMode: "invalid"},
	}
	assert.ErrorContains(t, Validate(cfg), "sync.delete_mode")
}

func TestIntervalDuration(t *testing.T) {
	s := SyncConfig{Interval: "1h"}
	assert.Equal(t, s.IntervalDuration().String(), "1h0m0s")
	s2 := SyncConfig{Timeout: "30s"}
	assert.Equal(t, s2.TimeoutDuration().String(), "30s")
}

func TestMigrateLegacy_NoDestinations(t *testing.T) {
	cfg := &Config{
		Sources: []SourceConfig{{Name: "src1", Type: "caldav", URL: "https://a"}},
		Destinations: []DestinationConfig{},
		Database: DatabaseConfig{Path: "/tmp/db"},
	}
	migrateLegacy(cfg)
	assert.Empty(t, cfg.Destinations)
}

func TestMigrateLegacy_WithDestinationsClearsNested(t *testing.T) {
	cfg := &Config{
		Sources: []SourceConfig{{Name: "src1", Type: "caldav", URL: "https://a", Destination: DestinationConfig{Name: "d1", URL: "https://x"}}},
		Destinations: []DestinationConfig{{Name: "d1", Type: "caldav", URL: "https://x", Username: "u", Password: "p", Source: "src1"}},
	}
	migrateLegacy(cfg)
	assert.Empty(t, cfg.Sources[0].Destination.Name)
}

func TestSetDefaults_AutoName(t *testing.T) {
	cfg := &Config{
		Sources: []SourceConfig{{URL: "https://a"}},
		Destinations: []DestinationConfig{{URL: "https://x"}},
		Database: DatabaseConfig{Path: "/tmp/db"},
	}
	setDefaults(cfg)
	assert.Equal(t, "source-1", cfg.Sources[0].Name)
	assert.Equal(t, "dest-1", cfg.Destinations[0].Name)
}
