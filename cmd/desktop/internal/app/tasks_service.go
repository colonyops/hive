package app

import (
	"context"
	"errors"
	"time"

	"github.com/colonyops/hive/internal/domain/hc"
	"github.com/colonyops/hive/internal/hive"
	hcsvc "github.com/colonyops/hive/internal/hive/hc"
)

// TaskBlocker is one explicit blocker on a task. A blocker whose item has since
// been deleted keeps its ID with an empty Title rather than being dropped.
type TaskBlocker struct {
	ID     string
	Title  string
	Status hc.Status
}

// TaskDetail is one hc item read in full, for a detail view.
type TaskDetail struct {
	hc.Item
	Blockers []TaskBlocker
	Comments []hc.Comment
}

// TasksService is the desktop's tasks view over hive's hc issue tracker. A nil
// engine makes every method return KindUnavailable.
type TasksService struct {
	hive *hive.Engine
}

func newTasksService(engine *hive.Engine) *TasksService {
	return &TasksService{hive: engine}
}

func (s *TasksService) tasks() (*hcsvc.Service, error) {
	if s.hive == nil {
		return nil, Errorf(KindUnavailable, "tasks are unavailable")
	}
	return s.hive.HC(), nil
}

// ListTasks returns every item for repoKey, epics and tasks alike, with no
// status filter: the tasks view's filter groups do not map to hc's single
// status field, so it filters the full list itself.
func (s *TasksService) ListTasks(ctx context.Context, repoKey string) ([]hc.Item, error) {
	tasks, err := s.tasks()
	if err != nil {
		return nil, err
	}
	items, err := tasks.ListItems(ctx, hc.ListFilter{RepoKey: repoKey})
	if err != nil {
		return nil, classifyTaskError(err, "listing tasks")
	}
	return items, nil
}

// TaskDetail reads one item in full: its own fields, its comments, and a title
// for each explicit blocker. ListItems never fills BlockerIDs, so each
// blocker's title costs its own GetItem.
func (s *TasksService) TaskDetail(ctx context.Context, id string) (TaskDetail, error) {
	tasks, err := s.tasks()
	if err != nil {
		return TaskDetail{}, err
	}
	item, err := tasks.GetItem(ctx, id)
	if err != nil {
		return TaskDetail{}, classifyTaskError(err, "reading task %q", id)
	}
	comments, err := tasks.ListComments(ctx, id)
	if err != nil {
		return TaskDetail{}, classifyTaskError(err, "reading task %q", id)
	}

	blockers := make([]TaskBlocker, 0, len(item.BlockerIDs))
	for _, blockerID := range item.BlockerIDs {
		blocker, err := tasks.GetItem(ctx, blockerID)
		if err != nil {
			if errors.Is(err, hc.ErrNotFound) {
				// The blocker item is gone, but the edge is not this read's to
				// fix: surface the ID with no title.
				blockers = append(blockers, TaskBlocker{ID: blockerID})
				continue
			}
			return TaskDetail{}, Wrap(err, KindInternal, "reading blocker %q of task %q", blockerID, id)
		}
		blockers = append(blockers, TaskBlocker{ID: blocker.ID, Title: blocker.Title, Status: blocker.Status})
	}
	return TaskDetail{Item: item, Blockers: blockers, Comments: comments}, nil
}

// SetTaskStatus sets id's status. A terminal status on an epic cascades to
// every non-terminal descendant when it changes the epic's status
// (hcsvc.Service.UpdateItem).
func (s *TasksService) SetTaskStatus(ctx context.Context, id, status string) error {
	tasks, err := s.tasks()
	if err != nil {
		return err
	}
	parsed, err := hc.ParseStatus(status)
	if err != nil {
		return Wrap(err, KindInvalid, "setting status for task %q", id)
	}
	if _, err := tasks.UpdateItem(ctx, id, hc.ItemUpdate{Status: &parsed}); err != nil {
		return classifyTaskError(err, "setting status for task %q", id)
	}
	return nil
}

// DeleteTask deletes id and its whole subtree, comments included. GetItem
// runs first so a missing id reports KindNotFound instead of DeleteItem's
// silent no-op on an unknown ID.
func (s *TasksService) DeleteTask(ctx context.Context, id string) error {
	tasks, err := s.tasks()
	if err != nil {
		return err
	}
	if _, err := tasks.GetItem(ctx, id); err != nil {
		return classifyTaskError(err, "deleting task %q", id)
	}
	if err := tasks.DeleteItem(ctx, id); err != nil {
		return classifyTaskError(err, "deleting task %q", id)
	}
	return nil
}

const maxPruneOlderThanDays = 36500 // 100 years

// PruneTasks removes terminal, stale items. It takes olderThanDays because that
// is the unit the prune dialog collects. Leaving the statuses unset keeps the
// store's done-and-cancelled default.
func (s *TasksService) PruneTasks(ctx context.Context, olderThanDays int, repoKey string, dryRun bool) (int, error) {
	tasks, err := s.tasks()
	if err != nil {
		return 0, err
	}
	// The upper bound keeps the day-to-duration conversion below from
	// overflowing int64 nanoseconds (~106751 days), which would turn the cutoff
	// negative and prune every terminal item regardless of age.
	if olderThanDays < 0 || olderThanDays > maxPruneOlderThanDays {
		return 0, Errorf(KindInvalid, "olderThanDays must be between 0 and %d", maxPruneOlderThanDays)
	}
	count, err := tasks.Prune(ctx, hc.PruneOpts{
		OlderThan: time.Duration(olderThanDays) * 24 * time.Hour,
		RepoKey:   repoKey,
		DryRun:    dryRun,
	})
	if err != nil {
		return count, classifyTaskError(err, "pruning tasks")
	}
	return count, nil
}

// TaskRepoKeys returns every distinct repo key that has at least one hc item.
func (s *TasksService) TaskRepoKeys(ctx context.Context) ([]string, error) {
	tasks, err := s.tasks()
	if err != nil {
		return nil, err
	}
	keys, err := tasks.ListRepoKeys(ctx)
	if err != nil {
		return nil, classifyTaskError(err, "listing task repo keys")
	}
	return keys, nil
}

func classifyTaskError(err error, format string, args ...any) error {
	if errors.Is(err, hc.ErrNotFound) {
		return Wrap(err, KindNotFound, format, args...)
	}
	return Wrap(err, KindInternal, format, args...)
}
