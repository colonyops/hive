package todo

import (
	"context"
	"time"
)

// Store persists todo items to durable storage.
type Store interface {
	Create(ctx context.Context, t Todo) error
	Get(ctx context.Context, id string) (Todo, error)
	Update(ctx context.Context, id string, status Status) error
	List(ctx context.Context, filter ListFilter) ([]Todo, error)
	CountPending(ctx context.Context) (int, error)
	CountOpen(ctx context.Context) (int, error)
	CountRecentBySession(ctx context.Context, sessionID string, since time.Time) (int, error)
	Delete(ctx context.Context, id string) error
}

// ListFilter controls which todo items are returned by List.
type ListFilter struct {
	Status    *Status
	SessionID string
	Category  *Category
}
