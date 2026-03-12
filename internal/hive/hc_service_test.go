package hive

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/hc"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeHCStore is an in-memory implementation of hc.Store for testing.
type fakeHCStore struct {
	items         map[string]hc.Item
	comments      map[string][]hc.Comment // keyed by itemID
	forceErr      error                   // when non-nil, all mutating methods return this error
	lastPruneOpts hc.PruneOpts
	pruneCount    int
}

var _ hc.Store = (*fakeHCStore)(nil)

func newFakeHCStore() *fakeHCStore {
	return &fakeHCStore{
		items:    make(map[string]hc.Item),
		comments: make(map[string][]hc.Comment),
	}
}

func (f *fakeHCStore) CreateItems(_ context.Context, items []hc.Item) error {
	if f.forceErr != nil {
		return f.forceErr
	}
	for _, item := range items {
		if err := item.Validate(); err != nil {
			return err
		}
		f.items[item.ID] = item
	}
	return nil
}

func (f *fakeHCStore) GetItem(_ context.Context, id string) (hc.Item, error) {
	item, ok := f.items[id]
	if !ok {
		return hc.Item{}, errors.New("item not found: " + id)
	}
	return item, nil
}

func (f *fakeHCStore) UpdateItem(_ context.Context, id string, update hc.ItemUpdate) (hc.Item, error) {
	if f.forceErr != nil {
		return hc.Item{}, f.forceErr
	}
	item, ok := f.items[id]
	if !ok {
		return hc.Item{}, errors.New("item not found: " + id)
	}
	if update.Status != nil {
		item.Status = *update.Status
	}
	if update.SessionID != nil {
		item.SessionID = *update.SessionID
	}
	if update.Title != nil {
		item.Title = *update.Title
	}
	if update.Desc != nil {
		item.Desc = *update.Desc
	}
	item.UpdatedAt = time.Now()
	f.items[id] = item
	return item, nil
}

func (f *fakeHCStore) BulkUpdateStatus(_ context.Context, epicID string, status hc.Status) error {
	if f.forceErr != nil {
		return f.forceErr
	}
	for id, item := range f.items {
		if item.EpicID == epicID && item.Status != hc.StatusDone && item.Status != hc.StatusCancelled {
			item.Status = status
			item.UpdatedAt = time.Now()
			f.items[id] = item
		}
	}
	return nil
}

func (f *fakeHCStore) ListItems(_ context.Context, filter hc.ListFilter) ([]hc.Item, error) {
	var result []hc.Item
	for _, item := range f.items {
		if filter.EpicID != "" && item.EpicID != filter.EpicID {
			continue
		}
		if filter.RepoKey != "" && item.RepoKey != filter.RepoKey {
			continue
		}
		if filter.SessionID != "" && item.SessionID != filter.SessionID {
			continue
		}
		if filter.Status != nil && item.Status != *filter.Status {
			continue
		}
		result = append(result, item)
	}
	return result, nil
}

func (f *fakeHCStore) NextItem(_ context.Context, filter hc.NextFilter) (hc.Item, bool, error) {
	for _, item := range f.items {
		if item.Type == hc.ItemTypeTask &&
			(item.Status == hc.StatusOpen || item.Status == hc.StatusInProgress) {
			if filter.EpicID != "" && item.EpicID != filter.EpicID {
				continue
			}
			if filter.SessionID != "" && item.SessionID != filter.SessionID {
				continue
			}
			return item, true, nil
		}
	}
	return hc.Item{}, false, nil
}

func (f *fakeHCStore) DeleteItem(_ context.Context, id string) error {
	if f.forceErr != nil {
		return f.forceErr
	}
	delete(f.items, id)
	return nil
}

func (f *fakeHCStore) AddComment(_ context.Context, c hc.Comment) error {
	if f.forceErr != nil {
		return f.forceErr
	}
	f.comments[c.ItemID] = append(f.comments[c.ItemID], c)
	return nil
}

func (f *fakeHCStore) ListComments(_ context.Context, itemID string) ([]hc.Comment, error) {
	return f.comments[itemID], nil
}

