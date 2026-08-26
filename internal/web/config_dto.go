package web

import (
	"os"

	"github.com/smiden/synccal/internal/config"
	"gopkg.in/yaml.v3"
)

// Sanitized view returned by GET /api/config — passwords never leave the server.
type configView struct {
	Sources      []sourceView      `json:"sources"`
	Destinations []destinationView `json:"destinations"`
	Database     databaseView      `json:"database"`
	Sync         syncView          `json:"sync"`
	Web          webView           `json:"web"`
	Logging      loggingView       `json:"logging"`
}

type transformerView struct {
	Type    string            `json:"type"`
	Options map[string]string `json:"options,omitempty"`
}

type sourceView struct {
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	URL          string            `json:"url"`
	Username     string            `json:"username"`
	Transformers []transformerView `json:"transformers,omitempty"`
}

type destinationView struct {
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	URL          string            `json:"url"`
	Username     string            `json:"username"`
	Source       string            `json:"source"`
	Transformers []transformerView `json:"transformers,omitempty"`
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
	Sources      *[]sourceUpdate      `json:"sources"`
	Destinations *[]destinationUpdate `json:"destinations"`
	Sync         *syncUpdate          `json:"sync"`
	Web          *webUpdate           `json:"web"`
	Logging      *loggingUpdate       `json:"logging"`
}

type transformerUpdate struct {
	Type    string            `json:"type"`
	Options map[string]string `json:"options"`
}

type sourceUpdate struct {
	Name         string              `json:"name"`
	Type         string              `json:"type"`
	URL          string              `json:"url"`
	Username     string              `json:"username"`
	Password     *string             `json:"password"`
	Transformers []transformerUpdate `json:"transformers"`
}

type destinationUpdate struct {
	Name         string              `json:"name"`
	Type         string              `json:"type"`
	URL          string              `json:"url"`
	Username     string              `json:"username"`
	Password     *string             `json:"password"`
	Source       string              `json:"source"`
	Transformers []transformerUpdate `json:"transformers"`
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
	for _, src := range cfg.Sources {
		sv := sourceView{
			Name:     src.Name,
			Type:     src.Type,
			URL:      src.URL,
			Username: src.Username,
		}
		for _, tr := range src.Transformers {
			sv.Transformers = append(sv.Transformers, transformerView{Type: tr.Type, Options: tr.Options})
		}
		cv.Sources = append(cv.Sources, sv)
	}
	for _, d := range cfg.Destinations {
		dv := destinationView{
			Name:     d.Name,
			Type:     d.Type,
			URL:      d.URL,
			Username: d.Username,
			Source:   d.Source,
		}
		for _, tr := range d.Transformers {
			dv.Transformers = append(dv.Transformers, transformerView{Type: tr.Type, Options: tr.Options})
		}
		cv.Destinations = append(cv.Destinations, dv)
	}
	return cv
}

// mergeConfigUpdate applies the update onto the current config in place.
func mergeConfigUpdate(cfg *config.Config, upd *configUpdate) {
	if upd.Sources != nil {
		srcs := make([]config.SourceConfig, 0, len(*upd.Sources))
		for i, s := range *upd.Sources {
			ns := config.SourceConfig{Name: s.Name, URL: s.URL, Username: s.Username}
			if s.Type != "" {
				ns.Type = s.Type
			} else if i < len(cfg.Sources) && cfg.Sources[i].Type != "" {
				ns.Type = cfg.Sources[i].Type
			} else {
				ns.Type = "caldav"
			}
			if s.Password != nil && *s.Password != "" {
				ns.Password = *s.Password
			} else if i < len(cfg.Sources) {
				ns.Password = cfg.Sources[i].Password
			}
			for _, tr := range s.Transformers {
				ns.Transformers = append(ns.Transformers, config.TransformerConfig{Type: tr.Type, Options: tr.Options})
			}
			srcs = append(srcs, ns)
		}
		cfg.Sources = srcs
	}

	if upd.Destinations != nil {
		dests := make([]config.DestinationConfig, 0, len(*upd.Destinations))
		for i, d := range *upd.Destinations {
			nd := config.DestinationConfig{Name: d.Name, URL: d.URL, Username: d.Username, Source: d.Source}
			if d.Type != "" {
				nd.Type = d.Type
			} else if i < len(cfg.Destinations) && cfg.Destinations[i].Type != "" {
				nd.Type = cfg.Destinations[i].Type
			} else {
				nd.Type = "caldav"
			}
			if d.Password != nil && *d.Password != "" {
				nd.Password = *d.Password
			} else if i < len(cfg.Destinations) {
				nd.Password = cfg.Destinations[i].Password
			}
			for _, tr := range d.Transformers {
				nd.Transformers = append(nd.Transformers, config.TransformerConfig{Type: tr.Type, Options: tr.Options})
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
