package stores

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hay-kot/hive/internal/core/review"
	"github.com/hay-kot/hive/internal/data/db"
)

// ReviewStore implements review.Store using SQLite.
type ReviewStore struct {
	db *db.DB
}

var _ review.Store = (*ReviewStore)(nil)

// NewReviewStore creates a new SQLite-backed review store.
func NewReviewStore(db *db.DB) *ReviewStore {
	return &ReviewStore{db: db}
}

// CreateSession creates a new review session for a document with content hash.
// For diff sessions, use CreateDiffSession instead.
func (s *ReviewStore) CreateSession(ctx context.Context, documentPath string, contentHash string) (review.Session, error) {
	sessionID := uuid.NewString()
	now := time.Now()

	err := s.db.Queries().CreateReviewSession(ctx, db.CreateReviewSessionParams{
		ID:           sessionID,
		DocumentPath: documentPath,
		ContentHash:  contentHash,
		CreatedAt:    now.UnixNano(),
		FinalizedAt:  sql.NullInt64{Valid: false},
		SessionName:  "",
		DiffContext:  "",
	})
	if err != nil {
		return review.Session{}, fmt.Errorf("failed to create review session: %w", err)
	}

	return review.Session{
		ID:           sessionID,
		DocumentPath: documentPath,
		ContentHash:  contentHash,
		CreatedAt:    now,
		FinalizedAt:  nil,
		SessionName:  "",
		DiffContext:  "",
	}, nil
}

// CreateDiffSession creates a new review session for a diff with context.
func (s *ReviewStore) CreateDiffSession(ctx context.Context, sessionName string, diffContext string, documentPath string, contentHash string) (review.Session, error) {
	sessionID := uuid.NewString()
	now := time.Now()

	err := s.db.Queries().CreateReviewSession(ctx, db.CreateReviewSessionParams{
		ID:           sessionID,
		DocumentPath: documentPath,
		ContentHash:  contentHash,
		CreatedAt:    now.UnixNano(),
		FinalizedAt:  sql.NullInt64{Valid: false},
		SessionName:  sessionName,
		DiffContext:  diffContext,
	})
	if err != nil {
		return review.Session{}, fmt.Errorf("failed to create diff review session: %w", err)
	}

	return review.Session{
		ID:           sessionID,
		DocumentPath: documentPath,
		ContentHash:  contentHash,
		CreatedAt:    now,
		FinalizedAt:  nil,
		SessionName:  sessionName,
		DiffContext:  diffContext,
	}, nil
}

// GetSession returns the most recent review session for the given document.
func (s *ReviewStore) GetSession(ctx context.Context, documentPath string) (review.Session, error) {
	row, err := s.db.Queries().GetReviewSessionByDocPath(ctx, documentPath)
	if IsNotFoundError(err) {
		return review.Session{}, review.ErrSessionNotFound
	}
	if err != nil {
		return review.Session{}, fmt.Errorf("failed to get review session: %w", err)
	}

	return rowToReviewSession(row), nil
}

// GetSessionByContext returns a review session for the given session name and diff context.
func (s *ReviewStore) GetSessionByContext(ctx context.Context, sessionName string, diffContext string) (review.Session, error) {
	row, err := s.db.Queries().GetReviewSessionByContext(ctx, db.GetReviewSessionByContextParams{
		SessionName: sessionName,
		DiffContext: diffContext,
	})
	if IsNotFoundError(err) {
		return review.Session{}, review.ErrSessionNotFound
	}
	if err != nil {
		return review.Session{}, fmt.Errorf("failed to get review session by context: %w", err)
	}

	return rowToReviewSession(row), nil
}

// GetSessionByHash returns a review session for the given document and content hash.
func (s *ReviewStore) GetSessionByHash(ctx context.Context, documentPath string, contentHash string) (review.Session, error) {
	row, err := s.db.Queries().GetReviewSessionByDocPathAndHash(ctx, db.GetReviewSessionByDocPathAndHashParams{
		DocumentPath: documentPath,
		ContentHash:  contentHash,
	})
	if IsNotFoundError(err) {
		return review.Session{}, review.ErrSessionNotFound
	}
	if err != nil {
		return review.Session{}, fmt.Errorf("failed to get review session by hash: %w", err)
	}

	return rowToReviewSession(row), nil
}

// CleanupStaleSessions removes review sessions for a document with different content hash.
func (s *ReviewStore) CleanupStaleSessions(ctx context.Context, documentPath string, currentHash string) error {
	err := s.db.Queries().DeleteReviewSessionsByDocPath(ctx, db.DeleteReviewSessionsByDocPathParams{
		DocumentPath: documentPath,
		ContentHash:  currentHash,
	})
	if err != nil {
		return fmt.Errorf("failed to cleanup stale sessions: %w", err)
	}
	return nil
}

