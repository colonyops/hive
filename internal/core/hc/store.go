package hc

import (
	"context"
	"time"
)

// Store persists hc items and activity to durable storage.
type Store interface {
	CreateItem(ctx context.Context, item Item) error
	CreateItemBatch(ctx context.Context, items []Item) error
	GetItem(ctx context.Context, id string) (Item, error)
	UpdateItem(ctx context.Context, id string, update ItemUpdate) (Item, error)
	ListItems(ctx context.Context, filter ListFilter) ([]Item, error)
	NextItem(ctx context.Context, filter NextFilter) (Item, bool, error)
	DeleteItem(ctx context.Context, id string) error
	LogActivity(ctx context.Context, a Activity) error
	ListActivity(ctx context.Context, itemID string) ([]Activity, error)
	LatestCheckpoint(ctx context.Context, itemID string) (Activity, bool, error)
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
