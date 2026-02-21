package stores

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/data/db"
)

// SessionStore implements session.Store using SQLite.
type SessionStore struct {
	db *db.DB
}

var _ session.Store = (*SessionStore)(nil)

// NewSessionStore creates a new SQLite-backed session store.
func NewSessionStore(db *db.DB) *SessionStore {
	return &SessionStore{db: db}
}

// List returns all sessions.
func (s *SessionStore) List(ctx context.Context) ([]session.Session, error) {
	rows, err := s.db.Queries().ListSessions(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}

	sessions := make([]session.Session, 0, len(rows))
	for _, row := range rows {
		sess, err := rowToSession(row)
		if err != nil {
			return nil, fmt.Errorf("failed to convert session: %w", err)
		}
		sessions = append(sessions, sess)
	}

	return sessions, nil
}

// Get returns a session by ID. Returns ErrNotFound if not found.
func (s *SessionStore) Get(ctx context.Context, id string) (session.Session, error) {
	row, err := s.db.Queries().GetSession(ctx, id)
	if IsNotFoundError(err) {
		return session.Session{}, session.ErrNotFound
	}
	if err != nil {
		return session.Session{}, fmt.Errorf("failed to get session: %w", err)
	}

	sess, err := rowToSession(row)
	if err != nil {
		return session.Session{}, fmt.Errorf("failed to convert session: %w", err)
	}

	return sess, nil
}

// Save creates or updates a session.
func (s *SessionStore) Save(ctx context.Context, sess session.Session) error {
	// Marshal metadata to JSON
	var metadataJSON sql.NullString
	if len(sess.Metadata) > 0 {
		data, err := json.Marshal(sess.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
		metadataJSON = sql.NullString{String: string(data), Valid: true}
	}

	cloneStrategy := sess.CloneStrategy
	if cloneStrategy == "" {
		cloneStrategy = "full"
	}

	err := s.db.Queries().SaveSession(ctx, db.SaveSessionParams{
		ID:            sess.ID,
		Name:          sess.Name,
		Slug:          sess.Slug,
		Path:          sess.Path,
		Remote:        sess.Remote,
		State:         string(sess.State),
		CloneStrategy: cloneStrategy,
		Metadata:      metadataJSON,
		CreatedAt:     sess.CreatedAt.UnixNano(),
		UpdatedAt:     sess.UpdatedAt.UnixNano(),
	})
	if err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}

	return nil
}

// Delete removes a session by ID. Returns ErrNotFound if not found.
func (s *SessionStore) Delete(ctx context.Context, id string) error {
	// Check if session exists first
	_, err := s.db.Queries().GetSession(ctx, id)
	if IsNotFoundError(err) {
		return session.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("failed to check session existence: %w", err)
	}

	err = s.db.Queries().DeleteSession(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	return nil
}

// FindRecyclable returns a recyclable session for the given remote.
// Returns ErrNoRecyclable if none available.
func (s *SessionStore) FindRecyclable(ctx context.Context, remote string) (session.Session, error) {
	row, err := s.db.Queries().FindRecyclableSession(ctx, remote)
	if IsNotFoundError(err) {
		return session.Session{}, session.ErrNoRecyclable
	}
	if err != nil {
		return session.Session{}, fmt.Errorf("failed to find recyclable session: %w", err)
	}

	sess, err := rowToSession(row)
	if err != nil {
		return session.Session{}, fmt.Errorf("failed to convert session: %w", err)
	}

	return sess, nil
}

// rowToSession converts a db.Session to a session.Session.
func rowToSession(row db.Session) (session.Session, error) {
	// Unmarshal metadata from JSON
	var metadata map[string]string
	if row.Metadata.Valid {
		if err := json.Unmarshal([]byte(row.Metadata.String), &metadata); err != nil {
			return session.Session{}, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	cloneStrategy := row.CloneStrategy
	if cloneStrategy == "" {
		cloneStrategy = "full"
	}

	return session.Session{
		ID:            row.ID,
		Name:          row.Name,
		Slug:          row.Slug,
		Path:          row.Path,
		Remote:        row.Remote,
		State:         session.State(row.State),
		CloneStrategy: cloneStrategy,
		Metadata:      metadata,
		CreatedAt:     time.Unix(0, row.CreatedAt),
		UpdatedAt:     time.Unix(0, row.UpdatedAt),
	}, nil
}
