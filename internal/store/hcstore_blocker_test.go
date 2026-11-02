package store

import (
	"context"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/domain/hc"
	"github.com/colonyops/hive/internal/store/db"
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

func TestAddBlocker_RefusesACycleAndKeepsTheEdges(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		edges   [][2]string
		blocker string
		blocked string
		wantErr error
	}{
		{name: "self", blocker: "a", blocked: "a", wantErr: hc.ErrCyclicDependency},
		{name: "direct", edges: [][2]string{{"a", "b"}}, blocker: "b", blocked: "a", wantErr: hc.ErrCyclicDependency},
		{name: "indirect", edges: [][2]string{{"a", "b"}, {"b", "c"}}, blocker: "c", blocked: "a", wantErr: hc.ErrCyclicDependency},
		{name: "chain", edges: [][2]string{{"a", "b"}}, blocker: "b", blocked: "c"},
		{name: "diamond", edges: [][2]string{{"a", "c"}, {"b", "c"}}, blocker: "a", blocked: "b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
			require.NoError(t, err)
			defer func() { _ = database.Close() }()
			store := NewHCStore(database)
			now := time.Now()
			require.NoError(t, store.CreateItems(ctx, []hc.Item{
				makeEpic("epic-1", now),
				makeItem("a", "epic-1", "epic-1", hc.StatusOpen, 1, now),
				makeItem("b", "epic-1", "epic-1", hc.StatusOpen, 1, now),
				makeItem("c", "epic-1", "epic-1", hc.StatusOpen, 1, now),
			}))
			for _, edge := range tt.edges {
				require.NoError(t, store.AddBlocker(ctx, edge[0], edge[1]))
			}

			err = store.AddBlocker(ctx, tt.blocker, tt.blocked)

			edges, listErr := store.ListBlockerEdges(ctx)
			require.NoError(t, listErr)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Len(t, edges, len(tt.edges))
				return
			}
			require.NoError(t, err)
			assert.Contains(t, edges, [2]string{tt.blocker, tt.blocked})
		})
	}
}

func TestFetchBlockedSet_ExplicitBlocker(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
