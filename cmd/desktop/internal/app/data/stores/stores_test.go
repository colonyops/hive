package stores

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStores_WithinTxJoinsTwoStoresAndCommits(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()

	err := st.WithinTx(ctx, func(ctx context.Context) error {
		if err := st.NodeKV.Set(ctx, "flow-1", "node-a", "k", "v", 0); err != nil {
			return err
		}
		if err := st.WebhookCaptures.Upsert(ctx, "source:flow-1/hook", 100, []byte(`{}`)); err != nil {
			return err
		}
		assert.Equal(t, 1, db.Conn().Stats().InUse, "both writes must share one connection")
		return nil
	})
	require.NoError(t, err)

	_, found, err := st.NodeKV.Get(ctx, "flow-1", "node-a", "k", 1)
	require.NoError(t, err)
	assert.True(t, found)
	_, err = st.WebhookCaptures.Get(ctx, "source:flow-1/hook")
	require.NoError(t, err)
}

func TestStores_WithinTxRollsBackBothWrites(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()
	errAbort := errors.New("abort")

	err := st.WithinTx(ctx, func(ctx context.Context) error {
		if err := st.NodeKV.Set(ctx, "flow-1", "node-a", "k", "v", 0); err != nil {
			return err
		}
		if err := st.WebhookCaptures.Upsert(ctx, "source:flow-1/hook", 100, []byte(`{}`)); err != nil {
			return err
		}
		return errAbort
	})
	require.ErrorIs(t, err, errAbort)

	_, found, err := st.NodeKV.Get(ctx, "flow-1", "node-a", "k", 1)
	require.NoError(t, err)
	assert.False(t, found, "the rollback must undo the node kv write")
	_, err = st.WebhookCaptures.Get(ctx, "source:flow-1/hook")
	assert.True(t, IsNotFound(err), "the rollback must undo the webhook capture write")
}

// TestStores_WithinTxJoinsAnAlreadyOpenTransaction proves a nested WithinTx
// joins the ambient transaction rather than opening a second one (which
// would busy-fail under this database's _txlock=immediate DSN and
// two-connection pool) and that only the outermost caller decides the
// outcome: the inner call returning nil must not commit the outer unit.
func TestStores_WithinTxJoinsAnAlreadyOpenTransaction(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()
	errAbort := errors.New("abort")

	err := st.WithinTx(ctx, func(outer context.Context) error {
		inner := st.WithinTx(outer, func(inner context.Context) error {
			return st.NodeKV.Set(inner, "flow-1", "node-a", "k", "v", 0)
		})
		require.NoError(t, inner)
		assert.Equal(t, 1, db.Conn().Stats().InUse, "the inner call must run on the outer transaction's connection")
		return errAbort
	})
	require.ErrorIs(t, err, errAbort)

	_, found, err := st.NodeKV.Get(ctx, "flow-1", "node-a", "k", 1)
	require.NoError(t, err)
	assert.False(t, found, "an inner WithinTx returning nil must not commit the outer transaction")
}
