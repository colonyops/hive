package claude

import (
	"sync"
	"time"
)

// Cache provides TTL-based caching for analytics.
type Cache struct {
	data map[string]*cacheEntry
	ttl  time.Duration
	mu   sync.RWMutex
}

type cacheEntry struct {
	analytics *SessionAnalytics
	timestamp time.Time
}

// NewCache creates a new cache with the given TTL.
func NewCache(ttl time.Duration) *Cache {
	return &Cache{
		data: make(map[string]*cacheEntry),
		ttl:  ttl,
	}
}

// Get retrieves cached analytics, or nil if expired/missing.
func (c *Cache) Get(sessionID string) *SessionAnalytics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.data[sessionID]
	if !ok {
		return nil
	}

	// Check if expired
	if time.Since(entry.timestamp) > c.ttl {
		return nil
	}

	return entry.analytics
}

// Set stores analytics in the cache.
func (c *Cache) Set(sessionID string, analytics *SessionAnalytics) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data[sessionID] = &cacheEntry{
		analytics: analytics,
		timestamp: time.Now(),
	}
}

// Clear removes all cached entries.
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data = make(map[string]*cacheEntry)
}
