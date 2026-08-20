package web

import (
	"os"

	"github.com/smiden/synccal/internal/config"
	"gopkg.in/yaml.v3"
)

// Sanitized view returned by GET /api/config — passwords never leave the server.
type configView struct {
	Sources  []sourceView `json:"sources"`
	Database databaseView `json:"database"`
	Sync     syncView     `json:"sync"`
	Web      webView      `json:"web"`
	Logging  loggingView  `json:"logging"`
}

type sourceView struct {
	URL         string          `json:"url"`
	Username    string          `json:"username"`
	Destination destinationView `json:"destination"`
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
	Sources *[]sourceUpdate `json:"sources"`
	Sync    *syncUpdate     `json:"sync"`
	Web     *webUpdate      `json:"web"`
	Logging *loggingUpdate  `json:"logging"`
}

type sourceUpdate struct {
	URL         string             `json:"url"`
	Username    string             `json:"username"`
	Password    *string            `json:"password"`
	Destination *destinationUpdate `json:"destination"`
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
	for _, s := range cfg.Sources {
		cv.Sources = append(cv.Sources, sourceView{
			URL:         s.URL,
			Username:    s.Username,
			Destination: destinationView{Name: s.Destination.Name, URL: s.Destination.URL, Username: s.Destination.Username},
		})
	}
	return cv
}

// mergeConfigUpdate applies the update onto the current config in place.
func mergeConfigUpdate(cfg *config.Config, upd *configUpdate) {
	if upd.Sources != nil {
		srcs := make([]config.SourceConfig, 0, len(*upd.Sources))
		for i, s := range *upd.Sources {
			ns := config.SourceConfig{URL: s.URL, Username: s.Username}
			if s.Password != nil && *s.Password != "" {
				ns.Password = *s.Password
			} else if i < len(cfg.Sources) {
				ns.Password = cfg.Sources[i].Password
			}

			if s.Destination != nil {
				nd := config.DestinationConfig{Name: s.Destination.Name, URL: s.Destination.URL, Username: s.Destination.Username}
				if s.Destination.Password != nil && *s.Destination.Password != "" {
					nd.Password = *s.Destination.Password
				} else if i < len(cfg.Sources) {
					nd.Password = cfg.Sources[i].Destination.Password
				}
				ns.Destination = nd
			} else if i < len(cfg.Sources) {
				ns.Destination = cfg.Sources[i].Destination
			}
			srcs = append(srcs, ns)
		}
		cfg.Sources = srcs
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
