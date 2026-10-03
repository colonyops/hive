package dispatch

import (
	"context"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/hc"
	coredb "github.com/colonyops/hive/internal/data/db"
	"github.com/colonyops/hive/internal/data/stores"
	hivesvc "github.com/colonyops/hive/internal/hive"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newHiveHoneycombTasks builds the real shared hc service over a temporary
// core database, so HoneycombManagement is satisfied structurally against
// hive's actual implementation rather than a fake shaped to fit it. svc is
// returned alongside the adapter so tests can seed and verify state through
// hive's own API.
func newHiveHoneycombTasks(t *testing.T) (*HiveHoneycomb, *hivesvc.HoneycombService) {
	t.Helper()
	database, err := coredb.Open(t.TempDir(), coredb.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })

	svc := hivesvc.NewHoneycombService(stores.NewHCStore(database), zerolog.Nop())
	return NewHiveHoneycomb(svc), svc
}

func TestHiveHoneycombListTasksProjectsEveryField(t *testing.T) {
	adapter, svc := newHiveHoneycombTasks(t)
	ctx := t.Context()

	epic, err := svc.CreateItem(ctx, "acme/repo", hc.CreateItemInput{Title: "Epic A", Type: hc.ItemTypeEpic})
	require.NoError(t, err)
	task, err := svc.CreateItem(ctx, "acme/repo", hc.CreateItemInput{
		Title: "Task A", Desc: "do the thing", Type: hc.ItemTypeTask, ParentID: epic.ID,
	})
	require.NoError(t, err)

	items, err := adapter.ListTasks(ctx, "acme/repo")
	require.NoError(t, err)
	require.Len(t, items, 2)

	byID := make(map[string]TaskItem, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}

	got := byID[task.ID]
	// CreatedAt/UpdatedAt round-trip through SQLite's UnixNano storage, which
	// drops the monotonic reading time.Now() attached in-process — compare
	// with time.Time.Equal rather than assert.Equal's struct-literal match.
	assert.True(t, got.CreatedAt.Equal(task.CreatedAt), "CreatedAt: got %v, want %v", got.CreatedAt, task.CreatedAt)
	assert.True(t, got.UpdatedAt.Equal(task.UpdatedAt), "UpdatedAt: got %v, want %v", got.UpdatedAt, task.UpdatedAt)
	got.CreatedAt, got.UpdatedAt = time.Time{}, time.Time{}
	assert.Equal(t, TaskItem{
		ID:        task.ID,
		RepoKey:   "acme/repo",
		EpicID:    epic.ID,
		ParentID:  epic.ID,
		SessionID: "",
		Title:     "Task A",
		Type:      string(hc.ItemTypeTask),
		Status:    TaskStatusOpen,
		Blocked:   false,
		Depth:     1,
	}, got)
}

func TestHiveHoneycombTaskDetailResolvesBlockerTitlesAndComments(t *testing.T) {
	adapter, svc := newHiveHoneycombTasks(t)
	ctx := t.Context()

	epic, err := svc.CreateItem(ctx, "acme/repo", hc.CreateItemInput{Title: "Epic", Type: hc.ItemTypeEpic})
	require.NoError(t, err)
	blocker, err := svc.CreateItem(ctx, "acme/repo", hc.CreateItemInput{Title: "Blocker", Type: hc.ItemTypeTask, ParentID: epic.ID})
	require.NoError(t, err)
	blocked, err := svc.CreateItem(ctx, "acme/repo", hc.CreateItemInput{
		Title: "Blocked", Desc: "needs blocker", Type: hc.ItemTypeTask, ParentID: epic.ID,
	})
	require.NoError(t, err)
	require.NoError(t, svc.AddBlocker(ctx, blocker.ID, blocked.ID))
	_, err = svc.AddComment(ctx, blocked.ID, "note")
	require.NoError(t, err)

	detail, err := adapter.TaskDetail(ctx, blocked.ID)
	require.NoError(t, err)
	assert.Equal(t, "needs blocker", detail.Desc)
	require.Len(t, detail.Blockers, 1)
	assert.Equal(t, TaskBlocker{ID: blocker.ID, Title: "Blocker", Status: TaskStatusOpen}, detail.Blockers[0])
	require.Len(t, detail.Comments, 1)
	assert.Equal(t, "note", detail.Comments[0].Message)
}

