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

// TaskDetail is one hc item read in full, for a detail view.
type TaskDetail struct {
	TaskItem
	Desc     string            `json:"desc"`
	Blockers []app.TaskBlocker `json:"blockers"`
	Comments []app.TaskComment `json:"comments"`
}

func taskDetailOf(d app.TaskDetail) TaskDetail {
	return TaskDetail{TaskItem: taskItemOf(d.Item), Desc: d.Desc, Blockers: d.Blockers, Comments: d.Comments}
}
