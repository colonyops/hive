package stores

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/hc"
	"github.com/colonyops/hive/internal/data/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHCStore_UpdateItemLogsStatusChangeActivity(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	defer func() { _ = database.Close() }()

	store := NewHCStore(database)
	now := time.Now()
	require.NoError(t, store.CreateItem(ctx, hc.Item{
		ID:        "task-1",
		Title:     "Epic",
		Type:      hc.ItemTypeEpic,
		Status:    hc.StatusOpen,
		CreatedAt: now,
		UpdatedAt: now,
	}))

	status := hc.StatusInProgress
	_, err = store.UpdateItem(ctx, "task-1", hc.ItemUpdate{Status: &status})
	require.NoError(t, err)

	activity, err := store.ListActivity(ctx, "task-1")
	require.NoError(t, err)
	require.Len(t, activity, 1)
	assert.Equal(t, hc.ActivityTypeStatusChange, activity[0].Type)
	assert.Equal(t, "status changed from open to in_progress", activity[0].Message)
	assert.True(t, strings.HasPrefix(activity[0].ID, "hc-"))
	assert.NotContains(t, activity[0].ID, "act_")
}

func TestHCStore_UpdateItemDoesNotLogActivityWhenStatusUnchanged(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	defer func() { _ = database.Close() }()

	store := NewHCStore(database)
	now := time.Now()
	require.NoError(t, store.CreateItem(ctx, hc.Item{
		ID:        "task-2",
		Title:     "Epic",
		Type:      hc.ItemTypeEpic,
		Status:    hc.StatusOpen,
		SessionID: "session-a",
		CreatedAt: now,
		UpdatedAt: now,
	}))

	sessionID := "session-b"
	_, err = store.UpdateItem(ctx, "task-2", hc.ItemUpdate{SessionID: &sessionID})
	require.NoError(t, err)

	activity, err := store.ListActivity(ctx, "task-2")
	require.NoError(t, err)
	assert.Empty(t, activity)
}

func TestHCStore_ListItemsBySessionFilter(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	defer func() { _ = database.Close() }()

	store := NewHCStore(database)
	now := time.Now()
	require.NoError(t, store.CreateItemBatch(ctx, []hc.Item{
		{
			ID:        "task-a",
			Title:     "Task A",
			Type:      hc.ItemTypeEpic,
			Status:    hc.StatusOpen,
			SessionID: "session-a",
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:        "task-b",
			Title:     "Task B",
			Type:      hc.ItemTypeEpic,
			Status:    hc.StatusOpen,
			SessionID: "session-b",
			CreatedAt: now,
			UpdatedAt: now,
		},
	}))

	items, err := store.ListItems(ctx, hc.ListFilter{SessionID: "session-a"})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "task-a", items[0].ID)
}

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

func TestHCStore_PruneScopesActivityToStatusesAndItemUpdatedAt(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	defer func() { _ = database.Close() }()

	store := NewHCStore(database)
	now := time.Now()
	old := now.Add(-2 * time.Hour)
	recent := now.Add(-30 * time.Minute)

	require.NoError(t, store.CreateItemBatch(ctx, []hc.Item{
		{
			ID:        "done-old",
			Title:     "Done old",
			Type:      hc.ItemTypeEpic,
			Status:    hc.StatusDone,
			CreatedAt: old,
			UpdatedAt: old,
		},
		{
			ID:        "cancelled-old",
			Title:     "Cancelled old",
			Type:      hc.ItemTypeEpic,
			Status:    hc.StatusCancelled,
			CreatedAt: old,
			UpdatedAt: old,
		},
		{
			ID:        "done-recent-update",
			Title:     "Done recent update",
			Type:      hc.ItemTypeEpic,
			Status:    hc.StatusDone,
			CreatedAt: old,
			UpdatedAt: recent,
		},
	}))

	require.NoError(t, store.LogActivity(ctx, hc.Activity{
		ID:        "act-done-old",
		ItemID:    "done-old",
		Type:      hc.ActivityTypeUpdate,
		Message:   "done old activity",
		CreatedAt: old,
	}))
	require.NoError(t, store.LogActivity(ctx, hc.Activity{
		ID:        "act-cancelled-old",
		ItemID:    "cancelled-old",
		Type:      hc.ActivityTypeUpdate,
		Message:   "cancelled old activity",
		CreatedAt: old,
	}))
	require.NoError(t, store.LogActivity(ctx, hc.Activity{
		ID:        "act-done-recent",
		ItemID:    "done-recent-update",
		Type:      hc.ActivityTypeUpdate,
		Message:   "done recent activity",
		CreatedAt: old,
	}))

	count, err := store.Prune(ctx, hc.PruneOpts{
		OlderThan: time.Hour,
		Statuses:  []hc.Status{hc.StatusDone},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	remaining, err := store.ListItems(ctx, hc.ListFilter{})
	require.NoError(t, err)
	require.Len(t, remaining, 2)

	remainingIDs := map[string]struct{}{}
	for _, item := range remaining {
		remainingIDs[item.ID] = struct{}{}
	}
	_, hasCancelled := remainingIDs["cancelled-old"]
	_, hasDoneRecent := remainingIDs["done-recent-update"]
	assert.True(t, hasCancelled)
	assert.True(t, hasDoneRecent)

	doneOldActivity, err := store.ListActivity(ctx, "done-old")
	require.NoError(t, err)
	assert.Empty(t, doneOldActivity)

	cancelledOldActivity, err := store.ListActivity(ctx, "cancelled-old")
	require.NoError(t, err)
	require.Len(t, cancelledOldActivity, 1)
	assert.Equal(t, "act-cancelled-old", cancelledOldActivity[0].ID)

	doneRecentActivity, err := store.ListActivity(ctx, "done-recent-update")
	require.NoError(t, err)
	require.Len(t, doneRecentActivity, 1)
	assert.Equal(t, "act-done-recent", doneRecentActivity[0].ID)
}
