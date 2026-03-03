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

func TestHCStore_ListItemsBySessionFilter(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	defer func() { _ = database.Close() }()

	store := NewHCStore(database)
	now := time.Now()
	require.NoError(t, store.CreateItems(ctx, []hc.Item{
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

func TestHCStore_AddComment_and_ListComments(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	defer func() { _ = database.Close() }()

	store := NewHCStore(database)
	now := time.Now()
	require.NoError(t, store.CreateItems(ctx, []hc.Item{{
		ID:        "item-1",
		Title:     "Epic",
		Type:      hc.ItemTypeEpic,
		Status:    hc.StatusOpen,
		CreatedAt: now,
		UpdatedAt: now,
	}}))

	require.NoError(t, store.AddComment(ctx, hc.Comment{
		ID:        "cmt-1",
		ItemID:    "item-1",
		Message:   "design decision: use flat list",
		CreatedAt: now,
	}))

	comments, err := store.ListComments(ctx, "item-1")
	require.NoError(t, err)
	require.Len(t, comments, 1)
	assert.Equal(t, "cmt-1", comments[0].ID)
	assert.Equal(t, "design decision: use flat list", comments[0].Message)
}

func TestHCStore_UpdateItem_noAutoComment(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	defer func() { _ = database.Close() }()

	store := NewHCStore(database)
	now := time.Now()
	require.NoError(t, store.CreateItems(ctx, []hc.Item{{
		ID:        "item-1",
		Title:     "Epic",
		Type:      hc.ItemTypeEpic,
		Status:    hc.StatusOpen,
		CreatedAt: now,
		UpdatedAt: now,
	}}))

	status := hc.StatusInProgress
	_, err = store.UpdateItem(ctx, "item-1", hc.ItemUpdate{Status: &status})
	require.NoError(t, err)

	// UpdateItem must not auto-create comments.
	comments, err := store.ListComments(ctx, "item-1")
	require.NoError(t, err)
	assert.Empty(t, comments)
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
	require.NoError(t, store.CreateItems(ctx, items))

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
	require.NoError(t, store.CreateItems(ctx, items))

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

func TestHCStore_NextItem_returnsLeafTask(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	defer func() { _ = database.Close() }()

	store := NewHCStore(database)
	now := time.Now()

	// Items are unassigned (session_id = "") — NextItem claims the next open task.
	require.NoError(t, store.CreateItems(ctx, []hc.Item{
		{
			ID:        "epic-1",
			Title:     "Epic",
			Type:      hc.ItemTypeEpic,
			Status:    hc.StatusOpen,
			Depth:     0,
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:        "task-leaf",
			EpicID:    "epic-1",
			ParentID:  "epic-1",
			Title:     "Leaf Task",
			Type:      hc.ItemTypeTask,
			Status:    hc.StatusOpen,
			Depth:     1,
			CreatedAt: now,
			UpdatedAt: now,
		},
	}))

	// No session ID: falls back to claiming an unassigned open task.
	item, ok, err := store.NextItem(ctx, hc.NextFilter{})
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "task-leaf", item.ID)
}

func TestHCStore_NextItem_returnsNotFoundWhenEmpty(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	defer func() { _ = database.Close() }()

	store := NewHCStore(database)

	_, ok, err := store.NextItem(ctx, hc.NextFilter{SessionID: "sess-nobody"})
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestHCStore_NextItem_withEpicFilter(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	defer func() { _ = database.Close() }()

	store := NewHCStore(database)
	now := time.Now()

	// Items are unassigned — EpicID filter scopes to the correct epic.
	require.NoError(t, store.CreateItems(ctx, []hc.Item{
		{
			ID:        "epic-a",
			Title:     "Epic A",
			Type:      hc.ItemTypeEpic,
			Status:    hc.StatusOpen,
			Depth:     0,
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:        "task-a",
			EpicID:    "epic-a",
			ParentID:  "epic-a",
			Title:     "Task A",
			Type:      hc.ItemTypeTask,
			Status:    hc.StatusOpen,
			Depth:     1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:        "epic-b",
			Title:     "Epic B",
			Type:      hc.ItemTypeEpic,
			Status:    hc.StatusOpen,
			Depth:     0,
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:        "task-b",
			EpicID:    "epic-b",
			ParentID:  "epic-b",
			Title:     "Task B",
			Type:      hc.ItemTypeTask,
			Status:    hc.StatusOpen,
			Depth:     1,
			CreatedAt: now,
			UpdatedAt: now,
		},
	}))

	item, ok, err := store.NextItem(ctx, hc.NextFilter{EpicID: "epic-a"})
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "task-a", item.ID)
}

func TestHCStore_GetItem_notFound(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	defer func() { _ = database.Close() }()

	store := NewHCStore(database)

	_, err = store.GetItem(ctx, "nonexistent-id")
	require.Error(t, err)
	assert.ErrorIs(t, err, hc.ErrNotFound)
}

func TestHCStore_ListItems_EpicAndStatusFilter(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	defer func() { _ = database.Close() }()

	store := NewHCStore(database)
	now := time.Now()

	epicID := "epic-filter"
	require.NoError(t, store.CreateItems(ctx, []hc.Item{
		{
			ID:        epicID,
			Title:     "Filter Epic",
			Type:      hc.ItemTypeEpic,
			Status:    hc.StatusOpen,
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:        "task-open",
			EpicID:    epicID,
			ParentID:  epicID,
			Title:     "Open Task",
			Type:      hc.ItemTypeTask,
			Status:    hc.StatusOpen,
			Depth:     1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:        "task-done",
			EpicID:    epicID,
			ParentID:  epicID,
			Title:     "Done Task",
			Type:      hc.ItemTypeTask,
			Status:    hc.StatusDone,
			Depth:     1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:        "task-other-epic",
			EpicID:    "other-epic",
			ParentID:  "other-epic",
			Title:     "Other Epic Task",
			Type:      hc.ItemTypeTask,
			Status:    hc.StatusOpen,
			Depth:     1,
			CreatedAt: now,
			UpdatedAt: now,
		},
	}))

	open := hc.StatusOpen
	items, err := store.ListItems(ctx, hc.ListFilter{EpicID: epicID, Status: &open})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "task-open", items[0].ID)
}

func TestHCStore_NextItem_ResumeInProgress(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	defer func() { _ = database.Close() }()

	store := NewHCStore(database)
	now := time.Now()

	require.NoError(t, store.CreateItems(ctx, []hc.Item{
		{
			ID:        "epic-1",
			Title:     "Epic",
			Type:      hc.ItemTypeEpic,
			Status:    hc.StatusOpen,
			Depth:     0,
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:        "task-inprogress",
			EpicID:    "epic-1",
			ParentID:  "epic-1",
			Title:     "In Progress Task",
			Type:      hc.ItemTypeTask,
			Status:    hc.StatusInProgress,
			SessionID: "sess-1",
			Depth:     1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:        "task-open",
			EpicID:    "epic-1",
			ParentID:  "epic-1",
			Title:     "Open Task",
			Type:      hc.ItemTypeTask,
			Status:    hc.StatusOpen,
			Depth:     1,
			CreatedAt: now,
			UpdatedAt: now,
		},
	}))

	// With session ID, should resume the in_progress task.
	item, ok, err := store.NextItem(ctx, hc.NextFilter{SessionID: "sess-1", EpicID: "epic-1"})
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "task-inprogress", item.ID)
}

func TestHCStore_Blocked_ParentWithOpenChild(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	defer func() { _ = database.Close() }()

	store := NewHCStore(database)
	now := time.Now()

	// Epic with a task child that itself has an open grandchild — epic should be blocked.
	require.NoError(t, store.CreateItems(ctx, []hc.Item{
		{
			ID:        "epic-block",
			Title:     "Blocked Epic",
			Type:      hc.ItemTypeEpic,
			Status:    hc.StatusOpen,
			Depth:     0,
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:        "parent-task",
			EpicID:    "epic-block",
			ParentID:  "epic-block",
			Title:     "Parent Task",
			Type:      hc.ItemTypeTask,
			Status:    hc.StatusOpen,
			Depth:     1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:        "child-task",
			EpicID:    "epic-block",
			ParentID:  "parent-task",
			Title:     "Child Task",
			Type:      hc.ItemTypeTask,
			Status:    hc.StatusOpen,
			Depth:     2,
			CreatedAt: now,
			UpdatedAt: now,
		},
	}))

	items, err := store.ListItems(ctx, hc.ListFilter{})
	require.NoError(t, err)

	parentFound := false
	for _, item := range items {
		if item.ID == "parent-task" {
			parentFound = true
			assert.True(t, item.Blocked, "parent-task with open child should be blocked")
		}
	}
	assert.True(t, parentFound, "parent-task should be in list")
}

func TestHCStore_ListRepoKeys(t *testing.T) {
	ctx := context.Background()

	t.Run("returns distinct sorted repo keys", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := NewHCStore(database)
		now := time.Now()
		require.NoError(t, store.CreateItems(ctx, []hc.Item{
			{
				ID:        "item-1",
				RepoKey:   "org/repo-b",
				Title:     "Item 1",
				Type:      hc.ItemTypeEpic,
				Status:    hc.StatusOpen,
				CreatedAt: now,
				UpdatedAt: now,
			},
			{
				ID:        "item-2",
				RepoKey:   "org/repo-a",
				Title:     "Item 2",
				Type:      hc.ItemTypeEpic,
				Status:    hc.StatusOpen,
				CreatedAt: now,
				UpdatedAt: now,
			},
			{
				ID:        "item-3",
				RepoKey:   "org/repo-b",
				Title:     "Item 3 (duplicate repo)",
				Type:      hc.ItemTypeEpic,
				Status:    hc.StatusOpen,
				CreatedAt: now,
				UpdatedAt: now,
			},
		}))

		keys, err := store.ListRepoKeys(ctx)
		require.NoError(t, err)
		assert.Equal(t, []string{"org/repo-a", "org/repo-b"}, keys)
	})

	t.Run("returns empty slice when no items exist", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := NewHCStore(database)

		keys, err := store.ListRepoKeys(ctx)
		require.NoError(t, err)
		assert.Empty(t, keys)
	})

	t.Run("excludes items with empty repo_key", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := NewHCStore(database)
		now := time.Now()
		require.NoError(t, store.CreateItems(ctx, []hc.Item{
			{
				ID:        "item-empty",
				RepoKey:   "",
				Title:     "No repo key",
				Type:      hc.ItemTypeEpic,
				Status:    hc.StatusOpen,
				CreatedAt: now,
				UpdatedAt: now,
			},
			{
				ID:        "item-with-key",
				RepoKey:   "org/repo-c",
				Title:     "Has repo key",
				Type:      hc.ItemTypeEpic,
				Status:    hc.StatusOpen,
				CreatedAt: now,
				UpdatedAt: now,
			},
		}))

		keys, err := store.ListRepoKeys(ctx)
		require.NoError(t, err)
		assert.Equal(t, []string{"org/repo-c"}, keys)
	})
}

