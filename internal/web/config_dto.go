package web

import (
	"os"

	"github.com/smiden/synccal/internal/config"
	"gopkg.in/yaml.v3"
)

// Sanitized view returned by GET /api/config — passwords never leave the server.
type configView struct {
	Source       sourceView        `json:"source"`
	Destinations []destinationView `json:"destinations"`
	Database     databaseView      `json:"database"`
	Sync         syncView          `json:"sync"`
	Web          webView           `json:"web"`
	Logging      loggingView       `json:"logging"`
}

type sourceView struct {
	URL      string `json:"url"`
	Username string `json:"username"`
}

type destinationView struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Username string `json:"username"`
}

type databaseView struct {
	Path string `json:"path"`
}

type syncView struct {
	Interval      string `json:"interval"`
	Timeout       string `json:"timeout"`
	BatchSize     int    `json:"batch_size"`
	DeleteMode    string `json:"delete_mode"`
	FilterPrivate bool   `json:"filter_private"`
}

type webView struct {
	Addr     string `json:"addr"`
	TokenSet bool   `json:"token_set"`
}

type loggingView struct {
	Level  string `json:"level"`
	Format string `json:"format"`
}

// Update payload accepted by PUT /api/config. A nil/omitted password means
// "keep the current one" — passwords are write-only.
type configUpdate struct {
	Source       *sourceUpdate        `json:"source"`
	Destinations *[]destinationUpdate `json:"destinations"`
	Sync         *syncUpdate          `json:"sync"`
	Web          *webUpdate           `json:"web"`
	Logging      *loggingUpdate       `json:"logging"`
}

type sourceUpdate struct {
	URL      string  `json:"url"`
	Username string  `json:"username"`
	Password *string `json:"password"`
}

type destinationUpdate struct {
	Name     string  `json:"name"`
	URL      string  `json:"url"`
	Username string  `json:"username"`
	Password *string `json:"password"`
}

type syncUpdate struct {
	Interval      *string `json:"interval"`
	Timeout       *string `json:"timeout"`
	BatchSize     *int    `json:"batch_size"`
	DeleteMode    *string `json:"delete_mode"`
	FilterPrivate *bool   `json:"filter_private"`
}

type webUpdate struct {
	Addr  *string `json:"addr"`
	Token *string `json:"token"`
}

type loggingUpdate struct {
	Level  *string `json:"level"`
	Format *string `json:"format"`
}

func (s *Server) configView() configView {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	cfg := s.cfg

	cv := configView{
		Database: databaseView{Path: cfg.Database.Path},
		Sync: syncView{
			Interval:      cfg.Sync.Interval,
			Timeout:       cfg.Sync.Timeout,
			BatchSize:     cfg.Sync.BatchSize,
			DeleteMode:    cfg.Sync.DeleteMode,
			FilterPrivate: cfg.Sync.FilterPrivate,
		},
		Web:     webView{Addr: cfg.Web.Addr, TokenSet: cfg.Web.Token != ""},
		Logging: loggingView{Level: cfg.Logging.Level, Format: cfg.Logging.Format},
	}
	cv.Source = sourceView{URL: cfg.Source.URL, Username: cfg.Source.Username}
	for _, d := range cfg.Destinations {
		cv.Destinations = append(cv.Destinations, destinationView{Name: d.Name, URL: d.URL, Username: d.Username})
	}
	return cv
}

// mergeConfigUpdate applies the update onto the current config in place.
func mergeConfigUpdate(cfg *config.Config, upd *configUpdate) {
	if upd.Source != nil {
		if upd.Source.URL != "" {
			cfg.Source.URL = upd.Source.URL
		}
		cfg.Source.Username = upd.Source.Username
		if upd.Source.Password != nil && *upd.Source.Password != "" {
			cfg.Source.Password = *upd.Source.Password
		}
	}

	if upd.Destinations != nil {
		dests := make([]config.DestinationConfig, 0, len(*upd.Destinations))
		for i, d := range *upd.Destinations {
			nd := config.DestinationConfig{Name: d.Name, URL: d.URL, Username: d.Username}
			if d.Password != nil && *d.Password != "" {
				nd.Password = *d.Password
			} else if i < len(cfg.Destinations) {
				nd.Password = cfg.Destinations[i].Password
			}
			dests = append(dests, nd)
		}
		cfg.Destinations = dests
	}

	if upd.Sync != nil {
		if upd.Sync.Interval != nil {
			cfg.Sync.Interval = *upd.Sync.Interval
		}
		if upd.Sync.Timeout != nil {
			cfg.Sync.Timeout = *upd.Sync.Timeout
		}
		if upd.Sync.BatchSize != nil {
			cfg.Sync.BatchSize = *upd.Sync.BatchSize
		}
		if upd.Sync.DeleteMode != nil {
			cfg.Sync.DeleteMode = *upd.Sync.DeleteMode
		}
		if upd.Sync.FilterPrivate != nil {
			cfg.Sync.FilterPrivate = *upd.Sync.FilterPrivate
		}
	}

	if upd.Web != nil {
		if upd.Web.Addr != nil {
			cfg.Web.Addr = *upd.Web.Addr
		}
		if upd.Web.Token != nil {
			cfg.Web.Token = *upd.Web.Token
		}
	}

	if upd.Logging != nil {
		if upd.Logging.Level != nil {
			cfg.Logging.Level = *upd.Logging.Level
		}
		if upd.Logging.Format != nil {
			cfg.Logging.Format = *upd.Logging.Format
		}
	}
}

func marshalConfig(cfg *config.Config) ([]byte, error) {
	return yaml.Marshal(cfg)
}

func writeConfigFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}
