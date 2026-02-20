package todo

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrNotFound is returned when a TODO item does not exist.
	ErrNotFound = errors.New("todo item not found")
	// ErrDuplicate is returned when a duplicate pending item already exists for a file path.
	ErrDuplicate = errors.New("duplicate pending todo item")
	// ErrRateLimited is returned when a session exceeds the custom item creation rate limit.
	ErrRateLimited = errors.New("rate limit exceeded for custom todo items")
)

// ListFilter controls which items are returned by List.
type ListFilter struct {
	Status     Status // empty means all statuses
	SessionID  string // empty means all sessions
	RepoRemote string // empty means all repos
}

// Store defines the interface for TODO item persistence.
type Store interface {
	// Create persists a new TODO item.
	// The store populates ID, Status, CreatedAt, and UpdatedAt if not already set.
	// Returns ErrDuplicate if a pending item already exists for the same file path.
	Create(ctx context.Context, item *Item) error

	// Get returns a single TODO item by ID.
	// Returns ErrNotFound if the item does not exist.
	Get(ctx context.Context, id string) (Item, error)

	// List returns TODO items matching the filter, ordered by created_at DESC.
	List(ctx context.Context, filter ListFilter) ([]Item, error)

	// UpdateStatus changes the status of a TODO item.
	// Returns ErrNotFound if the item does not exist.
	UpdateStatus(ctx context.Context, id string, status Status) error

	// DismissByPath dismisses all pending items matching the given file path.
	DismissByPath(ctx context.Context, filePath string) error

	// CompleteByPath completes all pending items matching the given file path.
	CompleteByPath(ctx context.Context, filePath string) error

	// CountPending returns the total number of pending items.
	CountPending(ctx context.Context) (int64, error)

	// CountPendingBySession returns pending item count for a specific session.
	CountPendingBySession(ctx context.Context, sessionID string) (int64, error)

	// CountCustomBySessionSince counts custom items created by a session since a given time.
	// Used for rate limiting.
	CountCustomBySessionSince(ctx context.Context, sessionID string, since time.Time) (int64, error)
}
