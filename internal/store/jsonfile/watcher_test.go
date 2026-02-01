package jsonfile

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hay-kot/hive/internal/core/messaging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTopicWatcher_Watch(t *testing.T) {
	t.Parallel()

	topicsDir := t.TempDir()

	watcher, err := NewTopicWatcher(topicsDir)
	require.NoError(t, err)
	defer watcher.Close() //nolint:errcheck

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	events, err := watcher.Watch(ctx, "test-topic")
	require.NoError(t, err)

	// Write a topic file
	topicPath := filepath.Join(topicsDir, "test-topic.json")
	err = os.WriteFile(topicPath, []byte(`{"name":"test-topic","messages":[]}`), 0o644)
	require.NoError(t, err)

	// Wait for event with timeout
	select {
	case event := <-events:
		assert.Equal(t, "test-topic", event.Topic)
		assert.False(t, event.Timestamp.IsZero())
	case <-ctx.Done():
		t.Fatal("timeout waiting for event")
	}
}

func TestTopicWatcher_WatchWildcard(t *testing.T) {
	t.Parallel()

	topicsDir := t.TempDir()

	watcher, err := NewTopicWatcher(topicsDir)
	require.NoError(t, err)
	defer watcher.Close() //nolint:errcheck

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	events, err := watcher.Watch(ctx, "*")
	require.NoError(t, err)

	// Write two different topics
	err = os.WriteFile(filepath.Join(topicsDir, "topic-a.json"), []byte(`{}`), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(topicsDir, "topic-b.json"), []byte(`{}`), 0o644)
	require.NoError(t, err)

	// Should receive both events
	received := make(map[string]bool)
	timeout := time.After(5 * time.Second)
	for len(received) < 2 {
		select {
		case event := <-events:
			received[event.Topic] = true
		case <-timeout:
			t.Fatal("timeout waiting for events")
		}
	}

	assert.True(t, received["topic-a"])
	assert.True(t, received["topic-b"])
}

func TestTopicWatcher_WatchPrefixPattern(t *testing.T) {
	t.Parallel()

	topicsDir := t.TempDir()

	watcher, err := NewTopicWatcher(topicsDir)
	require.NoError(t, err)
	defer watcher.Close() //nolint:errcheck

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	events, err := watcher.Watch(ctx, "hive.*")
	require.NoError(t, err)

	// Write matching and non-matching topics
	err = os.WriteFile(filepath.Join(topicsDir, "hive.test.json"), []byte(`{}`), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(topicsDir, "other.topic.json"), []byte(`{}`), 0o644)
	require.NoError(t, err)

	// Should only receive the matching event
	timeout := time.After(200 * time.Millisecond)
	var receivedTopics []string
	for {
		select {
		case event := <-events:
			receivedTopics = append(receivedTopics, event.Topic)
		case <-timeout:
			// Done collecting events
			assert.Equal(t, []string{"hive.test"}, receivedTopics)
			return
		}
	}
}

func TestTopicWatcher_IgnoreTmpAndLockFiles(t *testing.T) {
	t.Parallel()

	topicsDir := t.TempDir()

	watcher, err := NewTopicWatcher(topicsDir)
	require.NoError(t, err)
	defer watcher.Close() //nolint:errcheck

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	events, err := watcher.Watch(ctx, "*")
	require.NoError(t, err)

	// Write tmp and lock files (should be ignored)
	err = os.WriteFile(filepath.Join(topicsDir, "test.json.tmp"), []byte(`{}`), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(topicsDir, "test.json.lock"), []byte(`{}`), 0o644)
	require.NoError(t, err)

	// Then write a real topic file
	time.Sleep(100 * time.Millisecond) // Ensure tmp/lock events processed first
	err = os.WriteFile(filepath.Join(topicsDir, "real.json"), []byte(`{}`), 0o644)
	require.NoError(t, err)

	// Should only receive the real topic event
	timeout := time.After(300 * time.Millisecond)
	var receivedTopics []string
	for {
		select {
		case event := <-events:
			receivedTopics = append(receivedTopics, event.Topic)
		case <-timeout:
			assert.Equal(t, []string{"real"}, receivedTopics)
			return
		}
	}
}

