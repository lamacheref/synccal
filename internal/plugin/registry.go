package plugin

import (
	"fmt"
	"sort"
	"sync"
)

// Factory signatures
type SourceFactory func(cfg SourceConfig) (SourceConnector, error)
type DestinationFactory func(cfg DestinationConfig) (DestinationConnector, error)
type TransformerFactory func(options map[string]string) (EventTransformer, error)

// SourceConfig and DestinationConfig are lightweight DTOs passed to factories.
// They mirror the relevant fields from config.SourceConfig / DestinationConfig
// without importing the config package (to avoid circular import).
type SourceConfig struct {
	Name     string
	Type     string
	URL      string
	Username string
	Password string
	Options  map[string]string
}

type DestinationConfig struct {
	Name     string
	Type     string
	URL      string
	Username string
	Password string
	Source   string
	Options  map[string]string
}

var (
	mu                   sync.RWMutex
	sourceFactories      = make(map[string]SourceFactory)
	destinationFactories = make(map[string]DestinationFactory)
	transformerFactories = make(map[string]TransformerFactory)
	sourceInfos          = make(map[string]PluginInfo)
	destinationInfos     = make(map[string]PluginInfo)
	transformerInfos     = make(map[string]PluginInfo)
)

// RegisterSource registers a source connector factory.
func RegisterSource(pluginType string, info PluginInfo, factory SourceFactory) {
	mu.Lock()
	defer mu.Unlock()
	info.Type = pluginType
	info.Kind = "source"
	sourceFactories[pluginType] = factory
	sourceInfos[pluginType] = info
}

// RegisterDestination registers a destination connector factory.
func RegisterDestination(pluginType string, info PluginInfo, factory DestinationFactory) {
	mu.Lock()
	defer mu.Unlock()
	info.Type = pluginType
	info.Kind = "destination"
	destinationFactories[pluginType] = factory
	destinationInfos[pluginType] = info
}

// RegisterTransformer registers an event transformer factory.
func RegisterTransformer(pluginType string, info PluginInfo, factory TransformerFactory) {
	mu.Lock()
	defer mu.Unlock()
	info.Type = pluginType
	info.Kind = "transformer"
	transformerFactories[pluginType] = factory
	transformerInfos[pluginType] = info
}

// NewSource creates a source connector for the given type.
func NewSource(cfg SourceConfig) (SourceConnector, error) {
	mu.RLock()
	factory, ok := sourceFactories[cfg.Type]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown source plugin type %q", cfg.Type)
	}
	return factory(cfg)
}

// NewDestination creates a destination connector for the given type.
func NewDestination(cfg DestinationConfig) (DestinationConnector, error) {
	mu.RLock()
	factory, ok := destinationFactories[cfg.Type]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown destination plugin type %q", cfg.Type)
	}
	return factory(cfg)
}

// NewTransformer creates a transformer for the given type.
func NewTransformer(pluginType string, options map[string]string) (EventTransformer, error) {
	mu.RLock()
	factory, ok := transformerFactories[pluginType]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown transformer plugin type %q", pluginType)
	}
	return factory(options)
}

// List helpers for UI / API discovery.

func ListSources() []PluginInfo {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]PluginInfo, 0, len(sourceInfos))
	for _, v := range sourceInfos {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Type < out[j].Type })
	return out
}

func ListDestinations() []PluginInfo {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]PluginInfo, 0, len(destinationInfos))
	for _, v := range destinationInfos {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Type < out[j].Type })
	return out
}

func ListTransformers() []PluginInfo {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]PluginInfo, 0, len(transformerInfos))
	for _, v := range transformerInfos {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Type < out[j].Type })
	return out
}

func ListAll() []PluginInfo {
	all := append(ListSources(), ListDestinations()...)
	all = append(all, ListTransformers()...)
	sort.Slice(all, func(i, j int) bool {
		if all[i].Kind == all[j].Kind {
			return all[i].Type < all[j].Type
		}
		return all[i].Kind < all[j].Kind
	})
	return all
}
