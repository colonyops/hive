package hive

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/hc"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeHCStore implements hc.Store for unit testing.
// Unimplemented methods are provided by the embedded interface and will panic if called.
type fakeHCStore struct {
	hc.Store
	created                 []hc.Item
	createErr               error
	createBatchErr          error
	itemsByID               map[string]hc.Item
	updateItem              hc.Item
	updateErr               error
	listItems               []hc.Item
	listItemsErr            error
	lastListFilter          hc.ListFilter
	latestCheckpointByItem  map[string]hc.Activity
	latestCheckpointErrByID map[string]error
	logged                  []hc.Activity
	logErr                  error
	getItemErrByID          map[string]error
	getItemCalls            []string
	lastPruneOpts           hc.PruneOpts
	pruneCount              int
	pruneErr                error
}

func (f *fakeHCStore) CreateItem(_ context.Context, item hc.Item) error {
	if err := item.Validate(); err != nil {
		return err
	}
	if f.createErr != nil {
		return f.createErr
	}
	f.created = append(f.created, item)
	if f.itemsByID == nil {
		f.itemsByID = map[string]hc.Item{}
	}
	f.itemsByID[item.ID] = item
	return nil
}

func (f *fakeHCStore) CreateItemBatch(_ context.Context, items []hc.Item) error {
	if f.createBatchErr != nil {
		return f.createBatchErr
	}
	f.created = append(f.created, items...)
	return nil
}

func (f *fakeHCStore) GetItem(_ context.Context, id string) (hc.Item, error) {
	f.getItemCalls = append(f.getItemCalls, id)
	if err, ok := f.getItemErrByID[id]; ok {
		return hc.Item{}, err
	}
	item, ok := f.itemsByID[id]
	if !ok {
		return hc.Item{}, errors.New("not found")
	}
	return item, nil
}

func (f *fakeHCStore) UpdateItem(_ context.Context, _ string, _ hc.ItemUpdate) (hc.Item, error) {
	if f.updateErr != nil {
		return hc.Item{}, f.updateErr
	}
	return f.updateItem, nil
}

func (f *fakeHCStore) ListItems(_ context.Context, filter hc.ListFilter) ([]hc.Item, error) {
	f.lastListFilter = filter
	if f.listItemsErr != nil {
		return nil, f.listItemsErr
	}
	return f.listItems, nil
}

func (f *fakeHCStore) LatestCheckpoint(_ context.Context, itemID string) (hc.Activity, bool, error) {
	if err, ok := f.latestCheckpointErrByID[itemID]; ok {
		return hc.Activity{}, false, err
	}
	cp, ok := f.latestCheckpointByItem[itemID]
	if !ok {
		return hc.Activity{}, false, nil
	}
	return cp, true, nil
}

func (f *fakeHCStore) LogActivity(_ context.Context, a hc.Activity) error {
	if f.logErr != nil {
		return f.logErr
	}
	f.logged = append(f.logged, a)
	return nil
}

func (f *fakeHCStore) Prune(_ context.Context, opts hc.PruneOpts) (int, error) {
	f.lastPruneOpts = opts
	if f.pruneErr != nil {
		return 0, f.pruneErr
	}
	return f.pruneCount, nil
}

func newTestHoneycombService(store hc.Store) *HoneycombService {
	return NewHoneycombService(store, zerolog.Nop())
}

