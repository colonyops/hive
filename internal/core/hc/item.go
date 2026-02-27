package hc

import (
	"fmt"
	"time"

	"github.com/hay-kot/criterio"
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
	Blocked   bool      `json:"blocked"` // computed at read time: true when the item has open/in_progress children
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
	var epicIDErr error
	switch {
	case i.IsEpic() && i.EpicID != "":
		epicIDErr = criterio.Nest("epic_id", fmt.Errorf("must not be set for epic items"))
	case !i.IsEpic() && i.EpicID == "":
		epicIDErr = criterio.Nest("epic_id", fmt.Errorf("required for non-epic items"))
	}

	return criterio.ValidateStruct(
		criterio.Run("id", i.ID, criterio.StrNotEmpty),
		criterio.Run("title", i.Title, criterio.StrNotEmpty),
		criterio.Run("type", i.Type, criterio.OneOf(ItemTypeEpic, ItemTypeTask)),
		criterio.Run("status", i.Status, criterio.OneOf(StatusOpen, StatusInProgress, StatusDone, StatusCancelled)),
		criterio.Run("depth", i.Depth, criterio.Between(0, 10)),
		epicIDErr,
	)
}
