package app

import (
	"context"
	"errors"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
)

// taskSource is the seam's HiveHoneycomb surface TasksService consumes. It is
// declared here, over dispatch's projection types, rather than importing
// dispatch.HiveHoneycomb directly, so a fake can stand in without pulling in
// the shared hive packages (depguard forbids that import in this package).
type taskSource interface {
	ListTasks(ctx context.Context, repoKey string) ([]dispatch.TaskItem, error)
	TaskDetail(ctx context.Context, id string) (dispatch.TaskDetail, error)
	SetTaskStatus(ctx context.Context, id, status string) error
	DeleteTask(ctx context.Context, id string) error
	PruneTasks(ctx context.Context, opts dispatch.TaskPruneOptions) (int, error)
	TaskRepoKeys(ctx context.Context) ([]string, error)
}

// A nil source makes every method return KindUnavailable.
type TasksService struct {
	source taskSource
}

func newTasksService(source taskSource) *TasksService {
	return &TasksService{source: source}
}

func (s *TasksService) ListTasks(ctx context.Context, repoKey string) ([]dispatch.TaskItem, error) {
	if s.source == nil {
		return nil, Errorf(KindUnavailable, "tasks are unavailable")
	}
	items, err := s.source.ListTasks(ctx, repoKey)
	if err != nil {
		return nil, s.classifyError(err, "listing tasks")
	}
	return items, nil
}

func (s *TasksService) TaskDetail(ctx context.Context, id string) (dispatch.TaskDetail, error) {
	if s.source == nil {
		return dispatch.TaskDetail{}, Errorf(KindUnavailable, "tasks are unavailable")
	}
	detail, err := s.source.TaskDetail(ctx, id)
	if err != nil {
		return dispatch.TaskDetail{}, s.classifyError(err, "reading task %q", id)
	}
	return detail, nil
}

func (s *TasksService) SetTaskStatus(ctx context.Context, id, status string) error {
	if s.source == nil {
		return Errorf(KindUnavailable, "tasks are unavailable")
	}
	if !validTaskStatus(status) {
		return Errorf(KindInvalid, "unknown task status %q", status)
	}
	if err := s.source.SetTaskStatus(ctx, id, status); err != nil {
		return s.classifyError(err, "setting status for task %q", id)
	}
	return nil
}

func (s *TasksService) DeleteTask(ctx context.Context, id string) error {
	if s.source == nil {
		return Errorf(KindUnavailable, "tasks are unavailable")
	}
	if err := s.source.DeleteTask(ctx, id); err != nil {
		return s.classifyError(err, "deleting task %q", id)
	}
	return nil
}

const maxPruneOlderThanDays = 36500 // 100 years

// PruneTasks takes olderThanDays rather than a time.Duration: it is the unit
// the tasks view's prune dialog collects, and converting here keeps the
// dispatch DTO's duration out of the frontend binding.
func (s *TasksService) PruneTasks(ctx context.Context, olderThanDays int, repoKey string, dryRun bool) (int, error) {
	if s.source == nil {
		return 0, Errorf(KindUnavailable, "tasks are unavailable")
	}
	// The upper bound keeps the day→duration conversion below from overflowing
	// int64 nanoseconds (~106751 days), which would turn the cutoff negative
	// and prune every terminal item regardless of age.
	if olderThanDays < 0 || olderThanDays > maxPruneOlderThanDays {
		return 0, Errorf(KindInvalid, "olderThanDays must be between 0 and %d", maxPruneOlderThanDays)
	}
	count, err := s.source.PruneTasks(ctx, dispatch.TaskPruneOptions{
		OlderThan: time.Duration(olderThanDays) * 24 * time.Hour,
		RepoKey:   repoKey,
		DryRun:    dryRun,
	})
	if err != nil {
		return count, s.classifyError(err, "pruning tasks")
	}
	return count, nil
}

func (s *TasksService) TaskRepoKeys(ctx context.Context) ([]string, error) {
	if s.source == nil {
		return nil, Errorf(KindUnavailable, "tasks are unavailable")
	}
	keys, err := s.source.TaskRepoKeys(ctx)
	if err != nil {
		return nil, s.classifyError(err, "listing task repo keys")
	}
	return keys, nil
}

// validTaskStatus checks status against the seam's closed vocabulary, so a
// caller-supplied string only ever reaches hc as one of the four it accepts.
func validTaskStatus(status string) bool {
	switch status {
	case dispatch.TaskStatusOpen, dispatch.TaskStatusInProgress, dispatch.TaskStatusDone, dispatch.TaskStatusCancelled:
		return true
	default:
		return false
	}
}

// classifyError maps a source failure to its Kind. dispatch.ErrTaskNotFound
// is the only case the seam distinguishes; anything else is internal.
func (s *TasksService) classifyError(err error, format string, args ...any) error {
	if errors.Is(err, dispatch.ErrTaskNotFound) {
		return Wrap(err, KindNotFound, format, args...)
	}
	return Wrap(err, KindInternal, format, args...)
}
