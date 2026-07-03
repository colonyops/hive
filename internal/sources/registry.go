package sources

import "fmt"

// registeredSource pairs a source implementation with the session
// template configuration used to map its selected items into hive sessions.
type registeredSource struct {
	source    Source
	templates TemplateConfig
}

// Registry holds the set of sources configured for this hive instance,
// keyed by source id (e.g. "issues" or "prs").
type Registry struct {
	sources map[string]registeredSource
}

// NewRegistry constructs an empty Registry.
func NewRegistry() *Registry {
	return &Registry{sources: make(map[string]registeredSource)}
}

// Register adds a source under id along with the template configuration
// used to render its selected items into session fields. It returns an error
// if id is empty or already registered.
func (r *Registry) Register(id string, source Source, templates TemplateConfig) error {
	if id == "" {
		return fmt.Errorf("source registry: id is required")
	}
	if source == nil {
		return fmt.Errorf("source registry: source %q is nil", id)
	}
	if _, exists := r.sources[id]; exists {
		return fmt.Errorf("source registry: %q is already registered", id)
	}
	r.sources[id] = registeredSource{source: source, templates: templates}
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