// FinalizeSession marks a review session as finalized.
func (s *ReviewStore) FinalizeSession(ctx context.Context, sessionID string) error {
	now := time.Now()
	err := s.db.Queries().FinalizeReviewSession(ctx, db.FinalizeReviewSessionParams{
		FinalizedAt: sql.NullInt64{Int64: now.UnixNano(), Valid: true},
		ID:          sessionID,
	})
	if err != nil {
		return fmt.Errorf("failed to finalize review session: %w", err)
	}
	return nil
}

// DeleteSession removes a review session and all associated comments.
func (s *ReviewStore) DeleteSession(ctx context.Context, sessionID string) error {
	err := s.db.Queries().DeleteReviewSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to delete review session: %w", err)
	}
	return nil
}

// SaveComment adds a comment to a review session.
func (s *ReviewStore) SaveComment(ctx context.Context, comment review.Comment) error {
	err := s.db.Queries().SaveReviewComment(ctx, db.SaveReviewCommentParams{
		ID:          comment.ID,
		SessionID:   comment.SessionID,
		StartLine:   int64(comment.StartLine),
		EndLine:     int64(comment.EndLine),
		ContextText: comment.ContextText,
		CommentText: comment.CommentText,
		CreatedAt:   comment.CreatedAt.UnixNano(),
		Side:        comment.Side,
	})
	if err != nil {
		return fmt.Errorf("failed to save review comment: %w", err)
	}
	return nil
}

// ListComments returns all comments for a review session, sorted by start line.
func (s *ReviewStore) ListComments(ctx context.Context, sessionID string) ([]review.Comment, error) {
	rows, err := s.db.Queries().ListReviewComments(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to list review comments: %w", err)
	}

	comments := make([]review.Comment, 0, len(rows))
	for _, row := range rows {
		comments = append(comments, rowToReviewComment(row))
	}

	return comments, nil
}

// UpdateComment updates the comment text for an existing comment.
func (s *ReviewStore) UpdateComment(ctx context.Context, comment review.Comment) error {
	err := s.db.Queries().UpdateReviewComment(ctx, db.UpdateReviewCommentParams{
		CommentText: comment.CommentText,
		ID:          comment.ID,
	})
	if err != nil {
		return fmt.Errorf("failed to update review comment: %w", err)
	}
	return nil
}

// DeleteComment removes a specific comment.
func (s *ReviewStore) DeleteComment(ctx context.Context, commentID string) error {
	err := s.db.Queries().DeleteReviewComment(ctx, commentID)
	if err != nil {
		return fmt.Errorf("failed to delete review comment: %w", err)
	}
	return nil
}

// SessionInfo contains session data with comment count for efficient batch queries.
type SessionInfo struct {
	Session      review.Session
	CommentCount int
}

// GetAllActiveSessionsWithCounts returns all active (non-finalized) sessions with comment counts.
// This is optimized for batch operations like the document picker.
func (s *ReviewStore) GetAllActiveSessionsWithCounts(ctx context.Context) (map[string]SessionInfo, error) {
	rows, err := s.db.Queries().GetAllActiveSessionsWithCounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get active sessions with counts: %w", err)
	}

	// Convert to map keyed by document path for fast lookup
	result := make(map[string]SessionInfo)
	for _, row := range rows {
		var finalizedAt *time.Time
		if row.FinalizedAt.Valid {
			t := time.Unix(0, row.FinalizedAt.Int64)
			finalizedAt = &t
		}

		session := review.Session{
			ID:           row.ID,
			DocumentPath: row.DocumentPath,
			ContentHash:  row.ContentHash,
			CreatedAt:    time.Unix(0, row.CreatedAt),
			FinalizedAt:  finalizedAt,
			SessionName:  row.SessionName,
			DiffContext:  row.DiffContext,
		}

		result[row.DocumentPath] = SessionInfo{
			Session:      session,
			CommentCount: int(row.CommentCount),
		}
	}

	return result, nil
}

// rowToReviewSession converts a db.ReviewSession to a review.Session.
func rowToReviewSession(row db.ReviewSession) review.Session {
	var finalizedAt *time.Time
	if row.FinalizedAt.Valid {
		t := time.Unix(0, row.FinalizedAt.Int64)
		finalizedAt = &t
	}

	return review.Session{
		ID:           row.ID,
		DocumentPath: row.DocumentPath,
		ContentHash:  row.ContentHash,
		CreatedAt:    time.Unix(0, row.CreatedAt),
		FinalizedAt:  finalizedAt,
		SessionName:  row.SessionName,
		DiffContext:  row.DiffContext,
	}
}

// rowToReviewComment converts a db.ReviewComment to a review.Comment.
func rowToReviewComment(row db.ReviewComment) review.Comment {
	return review.Comment{
		ID:          row.ID,
		SessionID:   row.SessionID,
		StartLine:   int(row.StartLine),
		EndLine:     int(row.EndLine),
		ContextText: row.ContextText,
		CommentText: row.CommentText,
		CreatedAt:   time.Unix(0, row.CreatedAt),
		Side:        row.Side,
	}
}