func TestHoneycombService_CreateBulk_generatesIDs(t *testing.T) {
	ctx := context.Background()
	fake := &fakeHCStore{}
	svc := newTestHoneycombService(fake)

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

func TestHoneycombService_CreateBulk_BFS_depth(t *testing.T) {
	ctx := context.Background()
	fake := &fakeHCStore{}
	svc := newTestHoneycombService(fake)

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

func TestHoneycombService_CreateItem_validation(t *testing.T) {
	ctx := context.Background()
	fake := &fakeHCStore{}
	svc := newTestHoneycombService(fake)

	_, err := svc.CreateItem(ctx, hc.CreateItemInput{Title: "", Type: hc.ItemTypeEpic}, "owner/repo", "sess-1")
	require.EqualError(t, err, "title is required")
	assert.Empty(t, fake.created, "invalid item should not be persisted")
}

func TestHoneycombService_CreateItem_withParentTaskDerivesEpicAndDepth(t *testing.T) {
	ctx := context.Background()
	fake := &fakeHCStore{
		itemsByID: map[string]hc.Item{
			"hc-parent": {
				ID:     "hc-parent",
				EpicID: "hc-epic",
				Title:  "Parent Task",
				Type:   hc.ItemTypeTask,
				Status: hc.StatusOpen,
				Depth:  1,
			},
		},
	}
	svc := newTestHoneycombService(fake)

	item, err := svc.CreateItem(ctx, hc.CreateItemInput{
		Title:    "Child Task",
		Type:     hc.ItemTypeTask,
		ParentID: "hc-parent",
	}, "owner/repo", "sess-1")
	require.NoError(t, err)

	assert.Equal(t, "hc-parent", item.ParentID)
	assert.Equal(t, "hc-epic", item.EpicID)
	assert.Equal(t, 2, item.Depth)
	require.Len(t, fake.created, 1)
	assert.Equal(t, item.ID, fake.created[0].ID)
}

func TestHoneycombService_UpdateItem_noError(t *testing.T) {
	ctx := context.Background()
	fake := &fakeHCStore{updateItem: hc.Item{ID: "hc-task", EpicID: "hc-epic", Status: hc.StatusInProgress}}
	svc := newTestHoneycombService(fake)

	updated, err := svc.UpdateItem(ctx, "hc-task", hc.ItemUpdate{})
	require.NoError(t, err)
	assert.Equal(t, "hc-task", updated.ID)
}

func TestHoneycombService_Context_countsAndFiltering(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	fake := &fakeHCStore{
		itemsByID: map[string]hc.Item{
			"hc-epic": {ID: "hc-epic", Title: "Epic", Type: hc.ItemTypeEpic, Status: hc.StatusOpen, Depth: 0},
		},
		listItems: []hc.Item{
			{ID: "hc-open", Title: "Open", Type: hc.ItemTypeTask, Status: hc.StatusOpen, SessionID: "sess-1", EpicID: "hc-epic"},
			{ID: "hc-ip", Title: "In Progress", Type: hc.ItemTypeTask, Status: hc.StatusInProgress, SessionID: "sess-1", EpicID: "hc-epic"},
			{ID: "hc-done", Title: "Done", Type: hc.ItemTypeTask, Status: hc.StatusDone, SessionID: "sess-1", EpicID: "hc-epic"},
			{ID: "hc-cancelled", Title: "Cancelled", Type: hc.ItemTypeTask, Status: hc.StatusCancelled, SessionID: "sess-2", EpicID: "hc-epic"},
			{ID: "hc-other", Title: "Other Session", Type: hc.ItemTypeTask, Status: hc.StatusOpen, SessionID: "sess-2", EpicID: "hc-epic"},
		},
		latestCheckpointByItem: map[string]hc.Activity{
			"hc-open": {ID: "hc-cp", ItemID: "hc-open", Type: hc.ActivityTypeCheckpoint, Message: "cp", CreatedAt: now},
		},
	}
	svc := newTestHoneycombService(fake)

	block, err := svc.Context(ctx, "hc-epic", "sess-1")
	require.NoError(t, err)

	assert.Equal(t, hc.ListFilter{EpicID: "hc-epic"}, fake.lastListFilter)
	assert.Equal(t, 2, block.Counts.Open)
	assert.Equal(t, 1, block.Counts.InProgress)
	assert.Equal(t, 1, block.Counts.Done)
	assert.Equal(t, 1, block.Counts.Cancelled)
	require.Len(t, block.MyTasks, 2)
	assert.Equal(t, "hc-open", block.MyTasks[0].Item.ID)
	assert.Equal(t, "cp", block.MyTasks[0].LatestCheckpoint.Message)
	assert.Equal(t, "hc-ip", block.MyTasks[1].Item.ID)
	assert.Empty(t, block.MyTasks[1].LatestCheckpoint.Message)
	require.Len(t, block.AllOpenTasks, 3)
}

func TestHoneycombService_Prune_forwardsOptionsIncludingDryRun(t *testing.T) {
	ctx := context.Background()
	fake := &fakeHCStore{pruneCount: 4}
	svc := newTestHoneycombService(fake)

	opts := hc.PruneOpts{OlderThan: 24 * time.Hour, Statuses: []hc.Status{hc.StatusDone}, DryRun: true}
	n, err := svc.Prune(ctx, opts)
	require.NoError(t, err)
	assert.Equal(t, 4, n)
	assert.Equal(t, opts, fake.lastPruneOpts)

	opts.DryRun = false
	_, err = svc.Prune(ctx, opts)
	require.NoError(t, err)
	assert.False(t, fake.lastPruneOpts.DryRun)
}

func TestHoneycombService_LogActivity_returnsWrappedError(t *testing.T) {
	ctx := context.Background()
	fake := &fakeHCStore{logErr: errors.New("db write failed")}
	svc := newTestHoneycombService(fake)

	_, err := svc.LogActivity(ctx, "hc-item", hc.ActivityTypeComment, "msg")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `log hc activity for "hc-item": db write failed`)
}

func TestHoneycombService_LogActivity_success(t *testing.T) {
	ctx := context.Background()
	fake := &fakeHCStore{}
	svc := newTestHoneycombService(fake)

	a, err := svc.LogActivity(ctx, "hc-item", hc.ActivityTypeComment, "msg")
	require.NoError(t, err)
	assert.Equal(t, "hc-item", a.ItemID)
	require.Len(t, fake.logged, 1)
}
