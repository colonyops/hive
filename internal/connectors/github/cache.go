package github

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/colonyops/hive/internal/core/kv"
)

// searchCacheTTL bounds how long a Search result set is cached per
// (scope, query) key when a cache is configured. Kept short since issue
// lists change frequently and the connector has no push-invalidation.
const searchCacheTTL = 30 * time.Second

// searchCache wraps an optional kv.KV store for caching Search results.
// A nil store makes every method a pass-through cache miss, so callers
// never need to special-case a disabled cache.
type searchCache struct {
	typed *kv.TypedKV[[]issueListItem]
}

// newSearchCache returns a searchCache backed by store. store may be nil,
// in which case the cache is a permanent no-op.
func newSearchCache(store kv.KV) searchCache {
	if store == nil {
		return searchCache{}
	}
	return searchCache{typed: kv.Scoped[[]issueListItem](store, "connectors.github.search")}
}

// get returns a cached search result for key, or ok=false on a cache miss
// or disabled cache.
func (c searchCache) get(ctx context.Context, key string) ([]issueListItem, bool) {
	if c.typed == nil {
		return nil, false
	}
	items, err := c.typed.Get(ctx, key)
	if err != nil {
		// A missing/expired key is a normal miss; anything else (DB
		// failure, corrupt JSON) is still served as a miss but logged so
		// it doesn't vanish silently.
		if !errors.Is(err, sql.ErrNoRows) {
			log.Debug().Err(err).Str("key", key).Msg("github connector: search cache read failed")
		}
		return nil, false
	}
	return items, true
}

// set stores items under key with searchCacheTTL. It is a no-op when the
// cache is disabled.
func (c searchCache) set(ctx context.Context, key string, items []issueListItem) {
	if c.typed == nil {
		return
	}
	if err := c.typed.SetTTL(ctx, key, items, searchCacheTTL); err != nil {
		log.Debug().Err(err).Str("key", key).Msg("github connector: search cache write failed")
	}
}
