package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Sources      []SourceConfig      `mapstructure:"sources" yaml:"sources"`
	Destinations []DestinationConfig `mapstructure:"destinations" yaml:"destinations"`
	Database     DatabaseConfig      `mapstructure:"database" yaml:"database"`
	Sync         SyncConfig          `mapstructure:"sync" yaml:"sync"`
	Metrics      MetricsConfig       `mapstructure:"metrics" yaml:"metrics"`
	Web          WebConfig           `mapstructure:"web" yaml:"web"`
	Logging      LoggingConfig       `mapstructure:"logging" yaml:"logging"`
}

// TransformerConfig describes an event transformer in the pipeline.
type TransformerConfig struct {
	Type    string            `mapstructure:"type" yaml:"type"`
	Options map[string]string `mapstructure:"options" yaml:"options"`
}

// SourceConfig describes a calendar source (public .ics or authenticated CalDAV).
// Each source has a unique name used as reference from destinations.
type SourceConfig struct {
	Name         string              `mapstructure:"name" yaml:"name"`
	Type         string              `mapstructure:"type" yaml:"type"`
	URL          string              `mapstructure:"url" yaml:"url"`
	Username     string              `mapstructure:"username" yaml:"username"`
	Password     string              `mapstructure:"password" yaml:"password"`
	Transformers []TransformerConfig `mapstructure:"transformers" yaml:"transformers"`
	// Deprecated: legacy single-connection format where destination was nested inside source.
	Destination DestinationConfig `mapstructure:"destination" yaml:"destination,omitempty"`
}

type DestinationConfig struct {
	Name         string              `mapstructure:"name" yaml:"name"`
	Type         string              `mapstructure:"type" yaml:"type"`
	URL          string              `mapstructure:"url" yaml:"url"`
	Username     string              `mapstructure:"username" yaml:"username"`
	Password     string              `mapstructure:"password" yaml:"password"`
	Source       string              `mapstructure:"source" yaml:"source"`
	Transformers []TransformerConfig `mapstructure:"transformers" yaml:"transformers"`
}

type DatabaseConfig struct {
	Path string `mapstructure:"path" yaml:"path"`
}

type SyncConfig struct {
	Interval      string `mapstructure:"interval" yaml:"interval"`
	Timeout       string `mapstructure:"timeout" yaml:"timeout"`
	BatchSize     int    `mapstructure:"batch_size" yaml:"batch_size"`
	DeleteMode    string `mapstructure:"delete_mode" yaml:"delete_mode"`
	FilterPrivate bool   `mapstructure:"filter_private" yaml:"filter_private"`
}

func (s *SyncConfig) IntervalDuration() time.Duration {
	d, _ := time.ParseDuration(s.Interval)
	return d
}

func (s *SyncConfig) TimeoutDuration() time.Duration {
	d, _ := time.ParseDuration(s.Timeout)
	return d
}

type MetricsConfig struct {
	Addr string `mapstructure:"addr" yaml:"addr"`
}

type WebConfig struct {
	Addr  string `mapstructure:"addr" yaml:"addr"`
	Token string `mapstructure:"token" yaml:"token"`
}

type LoggingConfig struct {
	Level  string `mapstructure:"level" yaml:"level"`
	Format string `mapstructure:"format" yaml:"format"`
}

func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	setDefaults(&cfg)
	migrateLegacy(&cfg)

	if err := Validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func setDefaults(cfg *Config) {
	if cfg.Sync.BatchSize == 0 {
		cfg.Sync.BatchSize = 100
	}
	if cfg.Sync.DeleteMode == "" {
		cfg.Sync.DeleteMode = "soft"
	}
	if cfg.Metrics.Addr == "" {
		cfg.Metrics.Addr = ":8080"
	}
	if cfg.Web.Addr == "" {
		cfg.Web.Addr = cfg.Metrics.Addr
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = "json"
	}
	// Auto-name sources/destinations if name missing (for backward compat)
	for i := range cfg.Sources {
		if cfg.Sources[i].Name == "" && cfg.Sources[i].URL != "" {
			cfg.Sources[i].Name = fmt.Sprintf("source-%d", i+1)
		}
		if cfg.Sources[i].Type == "" {
			cfg.Sources[i].Type = "caldav"
		}
		// Migrate legacy filter_private to transformer (kept for compat, sync layer also checks FilterPrivate)
	}
	for i := range cfg.Destinations {
		if cfg.Destinations[i].Name == "" && cfg.Destinations[i].URL != "" {
			cfg.Destinations[i].Name = fmt.Sprintf("dest-%d", i+1)
		}
		if cfg.Destinations[i].Type == "" {
			cfg.Destinations[i].Type = "caldav"
		}
	}
}

