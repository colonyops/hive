package review

import (
	"context"
	"errors"
)

// Sentinel errors for review operations.
var (
	ErrSessionNotFound = errors.New("review session not found")
)

// Store defines persistence operations for review sessions and comments.
type Store interface {
	// CreateSession creates a new review session for a document with content hash.
	// Returns error if session for document+hash already exists.
	CreateSession(ctx context.Context, documentPath string, contentHash string) (Session, error)

	// GetSession returns the most recent review session for the given document.
	// Returns ErrSessionNotFound if not found.
	GetSession(ctx context.Context, documentPath string) (Session, error)

	// GetSessionByHash returns a review session for the given document and content hash.
	// Returns ErrSessionNotFound if not found.
	GetSessionByHash(ctx context.Context, documentPath string, contentHash string) (Session, error)

	// CleanupStaleSessions removes review sessions for a document with different content hash.
	// Used to clean up sessions when document content changes.
	CleanupStaleSessions(ctx context.Context, documentPath string, currentHash string) error

	// FinalizeSession marks a review session as finalized.
	// Returns ErrSessionNotFound if not found.
	FinalizeSession(ctx context.Context, sessionID string) error

	// DeleteSession removes a review session and all associated comments.
	// Returns ErrSessionNotFound if not found.
	DeleteSession(ctx context.Context, sessionID string) error

	// SaveComment adds a comment to a review session.
	SaveComment(ctx context.Context, comment Comment) error

	// ListComments returns all comments for a review session, sorted by start line.
	ListComments(ctx context.Context, sessionID string) ([]Comment, error)

	// DeleteComment removes a specific comment.
	DeleteComment(ctx context.Context, commentID string) error
}
