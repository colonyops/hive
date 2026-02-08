package stores

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hay-kot/hive/internal/core/messaging"
	"github.com/hay-kot/hive/internal/core/session"
	"github.com/hay-kot/hive/internal/data/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrateFromJSON_NoFiles(t *testing.T) {
	tempDir := t.TempDir()
	database, err := db.Open(tempDir, db.DefaultOpenOptions())
	require.NoError(t, err, "Open")
	defer func() { _ = database.Close() }()

	ctx := context.Background()

	// Migrate with no JSON files - should succeed silently
	require.NoError(t, MigrateFromJSON(ctx, database, tempDir), "MigrateFromJSON failed")

	// Verify no sessions were created
	store := NewSessionStore(database)
	sessions, _ := store.List(ctx)
	assert.Empty(t, sessions, "Expected 0 sessions, got %d", len(sessions))
}

func TestMigrateFromJSON_Sessions(t *testing.T) {
	tempDir := t.TempDir()
	ctx := context.Background()

	// Create sessions.json
	sessionsPath := filepath.Join(tempDir, "sessions.json")
	sessionsData := SessionFile{
		Sessions: []session.Session{
			{
				ID:        "test-1",
				Name:      "Test Session 1",
				Slug:      "test-session-1",
				Path:      "/path/to/session1",
				Remote:    "https://github.com/test/repo1",
				State:     session.StateActive,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			{
				ID:        "test-2",
				Name:      "Test Session 2",
				Slug:      "test-session-2",
				Path:      "/path/to/session2",
				Remote:    "https://github.com/test/repo2",
				State:     session.StateRecycled,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		},
	}

	data, err := json.Marshal(sessionsData)
	require.NoError(t, err, "Marshal sessions")
	require.NoError(t, os.WriteFile(sessionsPath, data, 0o644))

	// Open database and migrate
	database, err := db.Open(tempDir, db.DefaultOpenOptions())
	require.NoError(t, err, "Open")
	defer func() { _ = database.Close() }()

	require.NoError(t, MigrateFromJSON(ctx, database, tempDir))

	// Verify sessions were migrated
	store := NewSessionStore(database)
	sessions, err := store.List(ctx)
	require.NoError(t, err, "List sessions")
	require.Len(t, sessions, 2, "Expected 2 sessions, got %d", len(sessions))

	// Verify session data
	sess1, err := store.Get(ctx, "test-1")
	require.NoError(t, err, "Get test-1")
	assert.Equal(t, "Test Session 1", sess1.Name)
	assert.Equal(t, session.StateActive, sess1.State)
}

func TestMigrateFromJSON_Messages(t *testing.T) {
	tempDir := t.TempDir()
	ctx := context.Background()

	// Create empty sessions.json to trigger migration
	sessionsPath := filepath.Join(tempDir, "sessions.json")
	sessionsData := SessionFile{Sessions: []session.Session{}}
	data, _ := json.Marshal(sessionsData)
	require.NoError(t, os.WriteFile(sessionsPath, data, 0o644))

	// Create topics directory and message files
	topicsDir := filepath.Join(tempDir, "messages", "topics")
	require.NoError(t, os.MkdirAll(topicsDir, 0o755))

	// Create first topic file
	topic1 := TopicFile{
		Topic: "agent.build",
		Messages: []messaging.Message{
			{
				ID:        "msg-1",
				Topic:     "agent.build",
				Payload:   "build started",
				Sender:    "agent-1",
				CreatedAt: time.Now().Add(-10 * time.Minute),
			},
			{
				ID:        "msg-2",
				Topic:     "agent.build",
				Payload:   "build complete",
				Sender:    "agent-1",
				CreatedAt: time.Now(),
			},
		},
	}

	data1, _ := json.Marshal(topic1)
	require.NoError(t, os.WriteFile(filepath.Join(topicsDir, "agent.build.json"), data1, 0o644))

	// Create second topic file
	topic2 := TopicFile{
		Topic: "agent.test",
		Messages: []messaging.Message{
			{
				ID:        "msg-3",
				Topic:     "agent.test",
				Payload:   "tests running",
				Sender:    "agent-2",
				CreatedAt: time.Now(),
			},
		},
	}

	data2, _ := json.Marshal(topic2)
	require.NoError(t, os.WriteFile(filepath.Join(topicsDir, "agent.test.json"), data2, 0o644))

	// Open database and migrate
	database, err := db.Open(tempDir, db.DefaultOpenOptions())
	require.NoError(t, err, "Open")
	defer func() { _ = database.Close() }()

	require.NoError(t, MigrateFromJSON(ctx, database, tempDir))

	// Verify messages were migrated
	msgStore := NewMessageStore(database, 0)

	// Check first topic
	messages1, err := msgStore.Subscribe(ctx, "agent.build", time.Time{})
	require.NoError(t, err, "Subscribe agent.build")
	assert.Len(t, messages1, 2, "Expected 2 messages in agent.build, got %d", len(messages1))

	// Check second topic
	messages2, err := msgStore.Subscribe(ctx, "agent.test", time.Time{})
	require.NoError(t, err, "Subscribe agent.test")
	assert.Len(t, messages2, 1, "Expected 1 message in agent.test, got %d", len(messages2))

	// Verify message content
	assert.Equal(t, "build started", messages1[0].Payload)
}

func TestMigrateFromJSON_SkipIfPopulated(t *testing.T) {
	tempDir := t.TempDir()
	ctx := context.Background()

	// Create sessions.json
	sessionsPath := filepath.Join(tempDir, "sessions.json")
	sessionsData := SessionFile{
		Sessions: []session.Session{
			{
				ID:        "test-1",
				Name:      "Test Session",
				State:     session.StateActive,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		},
	}

	data, _ := json.Marshal(sessionsData)
	require.NoError(t, os.WriteFile(sessionsPath, data, 0o644))

	// Open database and add a session directly
	database, err := db.Open(tempDir, db.DefaultOpenOptions())
	require.NoError(t, err, "Open")
	defer func() { _ = database.Close() }()

	store := NewSessionStore(database)
	existingSession := session.Session{
		ID:        "existing",
		Name:      "Existing Session",
		State:     session.StateActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, store.Save(ctx, existingSession), "Save existing session")

	// Attempt migration - should skip
	require.NoError(t, MigrateFromJSON(ctx, database, tempDir))

	// Verify only the existing session is present
	sessions, _ := store.List(ctx)
	assert.Len(t, sessions, 1, "Expected 1 session (migration skipped), got %d", len(sessions))
	assert.Equal(t, "existing", sessions[0].ID)
}

func TestMigrateFromJSON_Combined(t *testing.T) {
	tempDir := t.TempDir()
	ctx := context.Background()

	// Create sessions.json
	sessionsPath := filepath.Join(tempDir, "sessions.json")
	sessionsData := SessionFile{
		Sessions: []session.Session{
			{
				ID:        "sess-1",
				Name:      "Session 1",
				State:     session.StateActive,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		},
	}
	sessData, _ := json.Marshal(sessionsData)
	require.NoError(t, os.WriteFile(sessionsPath, sessData, 0o644))

	// Create message topic
	topicsDir := filepath.Join(tempDir, "messages", "topics")
	require.NoError(t, os.MkdirAll(topicsDir, 0o755))

	topic := TopicFile{
		Topic: "test.topic",
		Messages: []messaging.Message{
			{
				ID:        "msg-1",
				Topic:     "test.topic",
				Payload:   "test message",
				CreatedAt: time.Now(),
			},
		},
	}
	topicData, _ := json.Marshal(topic)
	require.NoError(t, os.WriteFile(filepath.Join(topicsDir, "test.topic.json"), topicData, 0o644))

	// Migrate
	database, err := db.Open(tempDir, db.DefaultOpenOptions())
	require.NoError(t, err, "Open")
	defer func() { _ = database.Close() }()

	require.NoError(t, MigrateFromJSON(ctx, database, tempDir))

	// Verify both sessions and messages were migrated
	sessionStore := NewSessionStore(database)
	sessions, _ := sessionStore.List(ctx)
	assert.Len(t, sessions, 1, "Expected 1 session, got %d", len(sessions))

	msgStore := NewMessageStore(database, 0)
	messages, _ := msgStore.Subscribe(ctx, "test.topic", time.Time{})
	assert.Len(t, messages, 1, "Expected 1 message, got %d", len(messages))
}

func TestMigrateFromJSON_InvalidJSON(t *testing.T) {
	tempDir := t.TempDir()
	ctx := context.Background()

	// Create invalid sessions.json
	sessionsPath := filepath.Join(tempDir, "sessions.json")
	require.NoError(t, os.WriteFile(sessionsPath, []byte("invalid json {"), 0o644))

	database, err := db.Open(tempDir, db.DefaultOpenOptions())
	require.NoError(t, err, "Open")
	defer func() { _ = database.Close() }()

	// Migration should fail
	assert.Error(t, MigrateFromJSON(ctx, database, tempDir), "Expected migration to fail with invalid JSON")
}