// migrateLegacy converts old config where destination was nested inside source (sources[].destination)
// into the new separated format (sources[] + destinations[] with destination.source reference).
func migrateLegacy(cfg *Config) {
	if len(cfg.Destinations) > 0 {
		// New format already used, just clear any legacy nested destinations to avoid confusion.
		for i := range cfg.Sources {
			cfg.Sources[i].Destination = DestinationConfig{}
		}
		return
	}
	hasLegacy := false
	for _, s := range cfg.Sources {
		if s.Destination.Name != "" || s.Destination.URL != "" {
			hasLegacy = true
			break
		}
	}
	if !hasLegacy {
		return
	}
	// Ensure sources have names before creating destinations
	for i := range cfg.Sources {
		if cfg.Sources[i].Name == "" {
			cfg.Sources[i].Name = fmt.Sprintf("source-%d", i+1)
		}
	}
	var dests []DestinationConfig
	for _, s := range cfg.Sources {
		if s.Destination.Name == "" && s.Destination.URL == "" {
			continue
		}
		d := s.Destination
		d.Source = s.Name
		dests = append(dests, d)
	}
	cfg.Destinations = dests
	for i := range cfg.Sources {
		cfg.Sources[i].Destination = DestinationConfig{}
	}
}

func Validate(cfg *Config) error {
	if len(cfg.Sources) == 0 {
		return fmt.Errorf("at least one source is required")
	}
	sourceNames := make(map[string]int)
	for i, s := range cfg.Sources {
		if s.Name == "" {
			return fmt.Errorf("source[%d].name is required", i)
		}
		if _, ok := sourceNames[s.Name]; ok {
			return fmt.Errorf("source name %q duplicated (source[%d])", s.Name, i)
		}
		sourceNames[s.Name] = i
		if s.URL == "" {
			return fmt.Errorf("source[%d].url is required", i)
		}
		if s.Type == "" {
			return fmt.Errorf("source[%d].type is required", i)
		}
		for ti, tr := range s.Transformers {
			if tr.Type == "" {
				return fmt.Errorf("source[%d].transformers[%d].type is required", i, ti)
			}
		}
	}
	if len(cfg.Destinations) == 0 {
		return fmt.Errorf("at least one destination is required")
	}
	destNames := make(map[string]bool)
	for i, d := range cfg.Destinations {
		if d.Name == "" {
			return fmt.Errorf("destination[%d].name is required", i)
		}
		if destNames[d.Name] {
			return fmt.Errorf("destination name %q duplicated (destination[%d])", d.Name, i)
		}
		destNames[d.Name] = true
		if d.URL == "" {
			return fmt.Errorf("destination[%d].url is required", i)
		}
		if d.Type == "" {
			return fmt.Errorf("destination[%d].type is required", i)
		}
		if d.Username == "" || d.Password == "" {
			return fmt.Errorf("destination[%d] requires username and password (token)", i)
		}
		if d.Source == "" {
			return fmt.Errorf("destination[%d].source is required (must reference a source name)", i)
		}
		if _, ok := sourceNames[d.Source]; !ok {
			return fmt.Errorf("destination[%d].source %q does not match any source name", i, d.Source)
		}
		for ti, tr := range d.Transformers {
			if tr.Type == "" {
				return fmt.Errorf("destination[%d].transformers[%d].type is required", i, ti)
			}
		}
	}
	if cfg.Database.Path == "" {
		return fmt.Errorf("database.path is required")
	}
	if cfg.Sync.Interval != "" {
		if _, err := time.ParseDuration(cfg.Sync.Interval); err != nil {
			return fmt.Errorf("sync.interval: %w", err)
		}
	}
	if cfg.Sync.Timeout != "" {
		if _, err := time.ParseDuration(cfg.Sync.Timeout); err != nil {
			return fmt.Errorf("sync.timeout: %w", err)
		}
	}
	if cfg.Sync.DeleteMode != "" && cfg.Sync.DeleteMode != "soft" && cfg.Sync.DeleteMode != "hard" {
		return fmt.Errorf("sync.delete_mode must be 'soft' or 'hard'")
	}
	return nil
}