func (f *fakeHCStore) Prune(_ context.Context, opts hc.PruneOpts) (int, error) {
	if f.forceErr != nil {
		return 0, f.forceErr
	}
	f.lastPruneOpts = opts
	return f.pruneCount, nil
}

func (f *fakeHCStore) ListRepoKeys(_ context.Context) ([]string, error) {
	if f.forceErr != nil {
		return nil, f.forceErr
	}
	seen := make(map[string]struct{})
	for _, item := range f.items {
		if item.RepoKey != "" {
			seen[item.RepoKey] = struct{}{}
		}
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	return keys, nil
}

func (f *fakeHCStore) CreateBulkWithEdges(_ context.Context, items []hc.Item, _ [][2]string) error {
	if f.forceErr != nil {
		return f.forceErr
	}
	for _, item := range items {
		if err := item.Validate(); err != nil {
			return err
		}
		f.items[item.ID] = item
	}
	return nil
}

func (f *fakeHCStore) AddBlocker(_ context.Context, blockerID, blockedID string) error {
	return nil
}

func (f *fakeHCStore) RemoveBlocker(_ context.Context, blockerID, blockedID string) error {
	return nil
}

func (f *fakeHCStore) ListBlockers(_ context.Context, itemID string) ([]string, error) {
	return nil, nil
}

func (f *fakeHCStore) ListBlockerEdges(_ context.Context) ([][2]string, error) {
	return nil, nil
}

func newTestHoneycombService(store hc.Store) *HoneycombService {
	return NewHoneycombService(store, zerolog.Nop())
}

// ---------------------------------------------------------------------------
// CreateBulk tests
// ---------------------------------------------------------------------------

func TestCreateBulk_IDGeneration(t *testing.T) {
	store := newFakeHCStore()
	svc := newTestHoneycombService(store)

	input := hc.CreateInput{
		Title: "My Epic",
		Type:  hc.ItemTypeEpic,
		Children: []hc.CreateInput{
			{
				Title: "Task A",
				Type:  hc.ItemTypeTask,
				Children: []hc.CreateInput{
					{Title: "Sub-task A1", Type: hc.ItemTypeTask},
				},
			},
			{
				Title: "Task B",
				Type:  hc.ItemTypeTask,
			},
		},
	}

	items, err := svc.CreateBulk(context.Background(), "owner/repo", input)
	require.NoError(t, err)
	require.Len(t, items, 4)

	epic := items[0]
	assert.Equal(t, hc.ItemTypeEpic, epic.Type)
	assert.Empty(t, epic.EpicID, "epic should have no EpicID")
	assert.Equal(t, 0, epic.Depth)

	var depth1 []hc.Item
	var depth2 []hc.Item
	for _, it := range items[1:] {
		switch it.Depth {
		case 1:
			depth1 = append(depth1, it)
		case 2:
			depth2 = append(depth2, it)
		}
	}

	assert.Len(t, depth1, 2, "expected 2 tasks at depth 1")
	assert.Len(t, depth2, 1, "expected 1 task at depth 2")

	for _, it := range depth1 {
		assert.Equal(t, epic.ID, it.EpicID)
		assert.Equal(t, epic.ID, it.ParentID)
	}

	subTask := depth2[0]
	assert.Equal(t, epic.ID, subTask.EpicID)
	assert.Equal(t, 2, subTask.Depth)

	for _, it := range items {
		assert.True(t, strings.HasPrefix(it.ID, "hc-"), "ID %q missing hc- prefix", it.ID)
	}
}

func TestCreateBulk_RejectsNonEpicRoot(t *testing.T) {
	store := newFakeHCStore()
	svc := newTestHoneycombService(store)

	_, err := svc.CreateBulk(context.Background(), "owner/repo", hc.CreateInput{
		Title: "Just a task",
		Type:  hc.ItemTypeTask,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "epic")
}

func TestCreateBulk_PropagatesBatchError(t *testing.T) {
	store := newFakeHCStore()
	store.forceErr = errors.New("db exploded")
	svc := newTestHoneycombService(store)

	_, err := svc.CreateBulk(context.Background(), "owner/repo", hc.CreateInput{
		Title: "Epic",
		Type:  hc.ItemTypeEpic,
	})
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// CreateItem tests
// ---------------------------------------------------------------------------

func TestCreateItem_WithEpicParent(t *testing.T) {
	store := newFakeHCStore()
	svc := newTestHoneycombService(store)

	epic := hc.Item{
		ID:        hc.GenerateID(),
		RepoKey:   "owner/repo",
		Title:     "Parent Epic",
		Type:      hc.ItemTypeEpic,
		Status:    hc.StatusOpen,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	store.items[epic.ID] = epic

	item, err := svc.CreateItem(context.Background(), "owner/repo", hc.CreateItemInput{
		Title:    "Child Task",
		Type:     hc.ItemTypeTask,
		ParentID: epic.ID,
	})
	require.NoError(t, err)

	assert.Equal(t, epic.ID, item.EpicID)
	assert.Equal(t, epic.ID, item.ParentID)
	assert.Equal(t, 1, item.Depth)
}

func TestCreateItem_WithTaskParent(t *testing.T) {
	store := newFakeHCStore()
	svc := newTestHoneycombService(store)

	epicID := hc.GenerateID()

	epic := hc.Item{
		ID:        epicID,
		RepoKey:   "owner/repo",
		Title:     "Epic",
		Type:      hc.ItemTypeEpic,
		Status:    hc.StatusOpen,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	store.items[epic.ID] = epic

	parent := hc.Item{
		ID:        hc.GenerateID(),
		RepoKey:   "owner/repo",
		EpicID:    epicID,
		ParentID:  epicID,
		Title:     "Parent Task",
		Type:      hc.ItemTypeTask,
		Status:    hc.StatusOpen,
		Depth:     1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	store.items[parent.ID] = parent

	item, err := svc.CreateItem(context.Background(), "owner/repo", hc.CreateItemInput{
		Title:    "Sub Task",
		Type:     hc.ItemTypeTask,
		ParentID: parent.ID,
	})
	require.NoError(t, err)

	assert.Equal(t, parent.EpicID, item.EpicID)
	assert.Equal(t, parent.ID, item.ParentID)
	assert.Equal(t, parent.Depth+1, item.Depth)
}

func TestCreateItem_EmptyTitle(t *testing.T) {
	store := newFakeHCStore()
	svc := newTestHoneycombService(store)

	_, err := svc.CreateItem(context.Background(), "owner/repo", hc.CreateItemInput{
		Title: "",
		Type:  hc.ItemTypeTask,
	})
	require.Error(t, err)
	assert.Empty(t, store.items, "store should not be called when title is empty")
}

// ---------------------------------------------------------------------------
// UpdateItem tests
// ---------------------------------------------------------------------------

func TestUpdateItem_NoError(t *testing.T) {
	store := newFakeHCStore()
	svc := newTestHoneycombService(store)

	epicID := hc.GenerateID()
	epic := hc.Item{
		ID:        epicID,
		RepoKey:   "owner/repo",
		Title:     "Epic",
		Type:      hc.ItemTypeEpic,
		Status:    hc.StatusOpen,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	store.items[epic.ID] = epic

	status := hc.StatusDone
	updated, err := svc.UpdateItem(context.Background(), epicID, hc.ItemUpdate{Status: &status})
	require.NoError(t, err)
	assert.Equal(t, hc.StatusDone, updated.Status)
}

// ---------------------------------------------------------------------------
// Context tests
// ---------------------------------------------------------------------------

func seedEpicWithTasks(store *fakeHCStore, sessionID string) hc.Item {
	epicID := hc.GenerateID()
	epic := hc.Item{
		ID:        epicID,
		RepoKey:   "owner/repo",
		Title:     "Test Epic",
		Type:      hc.ItemTypeEpic,
		Status:    hc.StatusOpen,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	store.items[epicID] = epic

	statuses := []hc.Status{hc.StatusOpen, hc.StatusInProgress, hc.StatusDone, hc.StatusCancelled}
	for i, st := range statuses {
		sid := ""
		if i < 2 {
			sid = sessionID
		}
		task := hc.Item{
			ID:        hc.GenerateID(),
			RepoKey:   "owner/repo",
			EpicID:    epicID,
			ParentID:  epicID,
			Title:     "Task " + string(st),
			Type:      hc.ItemTypeTask,
			Status:    st,
			SessionID: sid,
			Depth:     1,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		store.items[task.ID] = task
	}

	return epic
}

func TestContext_Counts(t *testing.T) {
	store := newFakeHCStore()
	svc := newTestHoneycombService(store)

	epic := seedEpicWithTasks(store, "session-1")

	block, err := svc.Context(context.Background(), epic.ID, "session-1")
	require.NoError(t, err)

	assert.Equal(t, 1, block.Counts.Open)
	assert.Equal(t, 1, block.Counts.InProgress)
	assert.Equal(t, 1, block.Counts.Done)
	assert.Equal(t, 1, block.Counts.Cancelled)
}

func TestContext_MyTasksFiltering(t *testing.T) {
	store := newFakeHCStore()
	svc := newTestHoneycombService(store)

	epic := seedEpicWithTasks(store, "session-1")

	block, err := svc.Context(context.Background(), epic.ID, "session-1")
	require.NoError(t, err)

	assert.Len(t, block.MyTasks, 2)
	for _, twc := range block.MyTasks {
		assert.Equal(t, "session-1", twc.Item.SessionID)
		assert.True(t,
			twc.Item.Status == hc.StatusOpen || twc.Item.Status == hc.StatusInProgress,
			"MyTasks should only contain open/in_progress items",
		)
	}
}

func TestContext_CheckpointFetch(t *testing.T) {
	store := newFakeHCStore()
	svc := newTestHoneycombService(store)

	epicID := hc.GenerateID()
	epic := hc.Item{
		ID:        epicID,
		RepoKey:   "owner/repo",
		Title:     "Epic",
		Type:      hc.ItemTypeEpic,
		Status:    hc.StatusOpen,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	store.items[epicID] = epic

	taskID := hc.GenerateID()
	task := hc.Item{
		ID:        taskID,
		RepoKey:   "owner/repo",
		EpicID:    epicID,
		ParentID:  epicID,
		Title:     "My Task",
		Type:      hc.ItemTypeTask,
		Status:    hc.StatusInProgress,
		SessionID: "session-x",
		Depth:     1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	store.items[taskID] = task

	store.comments[taskID] = []hc.Comment{
		{ID: hc.GenerateID(), ItemID: taskID, Message: "first", CreatedAt: time.Now()},
		{ID: hc.GenerateID(), ItemID: taskID, Message: "latest", CreatedAt: time.Now()},
	}

	block, err := svc.Context(context.Background(), epicID, "session-x")
	require.NoError(t, err)

	require.Len(t, block.MyTasks, 1)
	assert.Equal(t, "latest", block.MyTasks[0].LatestComment.Message)
}

// ---------------------------------------------------------------------------
// Prune tests
// ---------------------------------------------------------------------------

func TestPrune_ForwardsOptions(t *testing.T) {
	store := newFakeHCStore()
	svc := newTestHoneycombService(store)

	opts := hc.PruneOpts{
		OlderThan: 48 * time.Hour,
		Statuses:  []hc.Status{hc.StatusDone, hc.StatusCancelled},
		DryRun:    true,
	}

	_, err := svc.Prune(context.Background(), opts)
	require.NoError(t, err)

	assert.Equal(t, opts.DryRun, store.lastPruneOpts.DryRun)
	assert.Equal(t, opts.OlderThan, store.lastPruneOpts.OlderThan)
	assert.Equal(t, opts.Statuses, store.lastPruneOpts.Statuses)
}

// ---------------------------------------------------------------------------
// AddComment tests
// ---------------------------------------------------------------------------

func TestAddComment_ReturnsComment(t *testing.T) {
	store := newFakeHCStore()
	svc := newTestHoneycombService(store)

	itemID := "hc-testitem"
	store.items[itemID] = hc.Item{ID: itemID, Title: "Test Item", Type: hc.ItemTypeEpic, Status: hc.StatusOpen}

	comment, err := svc.AddComment(context.Background(), itemID, "hello world")
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(comment.ID, "hcc-"), "comment ID should start with hcc-")
	assert.Equal(t, itemID, comment.ItemID)
	assert.Equal(t, "hello world", comment.Message)
	assert.False(t, comment.CreatedAt.IsZero())

	stored := store.comments[itemID]
	require.Len(t, stored, 1)
	assert.Equal(t, comment.ID, stored[0].ID)
}

// ---------------------------------------------------------------------------
// GetItem, ListItems, Next delegation tests
// ---------------------------------------------------------------------------

func TestGetItem_Delegates(t *testing.T) {
	store := newFakeHCStore()
	svc := newTestHoneycombService(store)

	epicID := hc.GenerateID()
	epic := hc.Item{
		ID:        epicID,
		RepoKey:   "owner/repo",
		Title:     "Epic",
		Type:      hc.ItemTypeEpic,
		Status:    hc.StatusOpen,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	store.items[epicID] = epic

	got, err := svc.GetItem(context.Background(), epicID)
	require.NoError(t, err)
	assert.Equal(t, epicID, got.ID)
}

func TestListItems_Delegates(t *testing.T) {
	store := newFakeHCStore()
	svc := newTestHoneycombService(store)

	epic := seedEpicWithTasks(store, "s1")

	items, err := svc.ListItems(context.Background(), hc.ListFilter{EpicID: epic.ID})
	require.NoError(t, err)
	// seedEpicWithTasks creates 4 tasks under the epic (EpicID is set on tasks, not on epic itself).
	assert.Len(t, items, 4)
}

func TestNext_Delegates(t *testing.T) {
	store := newFakeHCStore()
	svc := newTestHoneycombService(store)

	epic := seedEpicWithTasks(store, "s1")

	item, found, err := svc.Next(context.Background(), hc.NextFilter{EpicID: epic.ID})
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, hc.ItemTypeTask, item.Type)
}

// ---------------------------------------------------------------------------
// Context: empty sessionID tests
// ---------------------------------------------------------------------------

func TestHoneycombService_Context_EmptySessionID(t *testing.T) {
	store := newFakeHCStore()
	svc := newTestHoneycombService(store)

	epic := seedEpicWithTasks(store, "session-1")

	// Calling Context with empty sessionID must not populate MyTasks.
	block, err := svc.Context(context.Background(), epic.ID, "")
	require.NoError(t, err)
	assert.Empty(t, block.MyTasks, "MyTasks must be empty when sessionID is empty")
}

// ---------------------------------------------------------------------------
// AddComment: empty message validation
// ---------------------------------------------------------------------------

func TestHoneycombService_AddComment_EmptyMessage(t *testing.T) {
	store := newFakeHCStore()
	svc := newTestHoneycombService(store)

	_, err := svc.AddComment(context.Background(), "hc-item", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "message")

	_, err = svc.AddComment(context.Background(), "hc-item", "   ")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "message")
}

// ---------------------------------------------------------------------------
// UpdateItem cascade tests
// ---------------------------------------------------------------------------

func makeTestEpic(id string) hc.Item {
	return hc.Item{
		ID:        id,
		RepoKey:   "owner/repo",
		Title:     id,
		Type:      hc.ItemTypeEpic,
		Status:    hc.StatusOpen,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func makeTestTask(id, epicID string, status hc.Status) hc.Item {
	return hc.Item{
		ID:        id,
		EpicID:    epicID,
		ParentID:  epicID,
		RepoKey:   "owner/repo",
		Title:     id,
		Type:      hc.ItemTypeTask,
		Status:    status,
		Depth:     1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func TestHoneycombService_UpdateItem_CascadeOnEpicDone(t *testing.T) {
	store := newFakeHCStore()
	svc := newTestHoneycombService(store)

	epic := makeTestEpic("epic-1")
	task1 := makeTestTask("task-1", "epic-1", hc.StatusOpen)
	task2 := makeTestTask("task-2", "epic-1", hc.StatusInProgress)
	store.items["epic-1"] = epic
	store.items["task-1"] = task1
	store.items["task-2"] = task2

	done := hc.StatusDone
	_, err := svc.UpdateItem(context.Background(), "epic-1", hc.ItemUpdate{Status: &done})
	require.NoError(t, err)

	assert.Equal(t, hc.StatusDone, store.items["task-1"].Status, "open task should cascade to done")
	assert.Equal(t, hc.StatusDone, store.items["task-2"].Status, "in_progress task should cascade to done")
}

func TestHoneycombService_UpdateItem_CascadeOnEpicCancelled(t *testing.T) {
	store := newFakeHCStore()
	svc := newTestHoneycombService(store)

	epic := makeTestEpic("epic-1")
	task1 := makeTestTask("task-1", "epic-1", hc.StatusOpen)
	store.items["epic-1"] = epic
	store.items["task-1"] = task1

	cancelled := hc.StatusCancelled
	_, err := svc.UpdateItem(context.Background(), "epic-1", hc.ItemUpdate{Status: &cancelled})
	require.NoError(t, err)

	assert.Equal(t, hc.StatusCancelled, store.items["task-1"].Status, "open task should cascade to cancelled")
}

func TestHoneycombService_UpdateItem_CascadeDoneToCancel(t *testing.T) {
	store := newFakeHCStore()
	svc := newTestHoneycombService(store)

	epic := makeTestEpic("epic-1")
	epic.Status = hc.StatusDone
	task1 := makeTestTask("task-1", "epic-1", hc.StatusOpen)
	store.items["epic-1"] = epic
	store.items["task-1"] = task1

	cancelled := hc.StatusCancelled
	_, err := svc.UpdateItem(context.Background(), "epic-1", hc.ItemUpdate{Status: &cancelled})
	require.NoError(t, err)

	assert.Equal(t, hc.StatusCancelled, store.items["task-1"].Status, "status changed done→cancelled triggers cascade")
}

func TestHoneycombService_UpdateItem_NoCascadeOnNonTerminal(t *testing.T) {
	store := newFakeHCStore()
	svc := newTestHoneycombService(store)

	epic := makeTestEpic("epic-1")
	task1 := makeTestTask("task-1", "epic-1", hc.StatusOpen)
	store.items["epic-1"] = epic
	store.items["task-1"] = task1

	inProgress := hc.StatusInProgress
	_, err := svc.UpdateItem(context.Background(), "epic-1", hc.ItemUpdate{Status: &inProgress})
	require.NoError(t, err)

	assert.Equal(t, hc.StatusOpen, store.items["task-1"].Status, "non-terminal status should not cascade")
}

func TestHoneycombService_UpdateItem_NoCascadeOnTask(t *testing.T) {
	store := newFakeHCStore()
	svc := newTestHoneycombService(store)

	epic := makeTestEpic("epic-1")
	task1 := makeTestTask("task-1", "epic-1", hc.StatusOpen)
	task2 := makeTestTask("task-2", "epic-1", hc.StatusOpen)
	store.items["epic-1"] = epic
	store.items["task-1"] = task1
	store.items["task-2"] = task2

	done := hc.StatusDone
	_, err := svc.UpdateItem(context.Background(), "task-1", hc.ItemUpdate{Status: &done})
	require.NoError(t, err)

	assert.Equal(t, hc.StatusOpen, store.items["task-2"].Status, "sibling task should not be affected")
}

func TestHoneycombService_UpdateItem_NoCascadeWhenStatusUnchanged(t *testing.T) {
	store := newFakeHCStore()
	svc := newTestHoneycombService(store)

	epic := makeTestEpic("epic-1")
	epic.Status = hc.StatusDone
	task1 := makeTestTask("task-1", "epic-1", hc.StatusOpen)
	store.items["epic-1"] = epic
	store.items["task-1"] = task1

	done := hc.StatusDone
	_, err := svc.UpdateItem(context.Background(), "epic-1", hc.ItemUpdate{Status: &done})
	require.NoError(t, err)

	// status didn't change (done→done), task should remain open
	assert.Equal(t, hc.StatusOpen, store.items["task-1"].Status, "same status should not trigger cascade")
}