func TestTopicWatcher_Debounce(t *testing.T) {
	t.Parallel()

	topicsDir := t.TempDir()

	watcher, err := NewTopicWatcher(topicsDir)
	require.NoError(t, err)
	defer watcher.Close() //nolint:errcheck

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	events, err := watcher.Watch(ctx, "*")
	require.NoError(t, err)

	topicPath := filepath.Join(topicsDir, "debounce-test.json")

	// Rapidly write to the same file multiple times
	for i := 0; i < 5; i++ {
		err = os.WriteFile(topicPath, []byte(`{}`), 0o644)
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond) // Less than debounce delay
	}

	// Should only receive one event due to debouncing
	timeout := time.After(300 * time.Millisecond)
	eventCount := 0
	for {
		select {
		case <-events:
			eventCount++
		case <-timeout:
			assert.Equal(t, 1, eventCount, "should receive exactly one debounced event")
			return
		}
	}
}

func TestTopicWatcher_ContextCancellation(t *testing.T) {
	t.Parallel()

	topicsDir := t.TempDir()

	watcher, err := NewTopicWatcher(topicsDir)
	require.NoError(t, err)
	defer watcher.Close() //nolint:errcheck

	ctx, cancel := context.WithCancel(context.Background())
	events, err := watcher.Watch(ctx, "*")
	require.NoError(t, err)

	// Cancel the context
	cancel()

	// Channel should be closed
	time.Sleep(100 * time.Millisecond) // Give time for cleanup goroutine
	_, ok := <-events
	assert.False(t, ok, "channel should be closed after context cancellation")
}

func TestTopicWatcher_Close(t *testing.T) {
	t.Parallel()

	topicsDir := t.TempDir()

	watcher, err := NewTopicWatcher(topicsDir)
	require.NoError(t, err)

	ctx := context.Background()
	events, err := watcher.Watch(ctx, "*")
	require.NoError(t, err)

	err = watcher.Close()
	require.NoError(t, err)

	// Channel should be closed
	_, ok := <-events
	assert.False(t, ok, "channel should be closed after watcher close")
}

func TestTopicWatcher_TopicNameWithSlash(t *testing.T) {
	t.Parallel()

	topicsDir := t.TempDir()

	watcher, err := NewTopicWatcher(topicsDir)
	require.NoError(t, err)
	defer watcher.Close() //nolint:errcheck

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	events, err := watcher.Watch(ctx, "agents/test")
	require.NoError(t, err)

	// Topic names with "/" are stored as "_" in filename
	topicPath := filepath.Join(topicsDir, "agents_test.json")
	err = os.WriteFile(topicPath, []byte(`{}`), 0o644)
	require.NoError(t, err)

	select {
	case event := <-events:
		assert.Equal(t, "agents/test", event.Topic)
	case <-ctx.Done():
		t.Fatal("timeout waiting for event")
	}
}

func TestMatchesPattern(t *testing.T) {
	tests := []struct {
		pattern string
		topic   string
		want    bool
	}{
		// All patterns
		{"*", "anything", true},
		{"", "anything", true},

		// Prefix patterns
		{"hive.*", "hive.test", true},
		{"hive.*", "hive.agent.foo", true},
		{"hive.*", "other", false},
		{"agents.*", "agents.foo", true},
		{"agents.*", "agents/foo", false}, // / is different from .

		// Exact match
		{"test-topic", "test-topic", true},
		{"test-topic", "other-topic", false},
		{"test-topic", "test-topic-extra", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.topic, func(t *testing.T) {
			got := matchesPattern(tt.pattern, tt.topic)
			assert.Equal(t, tt.want, got)
		})
	}
}

var _ messaging.Watcher = (*TopicWatcher)(nil)
