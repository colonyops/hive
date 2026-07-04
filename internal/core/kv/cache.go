package kv

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/rs/zerolog/log"
)

// Cache is a namespaced, TTL-bound cache over a KV store. It layers
// cache semantics on top of the storage contract: reads treat every
// error as a miss (missing/expired keys silently, real failures logged
// at debug), and write failures are logged rather than surfaced, so
// callers degrade to uncached behavior instead of handling errors.
type Cache[T any] struct {
	typed *TypedKV[T]
	ttl   time.Duration
}

// NewCache returns a Cache over store, prefixing keys with namespace and
// expiring entries after ttl. store must be non-nil.
func NewCache[T any](store KV, namespace string, ttl time.Duration) *Cache[T] {
	return &Cache[T]{typed: Scoped[T](store, namespace), ttl: ttl}
}

// Get returns the cached value for key, or ok=false on a miss.
func (c *Cache[T]) Get(ctx context.Context, key string) (T, bool) {
	v, err := c.typed.Get(ctx, key)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Debug().Err(err).Str("key", key).Msg("kv cache: read failed")
		}
		var zero T
		return zero, false
	}
	return v, true
}

// Set stores value under key with the cache's TTL.
func (c *Cache[T]) Set(ctx context.Context, key string, value T) {
	if err := c.typed.SetTTL(ctx, key, value, c.ttl); err != nil {
		log.Debug().Err(err).Str("key", key).Msg("kv cache: write failed")
	}
}
