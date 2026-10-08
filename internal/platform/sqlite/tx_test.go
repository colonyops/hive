package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openTestPool(t *testing.T) *sql.DB {
	t.Helper()
	pool, err := Open(t.Context(), filepath.Join(t.TempDir(), "test.db"), Options{
		MaxOpenConns: 2,
		MaxIdleConns: 2,
		BusyTimeout:  time.Second,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Close() })
	_, err = pool.ExecContext(t.Context(), `CREATE TABLE row (v TEXT NOT NULL)`)
	require.NoError(t, err)
	return pool
}

func insertRow(ctx context.Context, tx *sql.Tx, v string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO row (v) VALUES (?)`, v)
	return err
}

func countRows(t *testing.T, pool *sql.DB) int {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM row`).Scan(&n))
	return n
}

func TestWithinTx_NestedCallJoinsAndCommitsOnce(t *testing.T) {
	t.Parallel()

	pool := openTestPool(t)
	err := WithinTx(t.Context(), pool, func(ctx context.Context, outer *sql.Tx) error {
		require.NoError(t, insertRow(ctx, outer, "outer"))
		return WithinTx(ctx, pool, func(ctx context.Context, inner *sql.Tx) error {
			assert.Same(t, outer, inner)
			assert.Equal(t, 1, pool.Stats().InUse, "the nested call opened a second connection")
			return insertRow(ctx, inner, "inner")
		})
	})
	require.NoError(t, err)
	assert.Equal(t, 2, countRows(t, pool))
	assert.Zero(t, pool.Stats().InUse)
}

func TestWithinTx_NestedFailureRollsBackTheOuterUnit(t *testing.T) {
	t.Parallel()

	pool := openTestPool(t)
	sentinel := errors.New("inner failed")
	err := WithinTx(t.Context(), pool, func(ctx context.Context, outer *sql.Tx) error {
		require.NoError(t, insertRow(ctx, outer, "outer"))
		return WithinTx(ctx, pool, func(ctx context.Context, inner *sql.Tx) error {
			require.NoError(t, insertRow(ctx, inner, "inner"))
			return sentinel
		})
	})
	require.ErrorIs(t, err, sentinel)
	assert.Zero(t, countRows(t, pool))
}

func TestWithinTx_NestedSuccessIsNotDurableUntilTheOuterCommits(t *testing.T) {
	t.Parallel()

	pool := openTestPool(t)
	sentinel := errors.New("outer failed after the inner write")
	err := WithinTx(t.Context(), pool, func(ctx context.Context, _ *sql.Tx) error {
		if err := WithinTx(ctx, pool, func(ctx context.Context, inner *sql.Tx) error {
			return insertRow(ctx, inner, "inner")
		}); err != nil {
			return err
		}
		return sentinel
	})
	require.ErrorIs(t, err, sentinel)
	assert.Zero(t, countRows(t, pool))
}

func TestWithinTx_CancelledContextRollsBack(t *testing.T) {
	t.Parallel()

	pool := openTestPool(t)
	ctx, cancel := context.WithCancel(t.Context())
	err := WithinTx(ctx, pool, func(ctx context.Context, tx *sql.Tx) error {
		require.NoError(t, insertRow(ctx, tx, "row"))
		cancel()
		return ctx.Err()
	})
	require.ErrorIs(t, err, context.Canceled)
	assert.NotContains(t, err.Error(), "rollback also failed")
	assert.Zero(t, countRows(t, pool))
	assert.Zero(t, pool.Stats().InUse)
}

func TestWithinTx_CancelledContextFailsTheCommit(t *testing.T) {
	t.Parallel()

	pool := openTestPool(t)
	ctx, cancel := context.WithCancel(t.Context())
	err := WithinTx(ctx, pool, func(ctx context.Context, tx *sql.Tx) error {
		require.NoError(t, insertRow(ctx, tx, "row"))
		cancel()
		return nil
	})
	require.Error(t, err)
	assert.Zero(t, countRows(t, pool))
}

func TestAmbientTx_IsScopedToItsPool(t *testing.T) {
	t.Parallel()

	first := openTestPool(t)
	second := openTestPool(t)
	sentinel := errors.New("first unit failed")

	err := WithinTx(t.Context(), first, func(ctx context.Context, firstTx *sql.Tx) error {
		_, ok := AmbientTx(ctx, second)
		assert.False(t, ok, "a transaction on one pool was ambient for another")

		require.NoError(t, WithinTx(ctx, second, func(ctx context.Context, secondTx *sql.Tx) error {
			assert.NotSame(t, firstTx, secondTx)
			got, ok := AmbientTx(ctx, first)
			assert.True(t, ok)
			assert.Same(t, firstTx, got)
			return insertRow(ctx, secondTx, "second")
		}))
		require.NoError(t, insertRow(ctx, firstTx, "first"))
		return sentinel
	})

	require.ErrorIs(t, err, sentinel)
	assert.Zero(t, countRows(t, first))
	assert.Equal(t, 1, countRows(t, second), "the second pool's unit committed on its own")
}

func TestAmbientTx_BareContextHasNone(t *testing.T) {
	t.Parallel()

	_, ok := AmbientTx(t.Context(), openTestPool(t))
	assert.False(t, ok)
}
