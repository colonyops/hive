//go:build !windows

// Package flock takes an exclusive advisory lock on a file, shared between
// processes of the same user.
package flock

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// ErrLocked is returned by Lock with wait false when another holder has the lock.
var ErrLocked = errors.New("file is locked")

const pollInterval = 50 * time.Millisecond

// Lock opens or creates path and takes an exclusive lock on it. With wait it
// retries until the lock frees or ctx ends; without wait it returns ErrLocked
// at once. The lock ends when unlock runs or the process exits.
func Lock(ctx context.Context, path string, wait bool) (unlock func() error, err error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
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
			return nil, errors.Join(fmt.Errorf("acquire lock: %w", err), file.Close())
		}
		if !wait {
			return nil, errors.Join(ErrLocked, file.Close())
		}
		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, errors.Join(ctx.Err(), file.Close())
		case <-timer.C:
		}
	}
}
