package hc

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned when a requested item does not exist.
var ErrNotFound = errors.New("not found")

// ErrCyclicDependency is returned when an AddBlocker call would create a cycle.
var ErrCyclicDependency = errors.New("cyclic dependency")

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
	// ListRepoKeys returns all distinct, non-empty repo keys in sorted order.
	ListRepoKeys(ctx context.Context) ([]string, error)
	// CreateBulkWithEdges creates items and wires their explicit blocker edges atomically.
	// edges is a slice of [blockerID, blockedID] pairs. Cycle validation must be done
	// by the caller before invoking this method; this method only enforces FK constraints.
	CreateBulkWithEdges(ctx context.Context, items []Item, edges [][2]string) error
	// AddBlocker records that blockerID blocks blockedID atomically, including cycle detection.
	// Returns ErrCyclicDependency if the edge would create a cycle.
	AddBlocker(ctx context.Context, blockerID, blockedID string) error
	// RemoveBlocker removes the blocker relationship.
	RemoveBlocker(ctx context.Context, blockerID, blockedID string) error
	// ListBlockers returns IDs of open/in_progress items that explicitly block the given item.
	ListBlockers(ctx context.Context, itemID string) ([]string, error)
	// ListBlockerEdges returns all blocker edges as [blockerID, blockedID] pairs.
	// Used by callers that need the full graph (e.g. for pre-validation before bulk create).
	ListBlockerEdges(ctx context.Context) ([][2]string, error)
	// BulkUpdateStatus sets the status of all non-terminal descendants of the given
	// epic to the specified status. Items already in a terminal status (done, cancelled)
	// are not modified.
	BulkUpdateStatus(ctx context.Context, epicID string, status Status) error
}

// ItemUpdate carries partial updates to an Item. Nil pointer fields are not changed.
type ItemUpdate struct {
	Status    *Status
	SessionID *string
	Title     *string
	Desc      *string
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
	if f.EpicID != "" && item.EpicID != f.EpicID {
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
	RepoKey   string
}

// PruneOpts controls which items are removed by Prune.
type PruneOpts struct {
	OlderThan time.Duration
	Statuses  []Status
	RepoKey   string
	DryRun    bool
}
