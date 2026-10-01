package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
)

// fakeTaskSource stands in for dispatch.HiveHoneycomb without importing
// the shared hive packages. err* fields fail their matching method; everything else records
// what it was called with, for assertions that don't care about kind mapping.
type fakeTaskSource struct {
	items    []dispatch.TaskItem
	detail   dispatch.TaskDetail
	repoKeys []string
	pruned   int

	listErr   error
	detailErr error
	statusErr error
	deleteErr error
	pruneErr  error
	repoErr   error

	statusCalls []string
	deleteCalls []string
	pruneOpts   []dispatch.TaskPruneOptions
}

func (f *fakeTaskSource) ListTasks(context.Context, string) ([]dispatch.TaskItem, error) {
	return f.items, f.listErr
}

func (f *fakeTaskSource) TaskDetail(context.Context, string) (dispatch.TaskDetail, error) {
	return f.detail, f.detailErr
}

func (f *fakeTaskSource) SetTaskStatus(_ context.Context, id, status string) error {
	f.statusCalls = append(f.statusCalls, id+":"+status)
	return f.statusErr
}

func (f *fakeTaskSource) DeleteTask(_ context.Context, id string) error {
	f.deleteCalls = append(f.deleteCalls, id)
	return f.deleteErr
}

func (f *fakeTaskSource) PruneTasks(_ context.Context, opts dispatch.TaskPruneOptions) (int, error) {
	f.pruneOpts = append(f.pruneOpts, opts)
	return f.pruned, f.pruneErr
}

func (f *fakeTaskSource) TaskRepoKeys(context.Context) ([]string, error) {
	return f.repoKeys, f.repoErr
}

func TestTasksService_NilSourceIsUnavailable(t *testing.T) {
	t.Parallel()

	svc := newTasksService(nil)

	_, err := svc.ListTasks(t.Context(), "repo")
	assert.Equal(t, KindUnavailable, KindOf(err))

	_, err = svc.TaskDetail(t.Context(), "task-1")
	assert.Equal(t, KindUnavailable, KindOf(err))

	err = svc.SetTaskStatus(t.Context(), "task-1", dispatch.TaskStatusDone)
	assert.Equal(t, KindUnavailable, KindOf(err))

	err = svc.DeleteTask(t.Context(), "task-1")
	assert.Equal(t, KindUnavailable, KindOf(err))

	_, err = svc.PruneTasks(t.Context(), 30, "repo", false)
	assert.Equal(t, KindUnavailable, KindOf(err))

	_, err = svc.TaskRepoKeys(t.Context())
	assert.Equal(t, KindUnavailable, KindOf(err))
}

func TestTasksService_SetTaskStatus_UnknownStatusIsInvalid(t *testing.T) {
	t.Parallel()

	fake := &fakeTaskSource{}
	svc := newTasksService(fake)

	err := svc.SetTaskStatus(t.Context(), "task-1", "archived")
	assert.Equal(t, KindInvalid, KindOf(err))
	assert.Empty(t, fake.statusCalls, "an invalid status must never reach the source")
}

func TestTasksService_SetTaskStatus_KnownStatusReachesSource(t *testing.T) {
	t.Parallel()

	fake := &fakeTaskSource{}
	svc := newTasksService(fake)

	require.NoError(t, svc.SetTaskStatus(t.Context(), "task-1", dispatch.TaskStatusInProgress))
	assert.Equal(t, []string{"task-1:" + dispatch.TaskStatusInProgress}, fake.statusCalls)
}

func TestTasksService_PruneTasks_NegativeDaysIsInvalid(t *testing.T) {
	t.Parallel()

	fake := &fakeTaskSource{}
	svc := newTasksService(fake)

	_, err := svc.PruneTasks(t.Context(), -1, "repo", false)
	assert.Equal(t, KindInvalid, KindOf(err))
	assert.Empty(t, fake.pruneOpts, "a rejected request must never reach the source")
}

func TestTasksService_PruneTasks_OverlargeDaysIsInvalid(t *testing.T) {
	t.Parallel()

	fake := &fakeTaskSource{}
	svc := newTasksService(fake)

	_, err := svc.PruneTasks(t.Context(), maxPruneOlderThanDays+1, "repo", false)
	assert.Equal(t, KindInvalid, KindOf(err))
	assert.Empty(t, fake.pruneOpts, "a rejected request must never reach the source")
}

func TestTasksService_PruneTasks_ConvertsDaysToDuration(t *testing.T) {
	t.Parallel()

	fake := &fakeTaskSource{pruned: 3}
	svc := newTasksService(fake)

	count, err := svc.PruneTasks(t.Context(), 7, "repo-a", true)
	require.NoError(t, err)
	assert.Equal(t, 3, count)
	require.Len(t, fake.pruneOpts, 1)
	assert.Equal(t, dispatch.TaskPruneOptions{
		OlderThan: 7 * 24 * time.Hour,
		RepoKey:   "repo-a",
		DryRun:    true,
	}, fake.pruneOpts[0])
}

func TestTasksService_TaskDetail_NotFound(t *testing.T) {
	t.Parallel()

	fake := &fakeTaskSource{detailErr: dispatch.ErrTaskNotFound}
	svc := newTasksService(fake)

	_, err := svc.TaskDetail(t.Context(), "missing")
	assert.Equal(t, KindNotFound, KindOf(err))
	assert.ErrorIs(t, err, dispatch.ErrTaskNotFound)
}

func TestTasksService_DeleteTask_NotFound(t *testing.T) {
	t.Parallel()

	fake := &fakeTaskSource{deleteErr: dispatch.ErrTaskNotFound}
	svc := newTasksService(fake)

	err := svc.DeleteTask(t.Context(), "missing")
	assert.Equal(t, KindNotFound, KindOf(err))
}

func TestTasksService_SetTaskStatus_NotFound(t *testing.T) {
	t.Parallel()

	fake := &fakeTaskSource{statusErr: dispatch.ErrTaskNotFound}
	svc := newTasksService(fake)

	err := svc.SetTaskStatus(t.Context(), "missing", dispatch.TaskStatusDone)
	assert.Equal(t, KindNotFound, KindOf(err))
}

func TestTasksService_UnclassifiedSourceErrorsAreInternal(t *testing.T) {
	t.Parallel()

	cause := errors.New("hc store unreachable")
	fake := &fakeTaskSource{listErr: cause, repoErr: cause, pruneErr: cause}
	svc := newTasksService(fake)

	_, err := svc.ListTasks(t.Context(), "repo")
	assert.Equal(t, KindInternal, KindOf(err))
	require.ErrorIs(t, err, cause)

	_, err = svc.TaskRepoKeys(t.Context())
	assert.Equal(t, KindInternal, KindOf(err))

	_, err = svc.PruneTasks(t.Context(), 1, "repo", false)
	assert.Equal(t, KindInternal, KindOf(err))
}

func TestTasksService_ListTasksAndRepoKeysPassThrough(t *testing.T) {
	t.Parallel()

	fake := &fakeTaskSource{
		items:    []dispatch.TaskItem{{ID: "task-1", Title: "Fix the thing"}},
		repoKeys: []string{"hay-kot/hive-desktop"},
	}
	svc := newTasksService(fake)

	items, err := svc.ListTasks(t.Context(), "hay-kot/hive-desktop")
	require.NoError(t, err)
	assert.Equal(t, fake.items, items)

	keys, err := svc.TaskRepoKeys(t.Context())
	require.NoError(t, err)
	assert.Equal(t, fake.repoKeys, keys)
}
