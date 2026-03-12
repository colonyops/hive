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

func makeItem(id, epicID, parentID string, status hc.Status, depth int, now time.Time) hc.Item {
	return hc.Item{
		ID:        id,
		EpicID:    epicID,
		ParentID:  parentID,
		Title:     id,
		Type:      hc.ItemTypeTask,
		Status:    status,
		Depth:     depth,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func makeEpic(id string, now time.Time) hc.Item {
	return hc.Item{
		ID:        id,
		Title:     id,
		Type:      hc.ItemTypeEpic,
		Status:    hc.StatusOpen,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestFetchBlockedSet_ExplicitBlocker(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	defer func() { _ = database.Close() }()

	store := NewHCStore(database)
	now := time.Now()

	// Two tasks: blocker (open) blocks blocked
	epic := makeEpic("epic-1", now)
	blocker := makeItem("blocker", "epic-1", "epic-1", hc.StatusOpen, 1, now)
	blocked := makeItem("blocked", "epic-1", "epic-1", hc.StatusOpen, 1, now)

	require.NoError(t, store.CreateItems(ctx, []hc.Item{epic, blocker, blocked}))
	require.NoError(t, store.AddBlocker(ctx, "blocker", "blocked"))

	items, err := store.ListItems(ctx, hc.ListFilter{})
	require.NoError(t, err)

	found := false
	for _, item := range items {
		if item.ID == "blocked" {
			found = true
			assert.True(t, item.Blocked, "item with open explicit blocker should be blocked")
		}
	}
	assert.True(t, found, "blocked item should be in list")
}

func TestFetchBlockedSet_ClosedBlocker(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	defer func() { _ = database.Close() }()

	store := NewHCStore(database)
	now := time.Now()

	epic := makeEpic("epic-1", now)
	blocker := makeItem("blocker", "epic-1", "epic-1", hc.StatusDone, 1, now)
	blocked := makeItem("blocked", "epic-1", "epic-1", hc.StatusOpen, 1, now)

	require.NoError(t, store.CreateItems(ctx, []hc.Item{epic, blocker, blocked}))
	require.NoError(t, store.AddBlocker(ctx, "blocker", "blocked"))

	items, err := store.ListItems(ctx, hc.ListFilter{})
	require.NoError(t, err)

	for _, item := range items {
		if item.ID == "blocked" {
			assert.False(t, item.Blocked, "item with done blocker should not be blocked")
		}
	}
}

func TestListBlockers_EmptyWhenBlockersDone(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	defer func() { _ = database.Close() }()

	store := NewHCStore(database)
	now := time.Now()

	epic := makeEpic("epic-1", now)
	blocker := makeItem("blocker", "epic-1", "epic-1", hc.StatusOpen, 1, now)
	blocked := makeItem("blocked", "epic-1", "epic-1", hc.StatusOpen, 1, now)

	require.NoError(t, store.CreateItems(ctx, []hc.Item{epic, blocker, blocked}))
	require.NoError(t, store.AddBlocker(ctx, "blocker", "blocked"))

	// Initially, blocker is open — should appear
	ids, err := store.ListBlockers(ctx, "blocked")
	require.NoError(t, err)
	assert.Equal(t, []string{"blocker"}, ids)

	// Mark blocker as done
	done := hc.StatusDone
	_, err = store.UpdateItem(ctx, "blocker", hc.ItemUpdate{Status: &done})
	require.NoError(t, err)

	// Now ListBlockers should return empty
	ids, err = store.ListBlockers(ctx, "blocked")
	require.NoError(t, err)
	assert.Empty(t, ids, "closed blocker should not appear in ListBlockers")
}

func TestFetchHCItem_BlockerIDs(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	defer func() { _ = database.Close() }()

	store := NewHCStore(database)
	now := time.Now()

	epic := makeEpic("epic-1", now)
	blocker := makeItem("blocker", "epic-1", "epic-1", hc.StatusOpen, 1, now)
	blocked := makeItem("blocked", "epic-1", "epic-1", hc.StatusOpen, 1, now)

	require.NoError(t, store.CreateItems(ctx, []hc.Item{epic, blocker, blocked}))
	require.NoError(t, store.AddBlocker(ctx, "blocker", "blocked"))

	// GetItem uses fetchHCItem — should populate BlockerIDs
	item, err := store.GetItem(ctx, "blocked")
	require.NoError(t, err)
	assert.Equal(t, []string{"blocker"}, item.BlockerIDs)
	assert.True(t, item.Blocked)
}
