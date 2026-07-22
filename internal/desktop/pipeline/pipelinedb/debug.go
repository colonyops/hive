package pipelinedb

import (
	"context"
	"os"
	"time"
)

// DebugPauseIngest widens the post-hydration/pre-ingest crash window when the
// duration environment variable is set. It is intentionally cancellable.
func DebugPauseIngest(ctx context.Context) {
	debugPause(ctx, "HIVE_DESKTOP_DEBUG_PAUSE_INGEST")
}

// debugPauseCommit widens the post-ingestion/pre-engine-commit crash window.
// It is deliberately separate from DebugPauseIngest: this hook runs before
// CommitBatch opens its transaction, after the producer has emitted its wakeup.
func debugPauseCommit(ctx context.Context) {
	debugPause(ctx, "HIVE_DESKTOP_DEBUG_PAUSE_COMMIT")
}

func debugPause(ctx context.Context, variable string) {
	d, err := time.ParseDuration(os.Getenv(variable))
	if err != nil || d <= 0 {
		return
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
