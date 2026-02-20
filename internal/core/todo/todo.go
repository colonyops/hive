// Package todo defines the TODO item domain model for operator action tracking.
package todo

import "time"

// ItemType classifies how a TODO item was created.
type ItemType string

const (
	// ItemTypeFileChange is auto-generated when a file changes in a context directory.
	ItemTypeFileChange ItemType = "file_change"
	// ItemTypeCustom is created via CLI by agents or operators.
	ItemTypeCustom ItemType = "custom"
)

// Status represents the lifecycle state of a TODO item.
type Status string

const (
	StatusPending   Status = "pending"
	StatusCompleted Status = "completed"
	StatusDismissed Status = "dismissed"
)

// Item represents a single actionable TODO for the operator.
type Item struct {
	ID          string    `json:"id"`
	Type        ItemType  `json:"type"`
	Status      Status    `json:"status"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	FilePath    string    `json:"file_path,omitempty"`
	SessionID   string    `json:"session_id,omitempty"`
	RepoRemote  string    `json:"repo_remote"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