func TestHCStore_PruneScopesCommentsToStatusesAndItemUpdatedAt(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	defer func() { _ = database.Close() }()

	store := NewHCStore(database)
	now := time.Now()
	old := now.Add(-2 * time.Hour)
	recent := now.Add(-30 * time.Minute)

	require.NoError(t, store.CreateItems(ctx, []hc.Item{
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

	require.NoError(t, store.AddComment(ctx, hc.Comment{
		ID:        "cmt-done-old",
		ItemID:    "done-old",
		Message:   "handoff: stopping here",
		CreatedAt: old,
	}))
	require.NoError(t, store.AddComment(ctx, hc.Comment{
		ID:        "cmt-cancelled-old",
		ItemID:    "cancelled-old",
		Message:   "design decision: dropped scope",
		CreatedAt: old,
	}))
	require.NoError(t, store.AddComment(ctx, hc.Comment{
		ID:        "cmt-done-recent",
		ItemID:    "done-recent-update",
		Message:   "handoff: resuming tomorrow",
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

	doneOldComments, err := store.ListComments(ctx, "done-old")
	require.NoError(t, err)
	assert.Empty(t, doneOldComments)

	cancelledOldComments, err := store.ListComments(ctx, "cancelled-old")
	require.NoError(t, err)
	require.Len(t, cancelledOldComments, 1)
	assert.Equal(t, "cmt-cancelled-old", cancelledOldComments[0].ID)

	doneRecentComments, err := store.ListComments(ctx, "done-recent-update")
	require.NoError(t, err)
	require.Len(t, doneRecentComments, 1)
	assert.Equal(t, "cmt-done-recent", doneRecentComments[0].ID)
}
