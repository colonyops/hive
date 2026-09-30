package queries

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func extTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(t.Context(), t.TempDir(), DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func countInboxItems(t *testing.T, db *DB) int {
	t.Helper()
	var n int
	require.NoError(t, db.Conn().QueryRowContext(t.Context(), `SELECT COUNT(*) FROM inbox_item`).Scan(&n))
	return n
}

func insertItem(ctx context.Context, db *DB, externalID string) error {
	_, err := db.InsertInboxItem(ctx, InsertInboxItemParams{
		ProfileID: "flow-1", SourceKind: "generic", SourceScope: "source-a",
		ExternalID: externalID, Payload: []byte(`{"v":1}`), Lifecycle: "active",
	})
	return err
}

func TestCtx_BareContextReturnsTheReceiver(t *testing.T) {
	t.Parallel()

	db := extTestDB(t)
	assert.Same(t, db, db.Ctx(t.Context()), "a context with no ambient transaction must not allocate a bound DB")
}

func TestCtx_AmbientTransactionReturnsABoundDB(t *testing.T) {
	t.Parallel()

	db := extTestDB(t)
	txCtx, tx, err := WithTransaction(t.Context(), db)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })

	bound := db.Ctx(txCtx)
	require.NotSame(t, db, bound)
	assert.Same(t, tx, bound.tx)
	assert.Same(t, tx, bound.querier(), "hand-written SQL on a bound DB must run in the transaction")
}

// TestWithinTx_JoinsRatherThanNesting is the property that keeps this from
// deadlocking. The pgx original always opens a transaction; on SQLite with
// _txlock=immediate and two connections, a second BEGIN IMMEDIATE while the
// first holds the write lock returns SQLITE_BUSY immediately.
//
// InUse staying at 1 is the observable that catches a regression to that
// behaviour: a nested implementation would check out a second connection.
func TestWithinTx_JoinsRatherThanNesting(t *testing.T) {
	t.Parallel()

	db := extTestDB(t)
	var inner int

	err := db.WithinTx(t.Context(), func(ctx context.Context, outerDB *DB) error {
		require.NoError(t, insertItem(ctx, outerDB, "item-outer"))
		return outerDB.WithinTx(ctx, func(ctx context.Context, innerDB *DB) error {
			inner = db.Conn().Stats().InUse
			return insertItem(ctx, innerDB, "item-inner")
		})
	})
	require.NoError(t, err)

	assert.Equal(t, 1, inner, "a nested WithinTx opened a second connection")
	assert.Equal(t, 2, countInboxItems(t, db), "both writes must land")
}

// TestWithinTx_NestedCommitsExactlyOnce: only the outermost caller commits, so
// an inner success is not durable until the outer one finishes.
func TestWithinTx_NestedCommitsExactlyOnce(t *testing.T) {
	t.Parallel()

	db := extTestDB(t)
	sentinel := errors.New("outer failed after the inner write")

	err := db.WithinTx(t.Context(), func(ctx context.Context, outerDB *DB) error {
		if err := outerDB.WithinTx(ctx, func(ctx context.Context, innerDB *DB) error {
			return insertItem(ctx, innerDB, "item-inner")
		}); err != nil {
			return err
		}
		return sentinel
	})

	require.ErrorIs(t, err, sentinel)
	assert.Zero(t, countInboxItems(t, db), "the inner WithinTx committed independently of its caller")
}

func TestWithinTx_ErrorRollsBackWithNoPartialWrite(t *testing.T) {
	t.Parallel()

	db := extTestDB(t)
	sentinel := errors.New("half way through")

	err := db.WithinTx(t.Context(), func(ctx context.Context, tx *DB) error {
		require.NoError(t, insertItem(ctx, tx, "item-1"))
		return sentinel
	})

	require.ErrorIs(t, err, sentinel)
	assert.Zero(t, countInboxItems(t, db))
}

func TestWithinTx_SuccessCommits(t *testing.T) {
	t.Parallel()

	db := extTestDB(t)
	require.NoError(t, db.WithinTx(t.Context(), func(ctx context.Context, tx *DB) error {
		return insertItem(ctx, tx, "item-1")
	}))
	assert.Equal(t, 1, countInboxItems(t, db))
}

// TestWithinTx_EntityMethodsUseOneConnection times out because a transaction
// escape blocks on SQLite's immediate write lock instead of failing.
func TestWithinTx_EntityMethodsUseOneConnection(t *testing.T) {
	t.Parallel()

	db := extTestDB(t)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	t.Cleanup(cancel)

	require.NoError(t, db.WithinTx(ctx, func(ctx context.Context, tx *DB) error {
		require.NoError(t, tx.NodeKVSet(ctx, "flow-1", "node-1", "key", `"value"`, 0))
		assert.Equal(t, 1, db.Conn().Stats().InUse, "NodeKVSet escaped the transaction")

		_, err := tx.Append(ctx, "source:flow-1/node-1", "event-1", []byte(`{"v":1}`))
		require.NoError(t, err)
		assert.Equal(t, 1, db.Conn().Stats().InUse, "Append escaped the transaction")
		return nil
	}))
}

func TestWithinTx_ReadSeesEarlierWrite(t *testing.T) {
	t.Parallel()

	db := extTestDB(t)
	require.NoError(t, db.WithinTx(t.Context(), func(ctx context.Context, tx *DB) error {
		require.NoError(t, insertItem(ctx, tx, "item-1"))
		items, err := tx.ListUnarchivedInboxItemsByProfile(ctx, "flow-1")
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "item-1", items[0].ExternalID)
		return nil
	}))
}

// TestWithinTx_ReleasesTheConnection guards the failure that would only show
// up under load: a transaction left open holds one of two connections
// forever.
func TestWithinTx_ReleasesTheConnection(t *testing.T) {
	t.Parallel()

	db := extTestDB(t)
	require.NoError(t, db.WithinTx(t.Context(), func(ctx context.Context, tx *DB) error {
		return insertItem(ctx, tx, "item-1")
	}))
	_ = db.WithinTx(t.Context(), func(ctx context.Context, tx *DB) error {
		return errors.New("rolled back")
	})

	assert.Zero(t, db.Conn().Stats().InUse, "a committed and a rolled-back transaction must both release their connection")
}

// TestWithTransaction_IsIdempotentOnAnAmbientContext: asking for a
// transaction when one is already ambient hands back the same one rather than
// starting a second.
func TestWithTransaction_IsIdempotentOnAnAmbientContext(t *testing.T) {
	t.Parallel()

	db := extTestDB(t)
	ctx, first, err := WithTransaction(t.Context(), db)
	require.NoError(t, err)
	t.Cleanup(func() { _ = first.Rollback() })

	sameCtx, second, err := WithTransaction(ctx, db)
	require.NoError(t, err)
	assert.Same(t, first, second)
	assert.Equal(t, ctx, sameCtx)
}
