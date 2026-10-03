package classifier

// Cache stores classification results, invalidated on process fingerprint change.
type Cache struct {
	entries map[string]cacheEntry
}

type cacheEntry struct {
	result      Result
	fingerprint int64
}

// NewCache creates an empty classification cache.
func NewCache() *Cache {
	return &Cache{entries: make(map[string]cacheEntry)}
}

// Get returns a cached classification if the process fingerprint hasn't changed.
func (c *Cache) Get(paneID string, currentFingerprint int64) (Result, bool) {
	if c == nil || c.entries == nil {
		return Result{}, false
	}
	entry, ok := c.entries[paneID]
	if !ok || entry.fingerprint != currentFingerprint {
		return Result{}, false
	}
	return entry.result, true
}

// Set stores a classification result for a pane.
func (c *Cache) Set(paneID string, fingerprint int64, result Result) {
	if c.entries == nil {
		c.entries = make(map[string]cacheEntry)
	}
	c.entries[paneID] = cacheEntry{result: result, fingerprint: fingerprint}
}

// Prune removes entries for pane IDs not in the provided set.
func (c *Cache) Prune(activePaneIDs map[string]bool) {
	if c == nil || c.entries == nil {
		return
	}
	for paneID := range c.entries {
		if !activePaneIDs[paneID] {
			delete(c.entries, paneID)
		}
	}
}

// Len returns the number of cached entries.
func (c *Cache) Len() int {
	if c == nil {
		return 0
	}
	return len(c.entries)
}
