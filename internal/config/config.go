package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Sources  []SourceConfig `mapstructure:"sources" yaml:"sources"`
	Database DatabaseConfig `mapstructure:"database" yaml:"database"`
	Sync     SyncConfig     `mapstructure:"sync" yaml:"sync"`
	Metrics  MetricsConfig  `mapstructure:"metrics" yaml:"metrics"`
	Web      WebConfig      `mapstructure:"web" yaml:"web"`
	Logging  LoggingConfig  `mapstructure:"logging" yaml:"logging"`
}

// SourceConfig describes one connection. Each source is paired 1:1 with its own
// destination calendar.
type SourceConfig struct {
	URL         string            `mapstructure:"url" yaml:"url"`
	Username    string            `mapstructure:"username" yaml:"username"`
	Password    string            `mapstructure:"password" yaml:"password"`
	Destination DestinationConfig `mapstructure:"destination" yaml:"destination"`
}

type DestinationConfig struct {
	Name     string `mapstructure:"name" yaml:"name"`
	URL      string `mapstructure:"url" yaml:"url"`
	Username string `mapstructure:"username" yaml:"username"`
	Password string `mapstructure:"password" yaml:"password"`
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
}

func Validate(cfg *Config) error {
	if len(cfg.Sources) == 0 {
		return fmt.Errorf("at least one source is required")
	}
	for i, s := range cfg.Sources {
		if s.URL == "" {
			return fmt.Errorf("source[%d].url is required", i)
		}
		if s.Destination.Name == "" {
			return fmt.Errorf("source[%d].destination.name is required", i)
		}
		if s.Destination.URL == "" {
			return fmt.Errorf("source[%d].destination.url is required", i)
		}
		if s.Destination.Username == "" || s.Destination.Password == "" {
			return fmt.Errorf("source[%d].destination requires username and password (token)", i)
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
