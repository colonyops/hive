package plugins

import (
	"maps"
	"sync"
)

// StatusCollector provides thread-safe caching for plugin statuses.
// It stores statuses indexed by plugin name and session ID.
type StatusCollector struct {
	mu       sync.RWMutex
	statuses map[string]map[string]Status // pluginName -> sessionID -> Status
}

// NewStatusCollector creates a new status collector.
func NewStatusCollector() *StatusCollector {
	return &StatusCollector{
		statuses: make(map[string]map[string]Status),
	}
}

// Set stores a status for the given plugin and session.
func (c *StatusCollector) Set(pluginName, sessionID string, status Status) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.statuses[pluginName] == nil {
		c.statuses[pluginName] = make(map[string]Status)
	}
	c.statuses[pluginName][sessionID] = status
}

// Get retrieves a status for the given plugin and session.
func (c *StatusCollector) Get(pluginName, sessionID string) (Status, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if pluginStatuses, ok := c.statuses[pluginName]; ok {
		status, found := pluginStatuses[sessionID]
		return status, found
	}
	return Status{}, false
}

// GetAll returns all statuses for a given plugin.
func (c *StatusCollector) GetAll(pluginName string) map[string]Status {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if pluginStatuses, ok := c.statuses[pluginName]; ok {
		// Return a copy to prevent concurrent modification
		result := make(map[string]Status, len(pluginStatuses))
		maps.Copy(result, pluginStatuses)
		return result
	}
	return nil
}

// Clear removes all cached statuses.
func (c *StatusCollector) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.statuses = make(map[string]map[string]Status)
}
