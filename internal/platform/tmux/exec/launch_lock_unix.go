//go:build !windows

package tmuxexec

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/colonyops/hive/pkg/flock"
)

func (r execRunner) lockLaunch(ctx context.Context, name string) (func() error, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("tmux launch lock directory: %w", err)
	}
	dir := filepath.Join(cache, "hive", "tmux-launch-locks")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create tmux launch lock directory: %w", err)
	}
	unlock, err := flock.Lock(ctx, filepath.Join(dir, launchLockName(r.socketIdentity(ctx), name)), true)
	if err != nil {
		return nil, fmt.Errorf("acquire tmux launch lock: %w", err)
	}
	return unlock, nil
}

// launchLockBuckets bounds the lock files per tmux server. A lock file cannot
// be deleted safely while another process may wait on it, so files are reused
// instead; two sessions that share a bucket only wait for each other.
const launchLockBuckets = 64

func launchLockName(socket, session string) string {
	server := sha256.Sum256([]byte(socket))
	key := sha256.Sum256([]byte(socket + "\x00" + session))
	return fmt.Sprintf("%x-%02d.lock", server[:8], int(key[0])%launchLockBuckets)
}

func (r execRunner) socketIdentity(ctx context.Context) string {
	getenv := os.Getenv
	if r.environ != nil {
		env := r.environ(ctx)
		getenv = func(name string) string {
			for _, pair := range env {
				if value, ok := strings.CutPrefix(pair, name+"="); ok {
					return value
				}
			}
			return ""
		}
	}
	label := ""
	if r.prepareArgs != nil {
		args := r.prepareArgs(nil)
		for i := 0; i+1 < len(args); i++ {
			switch args[i] {
			case "-S":
				return filepath.Clean(args[i+1])
			case "-L":
				label = args[i+1]
			}
		}
	}
	if label == "" {
		if path, _, ok := strings.Cut(getenv("TMUX"), ","); ok && path != "" {
			return filepath.Clean(path)
		}
		label = "default"
	}
	root := getenv("TMUX_TMPDIR")
	if root == "" {
		root = "/tmp"
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	return filepath.Join(root, fmt.Sprintf("tmux-%d", os.Getuid()), label)
}
