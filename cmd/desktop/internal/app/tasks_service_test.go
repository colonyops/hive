package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/hc"
	hcsvc "github.com/colonyops/hive/internal/hive/hc"
)

func newTasksHarness(t *testing.T) (*TasksService, *hcsvc.Service) {
	t.Helper()
	h := newHiveHarness(t, engineOptions{})
	return newTasksService(h.engine), h.engine.HC()
}

func createItem(t *testing.T, tasks *hcsvc.Service, repoKey string, input hc.CreateItemInput) hc.Item {
	t.Helper()
	item, err := tasks.CreateItem(t.Context(), repoKey, input)
	require.NoError(t, err)
	return item
}

func TestTasksService_NilEngineIsUnavailable(t *testing.T) {
	t.Parallel()

	svc := newTasksService(nil)

	_, err := svc.ListTasks(t.Context(), "repo")
	assert.Equal(t, KindUnavailable, KindOf(err))

	_, err = svc.TaskDetail(t.Context(), "task-1")
	assert.Equal(t, KindUnavailable, KindOf(err))

	err = svc.SetTaskStatus(t.Context(), "task-1", string(hc.StatusDone))
	assert.Equal(t, KindUnavailable, KindOf(err))

	err = svc.DeleteTask(t.Context(), "task-1")
	assert.Equal(t, KindUnavailable, KindOf(err))

	_, err = svc.PruneTasks(t.Context(), 30, "repo", false)
	assert.Equal(t, KindUnavailable, KindOf(err))

	_, err = svc.TaskRepoKeys(t.Context())
	assert.Equal(t, KindUnavailable, KindOf(err))
}

func TestTasksService_ListTasksReturnsEveryItemOfTheRepo(t *testing.T) {
	svc, tasks := newTasksHarness(t)
	epic := createItem(t, tasks, "acme/repo", hc.CreateItemInput{Title: "Epic A", Type: hc.ItemTypeEpic})
	task := createItem(t, tasks, "acme/repo", hc.CreateItemInput{Title: "Task A", Desc: "do the thing", Type: hc.ItemTypeTask, ParentID: epic.ID})
	createItem(t, tasks, "other/repo", hc.CreateItemInput{Title: "Elsewhere", Type: hc.ItemTypeEpic})

	items, err := svc.ListTasks(t.Context(), "acme/repo")
	require.NoError(t, err)
	require.Len(t, items, 2)
	byID := map[string]hc.Item{}
	for _, item := range items {
		byID[item.ID] = item
	}
	got := byID[task.ID]
	assert.Equal(t, epic.ID, got.EpicID)
	assert.Equal(t, epic.ID, got.ParentID)
	assert.Equal(t, hc.ItemTypeTask, got.Type)
	assert.Equal(t, hc.StatusOpen, got.Status)
	assert.Equal(t, 1, got.Depth)
}

func TestTasksService_TaskDetailResolvesBlockerTitlesAndComments(t *testing.T) {
	svc, tasks := newTasksHarness(t)
	ctx := t.Context()
	epic := createItem(t, tasks, "acme/repo", hc.CreateItemInput{Title: "Epic", Type: hc.ItemTypeEpic})
	blocker := createItem(t, tasks, "acme/repo", hc.CreateItemInput{Title: "Blocker", Type: hc.ItemTypeTask, ParentID: epic.ID})
	blocked := createItem(t, tasks, "acme/repo", hc.CreateItemInput{Title: "Blocked", Desc: "needs blocker", Type: hc.ItemTypeTask, ParentID: epic.ID})
	require.NoError(t, tasks.AddBlocker(ctx, blocker.ID, blocked.ID))
	_, err := tasks.AddComment(ctx, blocked.ID, "note")
	require.NoError(t, err)

	detail, err := svc.TaskDetail(ctx, blocked.ID)
	require.NoError(t, err)
	assert.Equal(t, "needs blocker", detail.Desc)
	assert.Equal(t, []TaskBlocker{{ID: blocker.ID, Title: "Blocker", Status: "open"}}, detail.Blockers)
	require.Len(t, detail.Comments, 1)
	assert.Equal(t, "note", detail.Comments[0].Message)
}

// The frontend clears its selection when a task vanishes, which rides on every
// read and write reporting a missing id as not found.
func TestTasksService_MissingIDsAreNotFound(t *testing.T) {
	svc, _ := newTasksHarness(t)

	_, err := svc.TaskDetail(t.Context(), "missing")
	assert.Equal(t, KindNotFound, KindOf(err))
	require.ErrorIs(t, err, hc.ErrNotFound)

	assert.Equal(t, KindNotFound, KindOf(svc.SetTaskStatus(t.Context(), "missing", string(hc.StatusDone))))
	assert.Equal(t, KindNotFound, KindOf(svc.DeleteTask(t.Context(), "missing")))
}

