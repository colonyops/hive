package sources

import (
	"fmt"
	"sort"
)

// registeredSource pairs a source implementation with the session
// template configuration used to map its selected items into hive sessions.
type registeredSource struct {
	source      Source
	templates   TemplateConfig
	displayName string
	order       int // registration order for stable tab ordering
}

// RegistryEntry exposes a registered source's public fields for the picker.
type RegistryEntry struct {
	ID          string
	DisplayName string
	Source      Source
	Templates   TemplateConfig
}

// Registry holds the set of sources configured for this hive instance,
// keyed by source id (e.g. "issues" or "prs").
type Registry struct {
	sources map[string]registeredSource
	nextOrd int
}

// NewRegistry constructs an empty Registry.
func NewRegistry() *Registry {
	return &Registry{sources: make(map[string]registeredSource)}
}

// Register adds a source under id along with the template configuration
// used to render its selected items into session fields. displayName is
// shown in the picker tab bar; if empty, id is used. It returns an error
// if id is empty or already registered.
func (r *Registry) Register(id string, source Source, templates TemplateConfig, displayName string) error {
	if id == "" {
		return fmt.Errorf("source registry: id is required")
	}
	if source == nil {
		return fmt.Errorf("source registry: source %q is nil", id)
	}
	if _, exists := r.sources[id]; exists {
		return fmt.Errorf("source registry: %q is already registered", id)
	}
	name := displayName
	if name == "" {
		name = id
	}
	r.sources[id] = registeredSource{source: source, templates: templates, displayName: name, order: r.nextOrd}
	r.nextOrd++
	return nil
}

// Get returns the source and template configuration registered under id.
func (r *Registry) Get(id string) (Source, TemplateConfig, bool) {
	entry, ok := r.sources[id]
	if !ok {
		return nil, TemplateConfig{}, false
	}
	return entry.source, entry.templates, true
}

// IDs returns the ids of all registered sources.
func (r *Registry) IDs() []string {
	ids := make([]string, 0, len(r.sources))
	for id := range r.sources {
		ids = append(ids, id)
	}
	return ids
}

// All returns all registered sources in registration order.
func (r *Registry) All() []RegistryEntry {
	entries := make([]RegistryEntry, 0, len(r.sources))
	type ordered struct {
		entry RegistryEntry
		order int
	}
	var ords []ordered
	for id, rs := range r.sources {
		ords = append(ords, ordered{
			entry: RegistryEntry{
				ID:          id,
				DisplayName: rs.displayName,
				Source:      rs.source,
				Templates:   rs.templates,
			},
			order: rs.order,
		})
	}
	sort.Slice(ords, func(i, j int) bool { return ords[i].order < ords[j].order })
	for _, o := range ords {
		entries = append(entries, o.entry)
	}
	return entries
}
