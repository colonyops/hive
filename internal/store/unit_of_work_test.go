package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/domain/hc"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/store/db"
	"github.com/stretchr/testify/require"
)

// TestStores_JoinAnAmbientUnitOfWork: store methods on hive.db, including
// ones that open their own transaction, join a unit of work their caller
// opened, so a later failure rolls back every write in it.
func TestStores_JoinAnAmbientUnitOfWork(t *testing.T) {
	t.Parallel()

	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })

	sessions := NewSessionStore(database)
	items := NewHCStore(database)
	now := time.Now()
	sentinel := errors.New("unit failed")

	err = database.WithinTx(t.Context(), func(ctx context.Context, _ *db.DB) error {
		require.NoError(t, sessions.Save(ctx, session.Session{
			ID: "sess-1", Name: "s", Path: "/tmp/s", State: session.StateActive, CreatedAt: now, UpdatedAt: now,
		}))
		require.NoError(t, items.CreateItems(ctx, []hc.Item{{
			ID: "item-1", RepoKey: "repo", Title: "t", Type: hc.ItemTypeEpic, Status: hc.StatusOpen, CreatedAt: now, UpdatedAt: now,
		}}))
		_, err := items.GetItem(ctx, "item-1")
		require.NoError(t, err, "a read in the unit must see its earlier write")
		return sentinel
	})
	require.ErrorIs(t, err, sentinel)

	_, err = sessions.Get(t.Context(), "sess-1")
	require.ErrorIs(t, err, session.ErrNotFound)
	_, err = items.GetItem(t.Context(), "item-1")
	require.ErrorIs(t, err, hc.ErrNotFound)
}
