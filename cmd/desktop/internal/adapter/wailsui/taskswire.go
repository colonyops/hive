package wailsui

import (
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/internal/domain/hc"
)

// TaskItem is one hc item as the tasks list sees it. Desc is absent: the list
// never needs it, and TaskDetail reads it on demand.
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

func taskItemsOf(items []hc.Item) []TaskItem {
	out := make([]TaskItem, 0, len(items))
	for _, item := range items {
		out = append(out, taskItemOf(item))
	}
	return out
}

// TaskBlocker is one explicit blocker on a task's detail view. A blocker whose
// item has since been deleted keeps its ID with an empty Title.
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

// TaskDetail is one hc item read in full, for a detail view.
type TaskDetail struct {
	TaskItem
	Desc     string        `json:"desc"`
	Blockers []TaskBlocker `json:"blockers"`
	Comments []TaskComment `json:"comments"`
}

func taskDetailOf(d app.TaskDetail) TaskDetail {
	blockers := make([]TaskBlocker, 0, len(d.Blockers))
	for _, b := range d.Blockers {
		blockers = append(blockers, TaskBlocker{ID: b.ID, Title: b.Title, Status: string(b.Status)})
	}
	comments := make([]TaskComment, 0, len(d.Comments))
	for _, c := range d.Comments {
		comments = append(comments, TaskComment{ID: c.ID, Message: c.Message, CreatedAt: c.CreatedAt})
	}
	return TaskDetail{TaskItem: taskItemOf(d.Item), Desc: d.Desc, Blockers: blockers, Comments: comments}
}
