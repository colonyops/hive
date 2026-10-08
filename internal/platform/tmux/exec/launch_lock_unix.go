//go:build !windows

package tmuxexec

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"
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
	key := sha256.Sum256([]byte(r.socketIdentity(ctx) + "\x00" + name))
	file, err := os.OpenFile(filepath.Join(dir, fmt.Sprintf("%x.lock", key)), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open tmux launch lock: %w", err)
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, errors.Join(err, file.Close())
		}
		err = unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return func() error { return errors.Join(unix.Flock(int(file.Fd()), unix.LOCK_UN), file.Close()) }, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			return nil, errors.Join(fmt.Errorf("acquire tmux launch lock: %w", err), file.Close())
		}
		timer := time.NewTimer(startupPollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, errors.Join(ctx.Err(), file.Close())
		case <-timer.C:
		}
	}
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
