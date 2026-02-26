package hc

import "time"

// ActivityType categorizes the kind of activity logged against an item.
//
// ENUM(
//
//	update
//	comment
//	checkpoint
//	status_change
//
// )
type ActivityType string

// Activity records a discrete event or note attached to an item.
type Activity struct {
	ID        string       `json:"id"`
	ItemID    string       `json:"item_id"`
	Type      ActivityType `json:"type"`
	Message   string       `json:"message"`
	CreatedAt time.Time    `json:"created_at"`
}
