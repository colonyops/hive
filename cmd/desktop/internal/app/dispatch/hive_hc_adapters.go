package dispatch

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/colonyops/hive/internal/core/hc"
)

// TaskStatus mirrors hc's status vocabulary so the desktop's tasks view can
// filter and set status without importing the shared hc package. Values match hc.Status's
// own strings.
const (
	TaskStatusOpen       = string(hc.StatusOpen)
	TaskStatusInProgress = string(hc.StatusInProgress)
	TaskStatusDone       = string(hc.StatusDone)
	TaskStatusCancelled  = string(hc.StatusCancelled)
)

// TaskType mirrors hc's item type vocabulary. Values match hc.ItemType's own
// strings.
const (
	TaskTypeEpic = string(hc.ItemTypeEpic)
	TaskTypeTask = string(hc.ItemTypeTask)
)

// ErrTaskNotFound is the seam-local translation of hc.ErrNotFound, so a core
// service can classify a missing task without importing the shared hc
// package.
var ErrTaskNotFound = errors.New("task not found")

// HoneycombManagement is the shared hc surface the desktop's tasks view
// manages issues through. Every method matches hive's HoneycombService
// structurally, so a signature change in the shared package breaks this file rather than
// the core.
type HoneycombManagement interface {
	ListItems(ctx context.Context, filter hc.ListFilter) ([]hc.Item, error)
	GetItem(ctx context.Context, id string) (hc.Item, error)
	UpdateItem(ctx context.Context, id string, update hc.ItemUpdate) (hc.Item, error)
	ListComments(ctx context.Context, itemID string) ([]hc.Comment, error)
	ListRepoKeys(ctx context.Context) ([]string, error)
	DeleteItem(ctx context.Context, id string) error
	Prune(ctx context.Context, opts hc.PruneOpts) (int, error)
}