// staleGetItem wraps a real HoneycombManagement and substitutes a fixed
// response for one id, so a test can force TaskDetail's per-blocker GetItem
// into the not-found branch deterministically. hc_task_blockers has ON
// DELETE CASCADE, so deleting a blocker item removes the edge along with it
// — item.BlockerIDs never actually names a deleted item once re-read. The
// race TaskDetail defends against (BlockerIDs read, then that blocker
// deleted before its own GetItem runs) is real but not reproducible through
// the store alone, so this replays a stale BlockerIDs snapshot instead.
type staleGetItem struct {
	HoneycombManagement
	id   string
	item hc.Item
}

func (s staleGetItem) GetItem(ctx context.Context, id string) (hc.Item, error) {
	if id == s.id {
		return s.item, nil
	}
	return s.HoneycombManagement.GetItem(ctx, id)
}

func TestHiveHoneycombTaskDetailTitlesAVanishedBlockerEmpty(t *testing.T) {
	_, svc := newHiveHoneycombTasks(t)
	ctx := t.Context()

	epic, err := svc.CreateItem(ctx, "acme/repo", hc.CreateItemInput{Title: "Epic", Type: hc.ItemTypeEpic})
	require.NoError(t, err)
	blocker, err := svc.CreateItem(ctx, "acme/repo", hc.CreateItemInput{Title: "Blocker", Type: hc.ItemTypeTask, ParentID: epic.ID})
	require.NoError(t, err)
	blocked, err := svc.CreateItem(ctx, "acme/repo", hc.CreateItemInput{Title: "Blocked", Type: hc.ItemTypeTask, ParentID: epic.ID})
	require.NoError(t, err)
	require.NoError(t, svc.AddBlocker(ctx, blocker.ID, blocked.ID))

	staleBlocked, err := svc.GetItem(ctx, blocked.ID)
	require.NoError(t, err)
	require.Equal(t, []string{blocker.ID}, staleBlocked.BlockerIDs)

	require.NoError(t, svc.DeleteItem(ctx, blocker.ID))

	adapter := NewHiveHoneycomb(staleGetItem{HoneycombManagement: svc, id: blocked.ID, item: staleBlocked})
	detail, err := adapter.TaskDetail(ctx, blocked.ID)
	require.NoError(t, err)
	require.Len(t, detail.Blockers, 1)
	assert.Equal(t, TaskBlocker{ID: blocker.ID, Title: "", Status: ""}, detail.Blockers[0])
}

// The frontend's clear-selection-on-vanish behavior rides on TaskDetail (and
// SetTaskStatus) specifically translating hc.ErrNotFound, not only DeleteTask.
func TestHiveHoneycombTaskDetailAndSetStatusReportMissingIDs(t *testing.T) {
	adapter, _ := newHiveHoneycombTasks(t)
	ctx := t.Context()

	_, err := adapter.TaskDetail(ctx, "does-not-exist")
	require.ErrorIs(t, err, ErrTaskNotFound)

	err = adapter.SetTaskStatus(ctx, "does-not-exist", TaskStatusDone)
	require.ErrorIs(t, err, ErrTaskNotFound)
}

func TestHiveHoneycombSetTaskStatusRejectsUnknownStatus(t *testing.T) {
	adapter, svc := newHiveHoneycombTasks(t)
	ctx := t.Context()
	item, err := svc.CreateItem(ctx, "acme/repo", hc.CreateItemInput{Title: "Epic", Type: hc.ItemTypeEpic})
	require.NoError(t, err)

	err = adapter.SetTaskStatus(ctx, item.ID, "archived")
	require.ErrorIs(t, err, ErrInvalidTaskStatus)

	got, err := svc.GetItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, hc.StatusOpen, got.Status)
}

