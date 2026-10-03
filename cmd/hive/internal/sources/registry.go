package sources

import "fmt"

type registeredSource struct {
	id          string
	backend     Backend
	source      Source
	templates   TemplateConfig
	displayName string
}

type regKey struct {
	id      string
	backend Backend
}

// RegistryEntry exposes a registered source's public fields for the picker.
type RegistryEntry struct {
	ID          string
	DisplayName string
	Source      Source
	Templates   TemplateConfig
}

// Registry holds the set of sources configured for this hive instance in
// registration order, indexed by (id, backend) so a source id can be serviced
// by a different driver depending on the repo's forge.
type Registry struct {
	entries []registeredSource
	index   map[regKey]int
}

// NewRegistry constructs an empty Registry.
func NewRegistry() *Registry {
	return &Registry{index: make(map[regKey]int)}
}

// Register adds a source under (id, backend). displayName defaults to id.
// It returns an error if id is empty, source is nil, or the (id, backend)
// pair is already registered.
func (r *Registry) Register(id string, backend Backend, source Source, templates TemplateConfig, displayName string) error {
	if id == "" {
		return fmt.Errorf("source registry: id is required")
	}
	if source == nil {
		return fmt.Errorf("source registry: source %q is nil", id)
	}
	key := regKey{id: id, backend: backend}
	if _, exists := r.index[key]; exists {
		return fmt.Errorf("source registry: %q (%s) is already registered", id, backend)
	}
	name := displayName
	if name == "" {
		name = id
	}
	r.index[key] = len(r.entries)
	r.entries = append(r.entries, registeredSource{id: id, backend: backend, source: source, templates: templates, displayName: name})
	return nil
}

// Get returns the source and template configuration registered under
// (id, backend).
func (r *Registry) Get(id string, backend Backend) (Source, TemplateConfig, bool) {
	i, ok := r.index[regKey{id: id, backend: backend}]
	if !ok {
		return nil, TemplateConfig{}, false
	}
	entry := r.entries[i]
	return entry.source, entry.templates, true
}

// IDs returns the distinct ids of all registered sources in registration
// order, deduplicated across backends.
func (r *Registry) IDs() []string {
	seen := make(map[string]struct{}, len(r.entries))
	ids := make([]string, 0, len(r.entries))
	for _, e := range r.entries {
		if _, ok := seen[e.id]; ok {
			continue
		}
		seen[e.id] = struct{}{}
		ids = append(ids, e.id)
	}
	return ids
}

// All returns the registered sources for backend in registration order.
func (r *Registry) All(backend Backend) []RegistryEntry {
	entries := make([]RegistryEntry, 0, len(r.entries))
	for _, e := range r.entries {
		if e.backend != backend {
			continue
		}
		entries = append(entries, RegistryEntry{
			ID:          e.id,
			DisplayName: e.displayName,
			Source:      e.source,
			Templates:   e.templates,
		})
	}
	return entries
}