// TaskItem is one hc item as the desktop's tasks list sees it. Desc is
// deliberately absent: the list view never needs it, and TaskDetail re-reads
// it on demand.
type TaskItem struct {
	ID        string    `json:"id"`
	RepoKey   string    `json:"repoKey"`
	EpicID    string    `json:"epicId"`
	ParentID  string    `json:"parentId"`
	SessionID string    `json:"sessionId"`
	Title     string    `json:"title"`
	Type      string    `json:"type"`
	Status    string    `json:"status"`
	Blocked   bool      `json:"blocked"`
	Depth     int       `json:"depth"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// TaskBlocker is one explicit blocker as shown on a task's detail view. A
// blocker whose item has since been deleted keeps its ID with an empty Title
// rather than being dropped from the list.
type TaskBlocker struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

// TaskComment is one comment on a task, in the order hc stored it.
type TaskComment struct {
	ID        string    `json:"id"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"createdAt"`
}

// TaskDetail is one hc item read in full, for a detail view. Blockers are
// resolved by TaskDetail's own per-blocker GetItem reads: ListItems never
// fills BlockerIDs, so there is no cheaper way to a blocker's title.
type TaskDetail struct {
	TaskItem
	Desc     string        `json:"desc"`
	Blockers []TaskBlocker `json:"blockers"`
	Comments []TaskComment `json:"comments"`
}

// TaskPruneOptions controls PruneTasks. Statuses is deliberately not exposed
// here: leaving it unset lets PruneTasks pass nil through to the store's own
// done+cancelled default (hc.Store.Prune).
type TaskPruneOptions struct {
	OlderThan time.Duration `json:"olderThan"`
	RepoKey   string        `json:"repoKey"`
	DryRun    bool          `json:"dryRun"`
}

// HiveHoneycomb adapts Hive's hc (Honeycomb) issue tracker to the desktop's
// tasks view.
type HiveHoneycomb struct {
	tasks HoneycombManagement
}

func NewHiveHoneycomb(tasks HoneycombManagement) *HiveHoneycomb {
	return &HiveHoneycomb{tasks: tasks}
}

// ListTasks returns every item for repoKey, epics and tasks alike, with no
// status filter: the tasks view's filter groups don't map to hc's single
// status field, so filtering happens client-side against the full list.
func (h *HiveHoneycomb) ListTasks(ctx context.Context, repoKey string) ([]TaskItem, error) {
	items, err := h.tasks.ListItems(ctx, hc.ListFilter{RepoKey: repoKey})
	if err != nil {
		return nil, fmt.Errorf("list hc items: %w", err)
	}
	out := make([]TaskItem, 0, len(items))
	for _, item := range items {
		out = append(out, taskItemOf(item))
	}
	return out, nil
}

// TaskDetail reads one item in full: its own fields, comments, and a title
// for each explicit blocker.
func (h *HiveHoneycomb) TaskDetail(ctx context.Context, id string) (TaskDetail, error) {
	item, err := h.tasks.GetItem(ctx, id)
	if err != nil {
		if errors.Is(err, hc.ErrNotFound) {
			return TaskDetail{}, fmt.Errorf("%w: %w", ErrTaskNotFound, err)
		}
		return TaskDetail{}, fmt.Errorf("get hc item: %w", err)
	}

	comments, err := h.tasks.ListComments(ctx, id)
	if err != nil {
		return TaskDetail{}, fmt.Errorf("list hc comments: %w", err)
	}

	blockers := make([]TaskBlocker, 0, len(item.BlockerIDs))
	for _, blockerID := range item.BlockerIDs {
		blocker, err := h.tasks.GetItem(ctx, blockerID)
		if err != nil {
			if errors.Is(err, hc.ErrNotFound) {
				// The blocker item is gone, but the edge isn't this call's to
				// fix — surface the ID with no title rather than error.
				blockers = append(blockers, TaskBlocker{ID: blockerID})
				continue
			}
			return TaskDetail{}, fmt.Errorf("get hc blocker item %q: %w", blockerID, err)
		}
		blockers = append(blockers, TaskBlocker{ID: blocker.ID, Title: blocker.Title, Status: string(blocker.Status)})
	}

	comm := make([]TaskComment, 0, len(comments))
	for _, c := range comments {
		comm = append(comm, TaskComment{ID: c.ID, Message: c.Message, CreatedAt: c.CreatedAt})
	}

	return TaskDetail{
		TaskItem: taskItemOf(item),
		Desc:     item.Desc,
		Blockers: blockers,
		Comments: comm,
	}, nil
}

// SetTaskStatus sets id's status. Setting a terminal status on an epic
// cascades to every non-terminal descendant, but only when the update
// actually changes the epic's status — hive.HoneycombService.UpdateItem's
// own behavior, not reimplemented here.
func (h *HiveHoneycomb) SetTaskStatus(ctx context.Context, id, status string) error {
	s := hc.Status(status)
	if _, err := h.tasks.UpdateItem(ctx, id, hc.ItemUpdate{Status: &s}); err != nil {
		if errors.Is(err, hc.ErrNotFound) {
			return fmt.Errorf("%w: %w", ErrTaskNotFound, err)
		}
		return fmt.Errorf("update hc item status: %w", err)
	}
	return nil
}

// DeleteTask deletes id and its whole subtree, comments included. GetItem
// runs first so a missing id reports ErrTaskNotFound instead of DeleteItem's
// own silent no-op on an unknown ID.
func (h *HiveHoneycomb) DeleteTask(ctx context.Context, id string) error {
	if _, err := h.tasks.GetItem(ctx, id); err != nil {
		if errors.Is(err, hc.ErrNotFound) {
			return fmt.Errorf("%w: %w", ErrTaskNotFound, err)
		}
		return fmt.Errorf("get hc item: %w", err)
	}
	if err := h.tasks.DeleteItem(ctx, id); err != nil {
		return fmt.Errorf("delete hc item: %w", err)
	}
	return nil
}

// PruneTasks removes terminal, stale items. A terminal-and-old root expands
// to every descendant regardless of that descendant's own status or age —
// the store's own hc.Store.Prune behavior, not reimplemented here.
func (h *HiveHoneycomb) PruneTasks(ctx context.Context, opts TaskPruneOptions) (int, error) {
	count, err := h.tasks.Prune(ctx, hc.PruneOpts{
		OlderThan: opts.OlderThan,
		RepoKey:   opts.RepoKey,
		DryRun:    opts.DryRun,
	})
	if err != nil {
		return count, fmt.Errorf("prune hc items: %w", err)
	}
	return count, nil
}

// TaskRepoKeys returns every distinct repo key that has at least one hc item.
func (h *HiveHoneycomb) TaskRepoKeys(ctx context.Context) ([]string, error) {
	keys, err := h.tasks.ListRepoKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("list hc repo keys: %w", err)
	}
	return keys, nil
}

func taskItemOf(item hc.Item) TaskItem {
	return TaskItem{
		ID:        item.ID,
		RepoKey:   item.RepoKey,
		EpicID:    item.EpicID,
		ParentID:  item.ParentID,
		SessionID: item.SessionID,
		Title:     item.Title,
		Type:      string(item.Type),
		Status:    string(item.Status),
		Blocked:   item.Blocked,
		Depth:     item.Depth,
		CreatedAt: item.CreatedAt,
		UpdatedAt: item.UpdatedAt,
	}
}
