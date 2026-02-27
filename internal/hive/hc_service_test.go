package hive

import (
	"context"
	"testing"

	"github.com/colonyops/hive/internal/core/hc"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeHCStore implements hc.Store for unit testing.
// Unimplemented methods are provided by the embedded interface and will panic if called.
type fakeHCStore struct {
	hc.Store
	created   []hc.Item
	createErr error
}

func (f *fakeHCStore) CreateItem(_ context.Context, item hc.Item) error {
	if err := item.Validate(); err != nil {
		return err
	}
	f.created = append(f.created, item)
	return f.createErr
}

func (f *fakeHCStore) CreateItemBatch(_ context.Context, items []hc.Item) error {
	f.created = append(f.created, items...)
	return f.createErr
}

func newTestHCService(store hc.Store) *HCService {
	return NewHCService(store, nil, zerolog.Nop())
}

func TestHCService_CreateBulk_generatesIDs(t *testing.T) {
	ctx := context.Background()
	fake := &fakeHCStore{}
	svc := newTestHCService(fake)

	input := hc.CreateInput{
		Title: "My Epic",
		Type:  hc.ItemTypeEpic,
		Children: []hc.CreateInput{
			{Title: "Task A", Type: hc.ItemTypeTask},
			{Title: "Task B", Type: hc.ItemTypeTask},
		},
	}

	items, err := svc.CreateBulk(ctx, input, "owner/repo", "sess-1")
	require.NoError(t, err)
	require.Len(t, items, 3)

	// All IDs must be non-empty.
	for _, item := range items {
		assert.NotEmpty(t, item.ID, "item %q has empty ID", item.Title)
	}

	// Root (epic) must have empty EpicID.
	root := items[0]
	assert.Empty(t, root.EpicID, "root epic should have no EpicID")

	// Children must reference the root as their EpicID.
	for _, child := range items[1:] {
		assert.Equal(t, root.ID, child.EpicID, "child %q should reference root as EpicID", child.Title)
	}
}

func TestHCService_CreateBulk_BFS_depth(t *testing.T) {
	ctx := context.Background()
	fake := &fakeHCStore{}
	svc := newTestHCService(fake)

	input := hc.CreateInput{
		Title: "Root Epic",
		Type:  hc.ItemTypeEpic,
		Children: []hc.CreateInput{
			{
				Title: "Child Task",
				Type:  hc.ItemTypeTask,
				Children: []hc.CreateInput{
					{Title: "Grandchild Task", Type: hc.ItemTypeTask},
				},
			},
		},
	}

	items, err := svc.CreateBulk(ctx, input, "owner/repo", "sess-1")
	require.NoError(t, err)
	require.Len(t, items, 3)

	byTitle := make(map[string]hc.Item, len(items))
	for _, item := range items {
		byTitle[item.Title] = item
	}

	assert.Equal(t, 0, byTitle["Root Epic"].Depth, "root depth must be 0")
	assert.Equal(t, 1, byTitle["Child Task"].Depth, "direct child depth must be 1")
	assert.Equal(t, 2, byTitle["Grandchild Task"].Depth, "grandchild depth must be 2")
}

func TestHCService_CreateItem_validation(t *testing.T) {
	ctx := context.Background()
	fake := &fakeHCStore{}
	svc := newTestHCService(fake)

	// An item with an empty Title fails validation in the store; create still returns an error.
	item := hc.Item{
		ID:     "hc-test",
		Title:  "", // invalid
		Type:   hc.ItemTypeEpic,
		Status: hc.StatusOpen,
		Depth:  0,
	}

	err := svc.CreateItem(ctx, item)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "title is required")
	assert.Empty(t, fake.created, "invalid item should not be persisted")
}
