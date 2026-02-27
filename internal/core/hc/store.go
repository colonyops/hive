package hc

import (
	"context"
	"time"
)

// Store persists hc items and comments to durable storage.
type Store interface {
	// CreateItems persists one or more items atomically.
	CreateItems(ctx context.Context, items []Item) error
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
	// AddComment records a comment on an item.
	AddComment(ctx context.Context, c Comment) error
	// ListComments returns all comments for an item in chronological order.
	ListComments(ctx context.Context, itemID string) ([]Comment, error)
	// Prune removes old items and related comments according to options.
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

// Matches reports whether item satisfies all non-zero filter fields.
func (f ListFilter) Matches(item Item) bool {
	if f.RepoKey != "" && item.RepoKey != f.RepoKey {
		return false
	}
	if f.SessionID != "" && item.SessionID != f.SessionID {
		return false
	}
	if f.Status != nil && item.Status != *f.Status {
		return false
	}
	return true
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
