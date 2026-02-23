package todo

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned when a todo item does not exist.
var ErrNotFound = errors.New("todo not found")

// ListFilter controls which todos are returned by List.
type ListFilter struct {
	Status    *Status // nil = all
	SessionID string  // empty = all
	Scheme    string  // filter by URI scheme; empty = all
}

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