func TestTasksService_SetTaskStatusRejectsAnUnknownStatus(t *testing.T) {
	svc, tasks := newTasksHarness(t)
	item := createItem(t, tasks, "acme/repo", hc.CreateItemInput{Title: "Epic", Type: hc.ItemTypeEpic})

	assert.Equal(t, KindInvalid, KindOf(svc.SetTaskStatus(t.Context(), item.ID, "archived")))

	got, err := tasks.GetItem(t.Context(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, hc.StatusOpen, got.Status)
}

func TestTasksService_SetTaskStatusCascadesToChildren(t *testing.T) {
	svc, tasks := newTasksHarness(t)
	epic := createItem(t, tasks, "acme/repo", hc.CreateItemInput{Title: "Epic", Type: hc.ItemTypeEpic})
	createItem(t, tasks, "acme/repo", hc.CreateItemInput{Title: "Child 1", Type: hc.ItemTypeTask, ParentID: epic.ID})
	createItem(t, tasks, "acme/repo", hc.CreateItemInput{Title: "Child 2", Type: hc.ItemTypeTask, ParentID: epic.ID})

	require.NoError(t, svc.SetTaskStatus(t.Context(), epic.ID, "In_Progress"), "status parses in any case")
	require.NoError(t, svc.SetTaskStatus(t.Context(), epic.ID, string(hc.StatusDone)))

	items, err := svc.ListTasks(t.Context(), "acme/repo")
	require.NoError(t, err)
	require.Len(t, items, 3)
	for _, item := range items {
		assert.Equal(t, hc.StatusDone, item.Status, item.Title)
	}
}

func TestTasksService_DeleteTaskRemovesTheSubtree(t *testing.T) {
	svc, tasks := newTasksHarness(t)
	epic := createItem(t, tasks, "acme/repo", hc.CreateItemInput{Title: "Epic", Type: hc.ItemTypeEpic})
	child := createItem(t, tasks, "acme/repo", hc.CreateItemInput{Title: "Child", Type: hc.ItemTypeTask, ParentID: epic.ID})

	require.NoError(t, svc.DeleteTask(t.Context(), epic.ID))

	_, err := tasks.GetItem(t.Context(), epic.ID)
	require.ErrorIs(t, err, hc.ErrNotFound)
	_, err = tasks.GetItem(t.Context(), child.ID)
	require.ErrorIs(t, err, hc.ErrNotFound)
}

func TestTasksService_PruneTasksRejectsDaysOutOfRange(t *testing.T) {
	svc, _ := newTasksHarness(t)

	_, err := svc.PruneTasks(t.Context(), -1, "repo", false)
	assert.Equal(t, KindInvalid, KindOf(err))
	_, err = svc.PruneTasks(t.Context(), maxPruneOlderThanDays+1, "repo", false)
	assert.Equal(t, KindInvalid, KindOf(err))
}

func TestTasksService_PruneTasksDryRunCountMatchesRealCount(t *testing.T) {
	svc, tasks := newTasksHarness(t)
	epic := createItem(t, tasks, "acme/repo", hc.CreateItemInput{Title: "Epic", Type: hc.ItemTypeEpic})
	child := createItem(t, tasks, "acme/repo", hc.CreateItemInput{Title: "Child", Type: hc.ItemTypeTask, ParentID: epic.ID})
	require.NoError(t, svc.SetTaskStatus(t.Context(), epic.ID, string(hc.StatusDone)))

	fresh, err := svc.PruneTasks(t.Context(), 7, "acme/repo", true)
	require.NoError(t, err)
	assert.Zero(t, fresh, "the days convert to a cutoff, and nothing is a week old yet")

	// Zero days puts the cutoff at now, so both done items count as older than
	// it however fast the test runs.
	time.Sleep(time.Millisecond)
	dryCount, err := svc.PruneTasks(t.Context(), 0, "acme/repo", true)
	require.NoError(t, err)
	assert.Equal(t, 2, dryCount)

	realCount, err := svc.PruneTasks(t.Context(), 0, "acme/repo", false)
	require.NoError(t, err)
	assert.Equal(t, dryCount, realCount)
	_, err = tasks.GetItem(t.Context(), child.ID)
	require.ErrorIs(t, err, hc.ErrNotFound)
}

func TestTasksService_TaskRepoKeysExcludesEmpty(t *testing.T) {
	svc, tasks := newTasksHarness(t)
	createItem(t, tasks, "acme/repo", hc.CreateItemInput{Title: "Epic", Type: hc.ItemTypeEpic})
	createItem(t, tasks, "", hc.CreateItemInput{Title: "No repo epic", Type: hc.ItemTypeEpic})

	keys, err := svc.TaskRepoKeys(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"acme/repo"}, keys)
}

func TestTasksService_StoreFailuresAreInternal(t *testing.T) {
	h := newHiveHarness(t, engineOptions{})
	svc := newTasksService(h.engine)
	require.NoError(t, h.engine.DB().Close())

	_, err := svc.ListTasks(t.Context(), "repo")
	assert.Equal(t, KindInternal, KindOf(err))
	_, err = svc.TaskRepoKeys(t.Context())
	assert.Equal(t, KindInternal, KindOf(err))
	_, err = svc.PruneTasks(t.Context(), 1, "repo", false)
	assert.Equal(t, KindInternal, KindOf(err))
}
