package daemon

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog"
)

// ownershipCheckInterval is a variable so a test need not wait for it.
var ownershipCheckInterval = 30 * time.Second

// claim remembers a file the daemon holds by identity, not by path, so the
// daemon can tell when the path names another file.
type claim struct {
	path string
	info os.FileInfo
}

func claimFile(path string) (claim, error) {
	info, err := os.Stat(path)
	if err != nil {
		return claim{}, fmt.Errorf("inspect %s: %w", path, err)
	}
	return claim{path: path, info: info}, nil
}

func (c claim) held() bool {
	now, err := os.Stat(c.path)
	return err == nil && os.SameFile(c.info, now)
}

// watchClaims stops the daemon once its lock file or socket is gone or
// replaced. A client that finds no lock file at the path starts a new daemon
// there, so without this a daemon whose data dir was deleted runs forever
// beside its replacement.
func watchClaims(ctx context.Context, logger zerolog.Logger, interval time.Duration, stop func(), claims ...claim) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		for _, c := range claims {
			if !c.held() {
				logger.Warn().Str("path", c.path).Msg("daemon file removed or replaced; stopping")
				stop()
				return
			}
		}
	}
}