func TestHiveHoneycombSetTaskStatusCascadesToChildrenVisibleOnRelist(t *testing.T) {
	adapter, svc := newHiveHoneycombTasks(t)
	ctx := t.Context()

	epic, err := svc.CreateItem(ctx, "acme/repo", hc.CreateItemInput{Title: "Epic", Type: hc.ItemTypeEpic})
	require.NoError(t, err)
	child1, err := svc.CreateItem(ctx, "acme/repo", hc.CreateItemInput{Title: "Child 1", Type: hc.ItemTypeTask, ParentID: epic.ID})
	require.NoError(t, err)
	child2, err := svc.CreateItem(ctx, "acme/repo", hc.CreateItemInput{Title: "Child 2", Type: hc.ItemTypeTask, ParentID: epic.ID})
	require.NoError(t, err)

	require.NoError(t, adapter.SetTaskStatus(ctx, epic.ID, TaskStatusDone))

	items, err := adapter.ListTasks(ctx, "acme/repo")
	require.NoError(t, err)
	byID := make(map[string]TaskItem, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}
	assert.Equal(t, TaskStatusDone, byID[epic.ID].Status)
	assert.Equal(t, TaskStatusDone, byID[child1.ID].Status)
	assert.Equal(t, TaskStatusDone, byID[child2.ID].Status)
}

func TestHiveHoneycombDeleteTaskRemovesSubtreeAndErrorsOnMissingID(t *testing.T) {
	adapter, svc := newHiveHoneycombTasks(t)
	ctx := t.Context()

	epic, err := svc.CreateItem(ctx, "acme/repo", hc.CreateItemInput{Title: "Epic", Type: hc.ItemTypeEpic})
	require.NoError(t, err)
	child, err := svc.CreateItem(ctx, "acme/repo", hc.CreateItemInput{Title: "Child", Type: hc.ItemTypeTask, ParentID: epic.ID})
	require.NoError(t, err)

	require.NoError(t, adapter.DeleteTask(ctx, epic.ID))

	_, err = svc.GetItem(ctx, epic.ID)
	require.ErrorIs(t, err, hc.ErrNotFound)
	_, err = svc.GetItem(ctx, child.ID)
	require.ErrorIs(t, err, hc.ErrNotFound)

	err = adapter.DeleteTask(ctx, "does-not-exist")
	require.ErrorIs(t, err, ErrTaskNotFound)
}

func TestHiveHoneycombPruneTasksDryRunCountMatchesRealCount(t *testing.T) {
	adapter, svc := newHiveHoneycombTasks(t)
	ctx := t.Context()

	epic, err := svc.CreateItem(ctx, "acme/repo", hc.CreateItemInput{Title: "Epic", Type: hc.ItemTypeEpic})
	require.NoError(t, err)
	child, err := svc.CreateItem(ctx, "acme/repo", hc.CreateItemInput{Title: "Child", Type: hc.ItemTypeTask, ParentID: epic.ID})
	require.NoError(t, err)
	// Cascades child to done too, so both fall within Prune's default
	// done+cancelled status set.
	require.NoError(t, adapter.SetTaskStatus(ctx, epic.ID, TaskStatusDone))

	// A cutoff pushed an hour into the future guarantees both items count as
	// "older than" it regardless of how fast the test runs.
	opts := TaskPruneOptions{OlderThan: -time.Hour}

	dryCount, err := adapter.PruneTasks(ctx, TaskPruneOptions{OlderThan: opts.OlderThan, DryRun: true})
	require.NoError(t, err)
	assert.Equal(t, 2, dryCount)

	realCount, err := adapter.PruneTasks(ctx, TaskPruneOptions{OlderThan: opts.OlderThan, DryRun: false})
	require.NoError(t, err)
	assert.Equal(t, dryCount, realCount)

	_, err = svc.GetItem(ctx, epic.ID)
	require.ErrorIs(t, err, hc.ErrNotFound)
	_, err = svc.GetItem(ctx, child.ID)
	require.ErrorIs(t, err, hc.ErrNotFound)
}

func TestHiveHoneycombTaskRepoKeysExcludesEmpty(t *testing.T) {
	adapter, svc := newHiveHoneycombTasks(t)
	ctx := t.Context()

	_, err := svc.CreateItem(ctx, "acme/repo", hc.CreateItemInput{Title: "Epic", Type: hc.ItemTypeEpic})
	require.NoError(t, err)
	_, err = svc.CreateItem(ctx, "", hc.CreateItemInput{Title: "No repo epic", Type: hc.ItemTypeEpic})
	require.NoError(t, err)

	keys, err := adapter.TaskRepoKeys(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"acme/repo"}, keys)
}
