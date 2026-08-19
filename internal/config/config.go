package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Source       SourceConfig       `mapstructure:"source"`
	Destinations []DestinationConfig `mapstructure:"destinations"`
	Database     DatabaseConfig     `mapstructure:"database"`
	Sync         SyncConfig         `mapstructure:"sync"`
	Metrics      MetricsConfig      `mapstructure:"metrics"`
	Logging      LoggingConfig      `mapstructure:"logging"`
}

type SourceConfig struct {
	URL      string `mapstructure:"url"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

type DestinationConfig struct {
	Name     string `mapstructure:"name"`
	URL      string `mapstructure:"url"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

type DatabaseConfig struct {
	Path string `mapstructure:"path"`
}

type SyncConfig struct {
	Interval      string `mapstructure:"interval"`
	Timeout       string `mapstructure:"timeout"`
	BatchSize     int    `mapstructure:"batch_size"`
	DeleteMode    string `mapstructure:"delete_mode"`
	FilterPrivate bool   `mapstructure:"filter_private"`

	intervalDur time.Duration
	timeoutDur  time.Duration
}

func (s *SyncConfig) IntervalDuration() time.Duration {
	if s.intervalDur == 0 && s.Interval != "" {
		d, _ := time.ParseDuration(s.Interval)
		s.intervalDur = d
	}
	return s.intervalDur
}

func (s *SyncConfig) TimeoutDuration() time.Duration {
	if s.timeoutDur == 0 && s.Timeout != "" {
		d, _ := time.ParseDuration(s.Timeout)
		s.timeoutDur = d
	}
	return s.timeoutDur
}

type MetricsConfig struct {
	Addr string `mapstructure:"addr"`
}

type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
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

	// Defaults
	if cfg.Sync.BatchSize == 0 {
		cfg.Sync.BatchSize = 100
	}
	if cfg.Sync.DeleteMode == "" {
		cfg.Sync.DeleteMode = "soft"
	}
	if cfg.Metrics.Addr == "" {
		cfg.Metrics.Addr = ":8080"
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = "json"
	}

	// Validation
	if cfg.Source.URL == "" {
		return nil, fmt.Errorf("source.url is required")
	}
	if len(cfg.Destinations) == 0 {
		return nil, fmt.Errorf("at least one destination is required")
	}
	for i, d := range cfg.Destinations {
		if d.Name == "" {
			return nil, fmt.Errorf("destination[%d].name is required", i)
		}
		if d.URL == "" {
			return nil, fmt.Errorf("destination[%d].url is required", i)
		}
		if d.Username == "" || d.Password == "" {
			return nil, fmt.Errorf("destination[%d] requires username and password (token)", i)
		}
	}
	if cfg.Database.Path == "" {
		return nil, fmt.Errorf("database.path is required")
	}

	return &cfg, nil
}