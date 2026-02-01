package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/hay-kot/hive/internal/core/config"
	"github.com/hay-kot/hive/internal/core/messaging"
	"github.com/hay-kot/hive/internal/core/session"
	"github.com/hay-kot/hive/internal/store/jsonfile"
	"github.com/urfave/cli/v3"
)

func TestRunTopic_DefaultPrefix(t *testing.T) {
	var buf bytes.Buffer

	flags := &Flags{
		Config: &config.Config{
			Messaging: config.MessagingConfig{
				TopicPrefix: "agent",
			},
		},
	}

	cmd := NewMsgCmd(flags)

	app := &cli.Command{
		Name:   "hive",
		Writer: &buf,
	}
	cmd.Register(app)

	err := app.Run(context.Background(), []string{"hive", "msg", "topic"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	// Remove trailing newline
	if len(output) > 0 && output[len(output)-1] == '\n' {
		output = output[:len(output)-1]
	}

	// Should be "agent.XXXX" format
	pattern := regexp.MustCompile(`^agent\.[a-z0-9]{4}$`)
	if !pattern.MatchString(output) {
		t.Errorf("output %q does not match expected pattern agent.[a-z0-9]{4}", output)
	}
}

func TestRunTopic_CustomPrefixFlag(t *testing.T) {
	var buf bytes.Buffer

	flags := &Flags{
		Config: &config.Config{
			Messaging: config.MessagingConfig{
				TopicPrefix: "agent",
			},
		},
	}

	cmd := NewMsgCmd(flags)

	app := &cli.Command{
		Name:   "hive",
		Writer: &buf,
	}
	cmd.Register(app)

	err := app.Run(context.Background(), []string{"hive", "msg", "topic", "--prefix", "task"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if len(output) > 0 && output[len(output)-1] == '\n' {
		output = output[:len(output)-1]
	}

	// Should be "task.XXXX" format (flag overrides config)
	pattern := regexp.MustCompile(`^task\.[a-z0-9]{4}$`)
	if !pattern.MatchString(output) {
		t.Errorf("output %q does not match expected pattern task.[a-z0-9]{4}", output)
	}
}

func TestRunTopic_EmptyPrefixFlag(t *testing.T) {
	var buf bytes.Buffer

	flags := &Flags{
		Config: &config.Config{
			Messaging: config.MessagingConfig{
				TopicPrefix: "agent",
			},
		},
	}

	cmd := NewMsgCmd(flags)

	app := &cli.Command{
		Name:   "hive",
		Writer: &buf,
	}
	cmd.Register(app)

	err := app.Run(context.Background(), []string{"hive", "msg", "topic", "--prefix", ""})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if len(output) > 0 && output[len(output)-1] == '\n' {
		output = output[:len(output)-1]
	}

	// Should be just "XXXX" format (no prefix, no dot)
	pattern := regexp.MustCompile(`^[a-z0-9]{4}$`)
	if !pattern.MatchString(output) {
		t.Errorf("output %q does not match expected pattern [a-z0-9]{4}", output)
	}

	// Verify no dot present
	if bytes.Contains([]byte(output), []byte(".")) {
		t.Errorf("output %q should not contain a dot when prefix is empty", output)
	}
}

func TestRunTopic_EmptyConfigPrefix(t *testing.T) {
	var buf bytes.Buffer

	flags := &Flags{
		Config: &config.Config{
			Messaging: config.MessagingConfig{
				TopicPrefix: "", // Empty config prefix
			},
		},
	}

	cmd := NewMsgCmd(flags)

	app := &cli.Command{
		Name:   "hive",
		Writer: &buf,
	}
	cmd.Register(app)

	err := app.Run(context.Background(), []string{"hive", "msg", "topic"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if len(output) > 0 && output[len(output)-1] == '\n' {
		output = output[:len(output)-1]
	}

	// Should be just "XXXX" format (no prefix from config)
	pattern := regexp.MustCompile(`^[a-z0-9]{4}$`)
	if !pattern.MatchString(output) {
		t.Errorf("output %q does not match expected pattern [a-z0-9]{4}", output)
	}
}

func TestRunTopic_Uniqueness(t *testing.T) {
	// Generate multiple topic IDs and verify they're unique
	seen := make(map[string]bool)

	for range 10 {
		var buf bytes.Buffer

		flags := &Flags{
			Config: &config.Config{
				Messaging: config.MessagingConfig{
					TopicPrefix: "agent",
				},
			},
		}

		cmd := NewMsgCmd(flags)

		app := &cli.Command{
			Name:   "hive",
			Writer: &buf,
		}
		cmd.Register(app)

		err := app.Run(context.Background(), []string{"hive", "msg", "topic"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		output := buf.String()
		if len(output) > 0 && output[len(output)-1] == '\n' {
			output = output[:len(output)-1]
		}

		seen[output] = true
	}

	// Should have generated unique IDs (with 36^4 = 1.6M combinations, duplicates in 10 tries would be very rare)
	if len(seen) < 9 {
		t.Errorf("generated only %d unique topic IDs in 10 attempts, expected near 10", len(seen))
	}
}

func TestPeekFlag_DoesNotUpdateLastInboxRead(t *testing.T) {
	// Setup temporary data directory
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("failed to create data dir: %v", err)
	}

	// Create a session directory
	sessionDir := filepath.Join(tmpDir, "session")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("failed to create session dir: %v", err)
	}

	// Resolve symlinks (important on macOS where /var -> /private/var)
	sessionDir, err := filepath.EvalSymlinks(sessionDir)
	if err != nil {
		t.Fatalf("failed to resolve symlinks: %v", err)
	}

	// Change to session directory so detectSessionID can find it
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer os.Chdir(originalWd)
	if err := os.Chdir(sessionDir); err != nil {
		t.Fatalf("failed to change to session directory: %v", err)
	}

	// Create a session
	sessionsPath := filepath.Join(dataDir, "sessions.json")
	sessStore := jsonfile.New(sessionsPath)
	ctx := context.Background()

	sess := session.Session{
		ID:    "test123",
		Path:  sessionDir,
		State: session.StateActive,
	}
	if err := sessStore.Save(ctx, sess); err != nil {
		t.Fatalf("failed to save session: %v", err)
	}

	// Publish a message to the inbox
	msgStore := jsonfile.NewMsgStore(filepath.Join(dataDir, "messages", "topics"))
	inboxTopic := "agent.test123.inbox"
	msg := messaging.Message{
		Topic:     inboxTopic,
		Payload:   "test message",
		SessionID: "test123",
	}
	if err := msgStore.Publish(ctx, msg); err != nil {
		t.Fatalf("failed to publish message: %v", err)
	}

	// Wait a moment to ensure timestamp differences
	time.Sleep(10 * time.Millisecond)

	// Create command flags
	flags := &Flags{
		Config:  &config.Config{},
		DataDir: dataDir,
	}

	// Test 1: Subscribe with --new --peek (should NOT update LastInboxRead)
	{
		var buf bytes.Buffer
		cmd := NewMsgCmd(flags)
		app := &cli.Command{
			Name:   "hive",
			Writer: &buf,
		}
		cmd.Register(app)

		err := app.Run(ctx, []string{"hive", "msg", "sub", "-t", inboxTopic, "--new", "--peek"})
		if err != nil {
			t.Fatalf("unexpected error with --peek: %v", err)
		}

		// Verify message was returned
		var receivedMsg messaging.Message
		if err := json.Unmarshal(buf.Bytes(), &receivedMsg); err != nil {
			t.Fatalf("failed to parse message: %v", err)
		}
		if receivedMsg.Payload != "test message" {
			t.Errorf("expected payload 'test message', got %q", receivedMsg.Payload)
		}

		// Verify LastInboxRead was NOT updated (should still be nil)
		sess, err := sessStore.Get(ctx, "test123")
		if err != nil {
			t.Fatalf("failed to get session: %v", err)
		}
		if sess.LastInboxRead != nil {
			t.Errorf("LastInboxRead should be nil after --peek, got %v", sess.LastInboxRead)
		}
	}

	// Test 2: Subscribe with --new (should update LastInboxRead)
	{
		var buf bytes.Buffer
		cmd := NewMsgCmd(flags)
		app := &cli.Command{
			Name:   "hive",
			Writer: &buf,
		}
		cmd.Register(app)

		beforeTime := time.Now()
		err := app.Run(ctx, []string{"hive", "msg", "sub", "-t", inboxTopic, "--new"})
		if err != nil {
			t.Fatalf("unexpected error without --peek: %v", err)
		}

		// Verify LastInboxRead WAS updated
		sess, err := sessStore.Get(ctx, "test123")
		if err != nil {
			t.Fatalf("failed to get session: %v", err)
		}
		if sess.LastInboxRead == nil {
			t.Errorf("LastInboxRead should not be nil after --new without --peek")
		} else if sess.LastInboxRead.Before(beforeTime) {
			t.Errorf("LastInboxRead should be updated to recent time, got %v", sess.LastInboxRead)
		}
	}

	// Test 3: Subscribe with --new again (should return no messages since LastInboxRead was updated)
	{
		var buf bytes.Buffer
		cmd := NewMsgCmd(flags)
		app := &cli.Command{
			Name:   "hive",
			Writer: &buf,
		}
		cmd.Register(app)

		err := app.Run(ctx, []string{"hive", "msg", "sub", "-t", inboxTopic, "--new"})
		if err != nil {
			t.Fatalf("unexpected error on second --new: %v", err)
		}

		// Should have no output (no new messages)
		if buf.Len() > 0 {
			t.Errorf("expected no messages on second --new call, got output: %s", buf.String())
		}
	}
}
