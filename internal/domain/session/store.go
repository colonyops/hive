package session

import (
	"context"
	"errors"
)

// Sentinel errors for session operations.
var (
	ErrNotFound      = errors.New("session not found")
	ErrDuplicateName = errors.New("session name already exists")
	// ErrNotActive is a session that is recycled or corrupted, so it has no
	// checkout to open a terminal in.
	ErrNotActive = errors.New("session is not active")
)

// Store defines persistence operations for sessions.
type Store interface {
	// List returns all sessions.
	List(ctx context.Context) ([]Session, error)
	// Get returns a session by ID. Returns ErrNotFound if not found.
	Get(ctx context.Context, id string) (Session, error)
	// Save creates or updates a session.
	Save(ctx context.Context, s Session) error
	// Update reads a session, applies edit, and saves the result as one
	// write that no other process can interleave with. Returns ErrNotFound
	// if not found, and edit's error unchanged.
	Update(ctx context.Context, id string, edit func(*Session) error) (Session, error)
	// Delete removes a session by ID. Returns ErrNotFound if not found.
	Delete(ctx context.Context, id string) error
	// ReplaceRecycled removes the recycled session recycledID and saves next
	// as one write, so two rows never share one clone. Returns ErrNotFound and
	// saves nothing if recycledID is gone or no longer recycled, as when
	// another create took the clone first.
	ReplaceRecycled(ctx context.Context, recycledID string, next Session) error
}
