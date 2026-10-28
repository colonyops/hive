package session

import (
	"context"
	"errors"
)

// Sentinel errors for session operations.
var (
	ErrNotFound      = errors.New("session not found")
	ErrDuplicateName = errors.New("session name already exists")
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
}
