package pipelinedb

import (
	"context"
	"os"
	"time"
)

// DebugPauseIngest widens the post-hydration/pre-ingest crash window when the
// duration environment variable is set. It is intentionally cancellable.
func DebugPauseIngest(ctx context.Context) {
	d, err := time.ParseDuration(os.Getenv("HIVE_DESKTOP_DEBUG_PAUSE_INGEST"))
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
