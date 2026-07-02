package connectors

import "fmt"

// registeredConnector pairs a connector implementation with the session
// template configuration used to map its selected items into hive sessions.
type registeredConnector struct {
	connector Connector
	templates TemplateConfig
}

// Registry holds the set of connectors configured for this hive instance,
// keyed by connector id (e.g. "github", or an external connector's
// configured id).
type Registry struct {
	connectors map[string]registeredConnector
}

// NewRegistry constructs an empty Registry.
func NewRegistry() *Registry {
	return &Registry{connectors: make(map[string]registeredConnector)}
}

// Register adds a connector under id along with the template configuration
// used to render its selected items into session fields. It returns an error
// if id is empty or already registered.
func (r *Registry) Register(id string, connector Connector, templates TemplateConfig) error {
	if id == "" {
		return fmt.Errorf("connector registry: id is required")
	}
	if connector == nil {
		return fmt.Errorf("connector registry: connector %q is nil", id)
	}
	if _, exists := r.connectors[id]; exists {
		return fmt.Errorf("connector registry: %q is already registered", id)
	}
	r.connectors[id] = registeredConnector{connector: connector, templates: templates}
	return nil
}

// Get returns the connector and template configuration registered under id.
func (r *Registry) Get(id string) (Connector, TemplateConfig, bool) {
	entry, ok := r.connectors[id]
	if !ok {
		return nil, TemplateConfig{}, false
	}
	return entry.connector, entry.templates, true
}

// IDs returns the ids of all registered connectors.
func (r *Registry) IDs() []string {
	ids := make([]string, 0, len(r.connectors))
	for id := range r.connectors {
		ids = append(ids, id)
	}
	return ids
}
