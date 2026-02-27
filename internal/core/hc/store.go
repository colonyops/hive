package hc

import (
	"context"
	"time"
)

// Store persists hc items and activity to durable storage.
type Store interface {
	// CreateItem persists one item.
	CreateItem(ctx context.Context, item Item) error
	// CreateItemBatch persists multiple items atomically.
	CreateItemBatch(ctx context.Context, items []Item) error
	// GetItem returns an item by ID.
	GetItem(ctx context.Context, id string) (Item, error)
	// UpdateItem applies a partial update and returns the updated item.
	UpdateItem(ctx context.Context, id string, update ItemUpdate) (Item, error)
	// ListItems returns items matching the filter.
	ListItems(ctx context.Context, filter ListFilter) ([]Item, error)
	// NextItem returns the next actionable leaf item for the filter.
	// Actionable means status is open/in_progress and no open/in_progress children.
	NextItem(ctx context.Context, filter NextFilter) (Item, bool, error)
	// DeleteItem removes an item by ID.
	// This is an internal maintenance path used by prune operations.
	DeleteItem(ctx context.Context, id string) error
	// LogActivity records one activity entry.
	LogActivity(ctx context.Context, a Activity) error
	// ListActivity lists activity entries for an item.
	ListActivity(ctx context.Context, itemID string) ([]Activity, error)
	// LatestCheckpoint returns the most recent checkpoint activity for an item.
	// The bool is false when no checkpoint exists.
	LatestCheckpoint(ctx context.Context, itemID string) (Activity, bool, error)
	// Prune removes old items and related activity according to options.
	Prune(ctx context.Context, opts PruneOpts) (int, error)
}

// ItemUpdate carries partial updates to an Item. Nil pointer fields are not changed.
type ItemUpdate struct {
	Status    *Status
	SessionID *string
}

// ListFilter controls which items are returned by ListItems.
type ListFilter struct {
	RepoKey   string
	EpicID    string
	SessionID string
	Status    *Status
}

// NextFilter selects candidate items for NextItem.
type NextFilter struct {
	EpicID    string
	SessionID string
}

// PruneOpts controls which items are removed by Prune.
type PruneOpts struct {
	OlderThan time.Duration
	Statuses  []Status
	DryRun    bool
}
