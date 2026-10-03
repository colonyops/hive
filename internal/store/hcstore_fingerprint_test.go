package store

import (
	"testing"
	"time"

	"github.com/colonyops/hive/internal/domain/hc"
	"github.com/colonyops/hive/internal/store/db"
	"github.com/stretchr/testify/require"
)

func TestHCStoreFingerprint_ChangesOnEveryWrite(t *testing.T) {
	ctx := t.Context()
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	store := NewHCStore(database)

	empty, err := store.Fingerprint(ctx)
	require.NoError(t, err)
	require.Equal(t, hc.Fingerprint{}, empty)

	now := time.Unix(1_700_000_000, 0)
	steps := []struct {
		name  string
		write func() error
	}{
		{"create", func() error {
			return store.CreateItems(ctx, []hc.Item{
				makeEpic("epic", now),
				makeItem("a", "epic", "epic", hc.StatusOpen, 1, now),
				makeItem("b", "epic", "epic", hc.StatusOpen, 1, now),
			})
		}},
		{"update", func() error {
			status := hc.StatusInProgress
			_, err := store.UpdateItem(ctx, "a", hc.ItemUpdate{Status: &status})
			return err
		}},
		{"comment", func() error {
			return store.AddComment(ctx, hc.Comment{ID: "c1", ItemID: "a", Message: "progress", CreatedAt: now})
		}},
		{"add blocker", func() error { return store.AddBlocker(ctx, "a", "b") }},
		{"remove blocker", func() error { return store.RemoveBlocker(ctx, "a", "b") }},
		{"delete", func() error { return store.DeleteItem(ctx, "b") }},
	}

	prev := empty
	for _, step := range steps {
		require.NoError(t, step.write(), step.name)
		next, err := store.Fingerprint(ctx)
		require.NoError(t, err, step.name)
		require.NotEqual(t, prev, next, "%s did not change the fingerprint", step.name)
		prev = next
	}
}
