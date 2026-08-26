package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/emersion/go-ical"
)

// Pipeline chains multiple EventTransformers.
type Pipeline struct {
	transformers []EventTransformer
}

func NewPipeline(transformers ...EventTransformer) *Pipeline {
	return &Pipeline{transformers: transformers}
}

func (p *Pipeline) Add(t EventTransformer) {
	p.transformers = append(p.transformers, t)
}

// Apply runs the pipeline on a VEVENT component. Returns (nil,false) if filtered.
func (p *Pipeline) Apply(ctx context.Context, comp *ical.Component) (*ical.Component, bool, error) {
	cur := comp
	for _, tr := range p.transformers {
		var keep bool
		var err error
		cur, keep, err = tr.Transform(ctx, cur)
		if err != nil {
			return nil, false, err
		}
		if !keep || cur == nil {
			return nil, false, nil
		}
	}
	return cur, true, nil
}

// --- Built-in transformers ---

// FilterPrivateTransformer drops events with CLASS:PRIVATE or CLASS:CONFIDENTIAL.
type FilterPrivateTransformer struct{}

func (t *FilterPrivateTransformer) Name() string { return "filter-private" }

func (t *FilterPrivateTransformer) Transform(_ context.Context, comp *ical.Component) (*ical.Component, bool, error) {
	class, _ := comp.Props.Text("CLASS")
	if class == "PRIVATE" || class == "CONFIDENTIAL" {
		return nil, false, nil
	}
	return comp, true, nil
}

// MaskPrivateTransformer keeps the event but strips private details.
type MaskPrivateTransformer struct {
	MaskSummary string
}

func (t *MaskPrivateTransformer) Name() string { return "mask-private" }

func (t *MaskPrivateTransformer) Transform(_ context.Context, comp *ical.Component) (*ical.Component, bool, error) {
	class, _ := comp.Props.Text("CLASS")
	if class != "PRIVATE" && class != "CONFIDENTIAL" {
		return comp, true, nil
	}
	// Clone and mask
	clone := cloneComponent(comp)
	clone.Props.SetText("SUMMARY", t.MaskSummary)
	if t.MaskSummary == "" {
		clone.Props.SetText("SUMMARY", "Busy")
	}
	clone.Props.Del("DESCRIPTION")
	clone.Props.Del("LOCATION")
	clone.Props.Del("ATTENDEE")
	return clone, true, nil
}

// PrefixTransformer prefixes the UID with a deterministic hash of the source.
type PrefixTransformer struct {
	Prefix string // e.g. "a1b2c3d4"
}

func (t *PrefixTransformer) Name() string { return "prefix-uid" }

func (t *PrefixTransformer) Transform(_ context.Context, comp *ical.Component) (*ical.Component, bool, error) {
	uid, err := comp.Props.Text("UID")
	if err != nil || uid == "" {
		return comp, true, nil
	}
	if strings.HasPrefix(uid, t.Prefix+"-") {
		return comp, true, nil
	}
	// Clone to avoid mutating original slice if needed
	clone := cloneComponent(comp)
	clone.Props.SetText("UID", t.Prefix+"-"+uid)
	return clone, true, nil
}

// NewPrefixForSource derives the 8-char hash used as UID prefix.
func NewPrefixForSource(sourceURL string) string {
	h := sha256.Sum256([]byte(sourceURL))
	return hex.EncodeToString(h[:])[:8]
}

// CategoryFilterTransformer keeps only events whose CATEGORIES matches allowed list.
type CategoryFilterTransformer struct {
	Allowed map[string]bool
}

func (t *CategoryFilterTransformer) Name() string { return "filter-category" }

func (t *CategoryFilterTransformer) Transform(_ context.Context, comp *ical.Component) (*ical.Component, bool, error) {
	if len(t.Allowed) == 0 {
		return comp, true, nil
	}
	cats, _ := comp.Props.Text("CATEGORIES")
	if cats == "" {
		return nil, false, nil
	}
	for _, c := range strings.Split(cats, ",") {
		if t.Allowed[strings.TrimSpace(c)] {
			return comp, true, nil
		}
	}
	return nil, false, nil
}

// SummaryPrefixTransformer prepends a prefix to SUMMARY.
type SummaryPrefixTransformer struct {
	Prefix string
}

func (t *SummaryPrefixTransformer) Name() string { return "prefix-summary" }

func (t *SummaryPrefixTransformer) Transform(_ context.Context, comp *ical.Component) (*ical.Component, bool, error) {
	if t.Prefix == "" {
		return comp, true, nil
	}
	sum, err := comp.Props.Text("SUMMARY")
	if err != nil {
		return comp, true, nil
	}
	clone := cloneComponent(comp)
	clone.Props.SetText("SUMMARY", t.Prefix+sum)
	return clone, true, nil
}

func cloneComponent(comp *ical.Component) *ical.Component {
	clone := &ical.Component{
		Name:     comp.Name,
		Props:    make(ical.Props),
		Children: comp.Children,
	}
	for k, v := range comp.Props {
		nv := make([]ical.Prop, len(v))
		copy(nv, v)
		clone.Props[k] = nv
	}
	return clone
}

func init() {
	RegisterTransformer("filter-private", PluginInfo{
		Name: "Filtre PRIVATE/CONFIDENTIAL",
		Description: "Exclut les événements avec CLASS:PRIVATE ou CLASS:CONFIDENTIAL",
	}, func(options map[string]string) (EventTransformer, error) {
		return &FilterPrivateTransformer{}, nil
	})
	RegisterTransformer("mask-private", PluginInfo{
		Name: "Masquage PRIVATE",
		Description: "Conserve l'événement mais masque SUMMARY/DESCRIPTION/LOCATION",
	}, func(options map[string]string) (EventTransformer, error) {
		prefix := options["summary"]
		if prefix == "" {
			prefix = "Busy"
		}
		return &MaskPrivateTransformer{MaskSummary: prefix}, nil
	})
	RegisterTransformer("prefix-uid", PluginInfo{
		Name: "Préfixe UID",
		Description: "Préfixe l'UID avec le hash de la source pour éviter les collisions",
	}, func(options map[string]string) (EventTransformer, error) {
		prefix := options["prefix"]
		if prefix == "" {
			prefix = options["source_url"] // fallback, will be hashed
			if prefix != "" {
				prefix = NewPrefixForSource(prefix)
			}
		}
		return &PrefixTransformer{Prefix: prefix}, nil
	})
	RegisterTransformer("filter-category", PluginInfo{
		Name: "Filtre par catégorie",
		Description: "Ne conserve que les événements dont CATEGORIES est dans la liste autorisée",
	}, func(options map[string]string) (EventTransformer, error) {
		allowed := make(map[string]bool)
		if cats, ok := options["categories"]; ok {
			for _, c := range strings.Split(cats, ",") {
				allowed[strings.TrimSpace(c)] = true
			}
		}
		return &CategoryFilterTransformer{Allowed: allowed}, nil
	})
	RegisterTransformer("prefix-summary", PluginInfo{
		Name: "Préfixe résumé",
		Description: "Ajoute un préfixe au SUMMARY de l'événement",
	}, func(options map[string]string) (EventTransformer, error) {
		return &SummaryPrefixTransformer{Prefix: options["prefix"]}, nil
	})
}
