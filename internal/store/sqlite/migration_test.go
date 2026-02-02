package sqlite

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hay-kot/hive/internal/core/messaging"
	"github.com/hay-kot/hive/internal/core/session"
)

func TestMigrateFromJSON_NoFiles(t *testing.T) {
	tempDir := t.TempDir()
	db, err := Open(tempDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	ctx := context.Background()

	// Migrate with no JSON files - should succeed silently
	err = MigrateFromJSON(ctx, db, tempDir)
	if err != nil {
		t.Errorf("MigrateFromJSON failed: %v", err)
	}

	// Verify no sessions were created
	store := NewSessionStore(db)
	sessions, _ := store.List(ctx)
	if len(sessions) != 0 {
		t.Errorf("Expected 0 sessions, got %d", len(sessions))
	}
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
	if err != nil {
		t.Fatalf("Marshal sessions: %v", err)
	}
	if err := os.WriteFile(sessionsPath, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Open database and migrate
	db, err := Open(tempDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	err = MigrateFromJSON(ctx, db, tempDir)
	if err != nil {
		t.Fatalf("MigrateFromJSON: %v", err)
	}

	// Verify sessions were migrated
	store := NewSessionStore(db)
	sessions, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List sessions: %v", err)
	}

	if len(sessions) != 2 {
		t.Fatalf("Expected 2 sessions, got %d", len(sessions))
	}

	// Verify session data
	sess1, err := store.Get(ctx, "test-1")
	if err != nil {
		t.Fatalf("Get test-1: %v", err)
	}
	if sess1.Name != "Test Session 1" {
		t.Errorf("Session name = %q, want %q", sess1.Name, "Test Session 1")
	}
	if sess1.State != session.StateActive {
		t.Errorf("Session state = %v, want %v", sess1.State, session.StateActive)
	}
}

func TestMigrateFromJSON_Messages(t *testing.T) {
	tempDir := t.TempDir()
	ctx := context.Background()

	// Create empty sessions.json to trigger migration
	sessionsPath := filepath.Join(tempDir, "sessions.json")
	sessionsData := SessionFile{Sessions: []session.Session{}}
	data, _ := json.Marshal(sessionsData)
	if err := os.WriteFile(sessionsPath, data, 0o644); err != nil {
		t.Fatalf("WriteFile sessions: %v", err)
	}

	// Create topics directory and message files
	topicsDir := filepath.Join(tempDir, "messages", "topics")
	if err := os.MkdirAll(topicsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

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
	if err := os.WriteFile(filepath.Join(topicsDir, "agent.build.json"), data1, 0o644); err != nil {
		t.Fatalf("WriteFile topic1: %v", err)
	}

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
	if err := os.WriteFile(filepath.Join(topicsDir, "agent.test.json"), data2, 0o644); err != nil {
		t.Fatalf("WriteFile topic2: %v", err)
	}

	// Open database and migrate
	db, err := Open(tempDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	err = MigrateFromJSON(ctx, db, tempDir)
	if err != nil {
		t.Fatalf("MigrateFromJSON: %v", err)
	}

	// Verify messages were migrated
	msgStore := NewMessageStore(db, 0)

	// Check first topic
	messages1, err := msgStore.Subscribe(ctx, "agent.build", time.Time{})
	if err != nil {
		t.Fatalf("Subscribe agent.build: %v", err)
	}
	if len(messages1) != 2 {
		t.Errorf("Expected 2 messages in agent.build, got %d", len(messages1))
	}

	// Check second topic
	messages2, err := msgStore.Subscribe(ctx, "agent.test", time.Time{})
	if err != nil {
		t.Fatalf("Subscribe agent.test: %v", err)
	}
	if len(messages2) != 1 {
		t.Errorf("Expected 1 message in agent.test, got %d", len(messages2))
	}

	// Verify message content
	if messages1[0].Payload != "build started" {
		t.Errorf("Message payload = %q, want %q", messages1[0].Payload, "build started")
	}
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
	if err := os.WriteFile(sessionsPath, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Open database and add a session directly
	db, err := Open(tempDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	store := NewSessionStore(db)
	existingSession := session.Session{
		ID:        "existing",
		Name:      "Existing Session",
		State:     session.StateActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := store.Save(ctx, existingSession); err != nil {
		t.Fatalf("Save existing session: %v", err)
	}

	// Attempt migration - should skip
	err = MigrateFromJSON(ctx, db, tempDir)
	if err != nil {
		t.Fatalf("MigrateFromJSON: %v", err)
	}

	// Verify only the existing session is present
	sessions, _ := store.List(ctx)
	if len(sessions) != 1 {
		t.Errorf("Expected 1 session (migration skipped), got %d", len(sessions))
	}
	if sessions[0].ID != "existing" {
		t.Errorf("Session ID = %q, want %q", sessions[0].ID, "existing")
	}
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
	if err := os.WriteFile(sessionsPath, sessData, 0o644); err != nil {
		t.Fatalf("WriteFile sessions: %v", err)
	}

	// Create message topic
	topicsDir := filepath.Join(tempDir, "messages", "topics")
	if err := os.MkdirAll(topicsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

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
	if err := os.WriteFile(filepath.Join(topicsDir, "test.topic.json"), topicData, 0o644); err != nil {
		t.Fatalf("WriteFile topic: %v", err)
	}

	// Migrate
	db, err := Open(tempDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	err = MigrateFromJSON(ctx, db, tempDir)
	if err != nil {
		t.Fatalf("MigrateFromJSON: %v", err)
	}

	// Verify both sessions and messages were migrated
	sessionStore := NewSessionStore(db)
	sessions, _ := sessionStore.List(ctx)
	if len(sessions) != 1 {
		t.Errorf("Expected 1 session, got %d", len(sessions))
	}

	msgStore := NewMessageStore(db, 0)
	messages, _ := msgStore.Subscribe(ctx, "test.topic", time.Time{})
	if len(messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(messages))
	}
}

func TestMigrateFromJSON_InvalidJSON(t *testing.T) {
	tempDir := t.TempDir()
	ctx := context.Background()

	// Create invalid sessions.json
	sessionsPath := filepath.Join(tempDir, "sessions.json")
	if err := os.WriteFile(sessionsPath, []byte("invalid json {"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	db, err := Open(tempDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Migration should fail
	err = MigrateFromJSON(ctx, db, tempDir)
	if err == nil {
		t.Error("Expected migration to fail with invalid JSON")
	}
}
