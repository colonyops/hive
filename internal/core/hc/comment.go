package hc

import "time"

// Comment records a note attached to an item. Comments are used for two
// purposes: capturing design decisions made during implementation, and
// leaving context for handoffs when stopping mid-implementation.
type Comment struct {
	ID        string    `json:"id"`
	ItemID    string    `json:"item_id"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}
