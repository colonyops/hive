package stores

import (
	"context"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/hc"
	"github.com/colonyops/hive/internal/data/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHCStore_PruneRemovesDescendantsOfPrunedRoot(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	defer func() { _ = database.Close() }()

	store := NewHCStore(database)
	now := time.Now()
	old := now.Add(-2 * time.Hour)
	recent := now.Add(-30 * time.Minute)

	items := []hc.Item{
		{
			ID:        "epic-old",
			Title:     "Old Epic",
			Type:      hc.ItemTypeEpic,
			Status:    hc.StatusDone,
			Depth:     0,
			CreatedAt: old,
			UpdatedAt: old,
		},
		{
			ID:        "task-old-child",
			EpicID:    "epic-old",
			ParentID:  "epic-old",
			Title:     "Old Child",
			Type:      hc.ItemTypeTask,
			Status:    hc.StatusOpen,
			Depth:     1,
			CreatedAt: old,
			UpdatedAt: old,
		},
		{
			ID:        "task-old-grandchild",
			EpicID:    "epic-old",
			ParentID:  "task-old-child",
			Title:     "Old Grandchild",
			Type:      hc.ItemTypeTask,
			Status:    hc.StatusInProgress,
			Depth:     2,
			CreatedAt: old,
			UpdatedAt: old,
		},
		{
			ID:        "epic-recent",
			Title:     "Recent Epic",
			Type:      hc.ItemTypeEpic,
			Status:    hc.StatusDone,
			Depth:     0,
			CreatedAt: recent,
			UpdatedAt: recent,
		},
		{
			ID:        "task-recent-child",
			EpicID:    "epic-recent",
			ParentID:  "epic-recent",
			Title:     "Recent Child",
			Type:      hc.ItemTypeTask,
			Status:    hc.StatusOpen,
			Depth:     1,
			CreatedAt: recent,
			UpdatedAt: recent,
		},
	}
	require.NoError(t, store.CreateItemBatch(ctx, items))

	count, err := store.Prune(ctx, hc.PruneOpts{
		OlderThan: time.Hour,
		Statuses:  []hc.Status{hc.StatusDone},
	})
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	remaining, err := store.ListItems(ctx, hc.ListFilter{})
	require.NoError(t, err)
	require.Len(t, remaining, 2)

	remainingIDs := map[string]struct{}{}
	for _, item := range remaining {
		remainingIDs[item.ID] = struct{}{}
	}
	_, hasRecentEpic := remainingIDs["epic-recent"]
	_, hasRecentChild := remainingIDs["task-recent-child"]
	assert.True(t, hasRecentEpic)
	assert.True(t, hasRecentChild)
}

func TestHCStore_PruneDryRunIncludesDescendants(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	defer func() { _ = database.Close() }()

	store := NewHCStore(database)
	old := time.Now().Add(-2 * time.Hour)

	items := []hc.Item{
		{
			ID:        "epic-old",
			Title:     "Old Epic",
			Type:      hc.ItemTypeEpic,
			Status:    hc.StatusDone,
			Depth:     0,
			CreatedAt: old,
			UpdatedAt: old,
		},
		{
			ID:        "task-old-child",
			EpicID:    "epic-old",
			ParentID:  "epic-old",
			Title:     "Old Child",
			Type:      hc.ItemTypeTask,
			Status:    hc.StatusOpen,
			Depth:     1,
			CreatedAt: old,
			UpdatedAt: old,
		},
	}
	require.NoError(t, store.CreateItemBatch(ctx, items))

	count, err := store.Prune(ctx, hc.PruneOpts{
		OlderThan: time.Hour,
		Statuses:  []hc.Status{hc.StatusDone},
		DryRun:    true,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	remaining, err := store.ListItems(ctx, hc.ListFilter{})
	require.NoError(t, err)
	assert.Len(t, remaining, 2)
}
