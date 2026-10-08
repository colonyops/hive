package db

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openUnitTestDB(t *testing.T) *DB {
	t.Helper()
	database, err := Open(t.TempDir(), DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func setKey(ctx context.Context, database *DB, key string) error {
	return database.Ctx(ctx).KVSet(ctx, KVSetParams{Key: key, Value: []byte(`"v"`)})
}

func countKeys(t *testing.T, database *DB) int {
	t.Helper()
	var n int
	require.NoError(t, database.Conn().QueryRowContext(t.Context(), `SELECT COUNT(*) FROM kv_store`).Scan(&n))
	return n
}

func TestCtx_BareContextReturnsTheReceiver(t *testing.T) {
	t.Parallel()

	database := openUnitTestDB(t)
	assert.Same(t, database, database.Ctx(t.Context()))
}

func TestCtx_AmbientTransactionReturnsABoundDB(t *testing.T) {
	t.Parallel()

	database := openUnitTestDB(t)
	require.NoError(t, database.WithinTx(t.Context(), func(ctx context.Context, tx *DB) error {
		bound := database.Ctx(ctx)
		require.NotSame(t, database, bound)
		assert.NotSame(t, database.Queries, bound.Queries)
		assert.Equal(t, tx.Queries, bound.Queries, "the bound queries must run in the transaction")
		return nil
	}))
}

func TestWithinTx_NestedWorkJoinsAndCommitsOnce(t *testing.T) {
	t.Parallel()

	database := openUnitTestDB(t)
	err := database.WithinTx(t.Context(), func(ctx context.Context, _ *DB) error {
		require.NoError(t, setKey(ctx, database, "outer"))
		return database.WithinTx(ctx, func(ctx context.Context, _ *DB) error {
			assert.Equal(t, 1, database.Conn().Stats().InUse, "the nested call opened a second connection")
			return setKey(ctx, database, "inner")
		})
	})
	require.NoError(t, err)
	assert.Equal(t, 2, countKeys(t, database))
}

func TestWithinTx_NestedFailureRollsBackTheOuterUnit(t *testing.T) {
	t.Parallel()

	database := openUnitTestDB(t)
	sentinel := errors.New("inner failed")
	err := database.WithinTx(t.Context(), func(ctx context.Context, _ *DB) error {
		require.NoError(t, setKey(ctx, database, "outer"))
		return database.WithinTx(ctx, func(ctx context.Context, _ *DB) error {
			require.NoError(t, setKey(ctx, database, "inner"))
			return sentinel
		})
	})
	require.ErrorIs(t, err, sentinel)
	assert.Zero(t, countKeys(t, database))
}

func TestWithinTx_AnotherHiveDBDoesNotJoin(t *testing.T) {
	t.Parallel()

	first := openUnitTestDB(t)
	second := openUnitTestDB(t)
	sentinel := errors.New("first unit failed")

	err := first.WithinTx(t.Context(), func(ctx context.Context, _ *DB) error {
		assert.Same(t, second, second.Ctx(ctx))
		require.NoError(t, setKey(ctx, second, "second"))
		require.NoError(t, setKey(ctx, first, "first"))
		return sentinel
	})
	require.ErrorIs(t, err, sentinel)
	assert.Zero(t, countKeys(t, first))
	assert.Equal(t, 1, countKeys(t, second))
}
