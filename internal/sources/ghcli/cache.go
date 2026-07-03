package ghcli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/colonyops/hive/internal/core/kv"
)

// searchCacheTTL bounds how long raw Search output is cached per
// (scope, query) key when a cache is configured. Kept short since issue/PR
// lists change frequently and the source has no push-invalidation.
const searchCacheTTL = 30 * time.Second

// searchCache wraps an optional kv.KV store for caching raw gh Search
// output. A nil store makes every method a pass-through cache miss, so
// callers never need to special-case a disabled cache.
type searchCache struct {
	typed *kv.TypedKV[json.RawMessage]
}

// newSearchCache returns a searchCache backed by store under namespace.
// store may be nil, in which case the cache is a permanent no-op.
func newSearchCache(store kv.KV, namespace string) searchCache {
	if store == nil {
		return searchCache{}
	}
	return searchCache{typed: kv.Scoped[json.RawMessage](store, namespace)}
}

// get returns cached gh output for key, or ok=false on a cache miss or
// disabled cache.
func (c searchCache) get(ctx context.Context, key string) ([]byte, bool) {
	if c.typed == nil {
		return nil, false
	}
	out, err := c.typed.Get(ctx, key)
	if err != nil {
		// A missing/expired key is a normal miss; anything else (DB
		// failure, corrupt JSON) is still served as a miss but logged so
		// it doesn't vanish silently.
		if !errors.Is(err, sql.ErrNoRows) {
			log.Debug().Err(err).Str("key", key).Msg("ghcli source: search cache read failed")
		}
		return nil, false
	}
	return out, true
}

// set stores gh output under key with searchCacheTTL. It is a no-op when
// the cache is disabled.
func (c searchCache) set(ctx context.Context, key string, out []byte) {
	if c.typed == nil {
		return
	}
	if err := c.typed.SetTTL(ctx, key, json.RawMessage(out), searchCacheTTL); err != nil {
		log.Debug().Err(err).Str("key", key).Msg("ghcli source: search cache write failed")
	}
}
