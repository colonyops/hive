package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/hc"
	"github.com/colonyops/hive/internal/domain/tagset"
	"github.com/colonyops/hive/internal/store/db"
)

func TestHCStore_Tags(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	store := NewHCStore(db.OpenForTest(t))
	now := time.Now()
	require.NoError(t, store.CreateItems(ctx, []hc.Item{
		{ID: "a", Title: "A", Type: hc.ItemTypeEpic, Status: hc.StatusOpen, Tags: []string{"Backend", "q1"}, CreatedAt: now, UpdatedAt: now},
		{ID: "b", Title: "B", Type: hc.ItemTypeEpic, Status: hc.StatusOpen, Tags: []string{"backend"}, CreatedAt: now, UpdatedAt: now},
		{ID: "c", Title: "C", Type: hc.ItemTypeEpic, Status: hc.StatusOpen, CreatedAt: now, UpdatedAt: now},
	}))

	got, err := store.GetItem(ctx, "a")
	require.NoError(t, err)
	assert.Equal(t, []string{"Backend", "q1"}, got.Tags)

	untagged, err := store.GetItem(ctx, "c")
	require.NoError(t, err)
	assert.Equal(t, []string{}, untagged.Tags, "an untagged item reads as an empty list, not null")

	ids := func(filter hc.ListFilter) []string {
		items, err := store.ListItems(ctx, filter)
		require.NoError(t, err)
		var out []string
		for _, item := range items {
			out = append(out, item.ID)
		}
		return out
	}
	assert.ElementsMatch(t, []string{"a", "b"}, ids(hc.ListFilter{Tags: []string{"BACKEND"}}))
	assert.Equal(t, []string{"a"}, ids(hc.ListFilter{Tags: []string{"backend", "q1"}}), "every tag must match")

	updated, err := store.UpdateItem(ctx, "a", hc.ItemUpdate{AddTags: []string{"needs-review", "backend"}, RemoveTags: []string{"Q1"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"Backend", "needs-review"}, updated.Tags)

	cleared, err := store.UpdateItem(ctx, "a", hc.ItemUpdate{ClearTags: true, AddTags: []string{"fresh"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"fresh"}, cleared.Tags)

	_, err = store.UpdateItem(ctx, "a", hc.ItemUpdate{AddTags: []string{"a,b"}})
	require.ErrorIs(t, err, tagset.ErrInvalid)
	unchanged, err := store.GetItem(ctx, "a")
	require.NoError(t, err)
	assert.Equal(t, []string{"fresh"}, unchanged.Tags, "a refused edit changes nothing")
}
