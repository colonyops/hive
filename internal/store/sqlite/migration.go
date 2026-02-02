package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hay-kot/hive/internal/core/messaging"
	"github.com/hay-kot/hive/internal/core/session"
)

// SessionFile is the root JSON structure for sessions.json
type SessionFile struct {
	Sessions []session.Session `json:"sessions"`
}

// TopicFile is the root JSON structure for per-topic message files.
type TopicFile struct {
	Topic    string              `json:"topic"`
	Messages []messaging.Message `json:"messages"`
}

// MigrateFromJSON migrates data from JSON files to SQLite if conditions are met:
// - sessions.json exists
// - Database has no sessions
// Skips migration if DB already populated to avoid duplicates.
func MigrateFromJSON(ctx context.Context, db *DB, dataDir string) error {
	sessionsPath := filepath.Join(dataDir, "sessions.json")
	topicsDir := filepath.Join(dataDir, "messages", "topics")

	// Check if sessions.json exists
	if _, err := os.Stat(sessionsPath); os.IsNotExist(err) {
		// No JSON files to migrate
		return nil
	}

	// Check if database already has sessions
	sessions, err := db.queries.ListSessions(ctx)
	if err != nil {
		return fmt.Errorf("failed to check existing sessions: %w", err)
	}
	if len(sessions) > 0 {
		// Database already populated, skip migration
		return nil
	}

	// Load and migrate sessions
	if err := migrateSessions(ctx, db, sessionsPath); err != nil {
		return fmt.Errorf("failed to migrate sessions: %w", err)
	}

	// Load and migrate messages
	if err := migrateMessages(ctx, db, topicsDir); err != nil {
		return fmt.Errorf("failed to migrate messages: %w", err)
	}

	return nil
}

// migrateSessions loads sessions from JSON and inserts into SQLite.
func migrateSessions(ctx context.Context, db *DB, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read sessions file: %w", err)
	}

	var file SessionFile
	if err := json.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("failed to parse sessions file: %w", err)
	}

	// Create session store and save each session
	store := NewSessionStore(db)
	for _, sess := range file.Sessions {
		if err := store.Save(ctx, sess); err != nil {
			return fmt.Errorf("failed to save session %s: %w", sess.ID, err)
		}
	}

	return nil
}

// migrateMessages loads messages from per-topic JSON files and inserts into SQLite.
func migrateMessages(ctx context.Context, db *DB, topicsDir string) error {
	// Check if topics directory exists
	if _, err := os.Stat(topicsDir); os.IsNotExist(err) {
		// No messages to migrate
		return nil
	}

	// Read all topic files
	entries, err := os.ReadDir(topicsDir)
	if err != nil {
		return fmt.Errorf("failed to read topics directory: %w", err)
	}

	// Create message store (no retention during migration)
	store := NewMessageStore(db, 0)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		topicPath := filepath.Join(topicsDir, entry.Name())
		if err := migrateTopicFile(ctx, store, topicPath); err != nil {
			// Log error but continue with other topics
			fmt.Fprintf(os.Stderr, "Warning: failed to migrate topic file %s: %v\n", entry.Name(), err)
			continue
		}
	}

	return nil
}

// migrateTopicFile loads a single topic file and inserts messages.
func migrateTopicFile(ctx context.Context, store *MessageStore, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read topic file: %w", err)
	}

	var file TopicFile
	if err := json.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("failed to parse topic file: %w", err)
	}

	// Insert all messages
	for _, msg := range file.Messages {
		if err := store.Publish(ctx, msg); err != nil {
			return fmt.Errorf("failed to publish message %s: %w", msg.ID, err)
		}
	}

	return nil
}
