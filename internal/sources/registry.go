package sources

import "fmt"

// registeredSource pairs a source implementation with the session
// template configuration used to map its selected items into hive sessions.
type registeredSource struct {
	id          string
	source      Source
	templates   TemplateConfig
	displayName string
}

// RegistryEntry exposes a registered source's public fields for the picker.
type RegistryEntry struct {
	ID          string
	DisplayName string
	Source      Source
	Templates   TemplateConfig
}

// Registry holds the set of sources configured for this hive instance in
// registration order, indexed by source id (e.g. "issues" or "prs") for
// lookup.
type Registry struct {
	entries []registeredSource
	index   map[string]int // source id -> entries index
}

// NewRegistry constructs an empty Registry.
func NewRegistry() *Registry {
	return &Registry{index: make(map[string]int)}
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
	if _, exists := r.index[id]; exists {
		return fmt.Errorf("source registry: %q is already registered", id)
	}
	name := displayName
	if name == "" {
		name = id
	}
	r.index[id] = len(r.entries)
	r.entries = append(r.entries, registeredSource{id: id, source: source, templates: templates, displayName: name})
	return nil
}

// Get returns the source and template configuration registered under id.
func (r *Registry) Get(id string) (Source, TemplateConfig, bool) {
	i, ok := r.index[id]
	if !ok {
		return nil, TemplateConfig{}, false
	}
	entry := r.entries[i]
	return entry.source, entry.templates, true
}

// IDs returns the ids of all registered sources in registration order.
func (r *Registry) IDs() []string {
	ids := make([]string, 0, len(r.entries))
	for _, e := range r.entries {
		ids = append(ids, e.id)
	}
	return ids
}

// All returns all registered sources in registration order.
func (r *Registry) All() []RegistryEntry {
	entries := make([]RegistryEntry, 0, len(r.entries))
	for _, e := range r.entries {
		entries = append(entries, RegistryEntry{
			ID:          e.id,
			DisplayName: e.displayName,
			Source:      e.source,
			Templates:   e.templates,
		})
	}
	return entries
}
