package sweep

import (
	"context"
	"time"

	"github.com/colonyops/hive/pkg/logutils"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/store"
)

// Start launches a background goroutine that periodically sweeps expired KV entries.
// It blocks until the context is cancelled.
func Start(ctx context.Context, logger zerolog.Logger, kvStore *store.KVStore, interval time.Duration) {
	logger = logutils.Component(logger, "sweep")
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := kvStore.SweepExpired(ctx); err != nil {
				logger.Debug().Err(err).Msg("kv sweep failed")
			}
		}
	}
}
