//go:build !windows

package flock_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/pkg/flock"
)

func TestLockWithoutWaitFailsWhileHeld(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.lock")
	unlock, err := flock.Lock(t.Context(), path, false)
	require.NoError(t, err)

	_, err = flock.Lock(t.Context(), path, false)
	require.ErrorIs(t, err, flock.ErrLocked)

	require.NoError(t, unlock())
	again, err := flock.Lock(t.Context(), path, false)
	require.NoError(t, err)
	require.NoError(t, again())
}

func TestLockWithWaitBlocksUntilReleased(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.lock")
	unlock, err := flock.Lock(t.Context(), path, false)
	require.NoError(t, err)

	acquired := make(chan error, 1)
	go func() {
		second, err := flock.Lock(t.Context(), path, true)
		if err == nil {
			err = second()
		}
		acquired <- err
	}()

	select {
	case <-acquired:
		t.Fatal("the waiter took a held lock")
	case <-time.After(150 * time.Millisecond):
	}
	require.NoError(t, unlock())
	select {
	case err := <-acquired:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("the waiter never took the released lock")
	}
}

func TestLockWithWaitStopsWithTheContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.lock")
	unlock, err := flock.Lock(t.Context(), path, false)
	require.NoError(t, err)
	t.Cleanup(func() { _ = unlock() })

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	_, err = flock.Lock(ctx, path, true)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}
