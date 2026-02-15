package sweep

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/hay-kot/hive/internal/data/stores"
)

// Start launches a background goroutine that periodically sweeps expired KV entries.
// It blocks until the context is cancelled.
func Start(ctx context.Context, kvStore *stores.KVStore, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := kvStore.SweepExpired(ctx); err != nil {
				log.Debug().Err(err).Msg("kv sweep failed")
			}
		}
	}
}
