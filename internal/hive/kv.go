package hive

import (
	"context"
	"time"

	"github.com/colonyops/hive/pkg/logutils"
)

// SweepKV deletes expired KV entries every interval until ctx is done. Reads
// already treat an expired entry as missing; the sweep only reclaims the rows.
func (e *Engine) SweepKV(ctx context.Context, interval time.Duration) {
	logger := logutils.Component(e.ports.Logger, "kv")
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := e.kv.SweepExpired(ctx); err != nil {
				logger.Debug().Err(err).Msg("kv sweep failed")
			}
		}
	}
}
