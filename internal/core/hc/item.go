package hc

import (
	"fmt"
	"time"
)

// ItemType categorizes an item as an epic or a task.
//
// ENUM(
//
//	epic
//	task
//
// )
type ItemType string

// Status tracks the lifecycle of an hc item.
//
// ENUM(
//
//	open
//	in_progress
//	done
//	cancelled
//
// )
type Status string

// Item represents a single unit of work tracked by hc.
type Item struct {
	ID        string    `json:"id"`
	RepoKey   string    `json:"repo_key"`   // "owner/repo" or ""
	EpicID    string    `json:"epic_id"`    // "" for epics themselves
	ParentID  string    `json:"parent_id"`  // "" for root items
	SessionID string    `json:"session_id"` // assigned agent session, may be ""
	Title     string    `json:"title"`
	Desc      string    `json:"desc"`
	Type      ItemType  `json:"type"`
	Status    Status    `json:"status"`
	Blocked   bool      `json:"blocked"` // computed: has open children
	Depth     int       `json:"depth"`   // 0 for epics/roots
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// IsEpic reports whether the item is an epic.
func (i Item) IsEpic() bool {
	return i.Type == ItemTypeEpic
}

// IsRoot reports whether the item has no parent.
func (i Item) IsRoot() bool {
	return i.ParentID == ""
}

// Validate checks that an Item has internally consistent required fields.
func (i Item) Validate() error {
	if i.ID == "" {
		return fmt.Errorf("item ID is required")
	}
	if i.Title == "" {
		return fmt.Errorf("item title is required")
	}
	if !i.Type.IsValid() {
		return fmt.Errorf("invalid type %q", i.Type)
	}
	if !i.Status.IsValid() {
		return fmt.Errorf("invalid status %q", i.Status)
	}
	if i.Depth < 0 || i.Depth > 10 {
		return fmt.Errorf("depth must be between 0 and 10, got %d", i.Depth)
	}
	if i.IsEpic() && i.EpicID != "" {
		return fmt.Errorf("epic items must not have an epic_id")
	}
	if !i.IsEpic() && i.EpicID == "" {
		return fmt.Errorf("non-epic items must have an epic_id")
	}
	return nil
}
