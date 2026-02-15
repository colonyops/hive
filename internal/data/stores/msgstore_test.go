package stores

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/messaging"
	"github.com/colonyops/hive/internal/data/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMsgStore_PublishAndSubscribe(t *testing.T) {
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err, "Open")
	defer func() { _ = database.Close() }()

	store := NewMessageStore(database, 0)
	ctx := context.Background()

	msg := messaging.Message{
		Topic:   "test.topic",
		Payload: "hello world",
		Sender:  "test-sender",
	}

	err = store.Publish(ctx, msg, []string{"test.topic"})
	require.NoError(t, err, "Publish failed")

	messages, err := store.Subscribe(ctx, "test.topic", time.Time{})
	require.NoError(t, err, "Subscribe failed")
	require.Len(t, messages, 1, "Subscribe returned %d messages, want 1", len(messages))
	assert.Equal(t, "hello world", messages[0].Payload)
	assert.Equal(t, "test-sender", messages[0].Sender)
	assert.NotEmpty(t, messages[0].ID, "ID should be auto-generated")
	assert.False(t, messages[0].CreatedAt.IsZero(), "CreatedAt should be auto-set")
}

func TestMsgStore_SubscribeNotFound(t *testing.T) {
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err, "Open")
	defer func() { _ = database.Close() }()

	store := NewMessageStore(database, 0)
	ctx := context.Background()

	_, err = store.Subscribe(ctx, "nonexistent", time.Time{})
	assert.ErrorIs(t, err, messaging.ErrTopicNotFound, "Subscribe error = %v, want ErrTopicNotFound", err)
}

func TestMsgStore_SubscribeSince(t *testing.T) {
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err, "Open")
	defer func() { _ = database.Close() }()

	store := NewMessageStore(database, 0)
	ctx := context.Background()

	// Publish first message
	_ = store.Publish(ctx, messaging.Message{Topic: "events", Payload: "first"}, []string{"events"})
	time.Sleep(10 * time.Millisecond)

	// Record time between messages
	midpoint := time.Now()
	time.Sleep(10 * time.Millisecond)

	// Publish second message
	_ = store.Publish(ctx, messaging.Message{Topic: "events", Payload: "second"}, []string{"events"})

	// Subscribe since midpoint
	messages, err := store.Subscribe(ctx, "events", midpoint)
	require.NoError(t, err, "Subscribe failed")
	require.Len(t, messages, 1, "Subscribe returned %d messages, want 1", len(messages))
	assert.Equal(t, "second", messages[0].Payload)
}

func TestMsgStore_SubscribeWildcard(t *testing.T) {
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err, "Open")
	defer func() { _ = database.Close() }()

	store := NewMessageStore(database, 0)
	ctx := context.Background()

	// Publish to multiple topics
	_ = store.Publish(ctx, messaging.Message{Topic: "agent.build", Payload: "build started"}, []string{"agent.build"})
	_ = store.Publish(ctx, messaging.Message{Topic: "agent.test", Payload: "tests running"}, []string{"agent.test"})
	_ = store.Publish(ctx, messaging.Message{Topic: "other.topic", Payload: "unrelated"}, []string{"other.topic"})

	// Subscribe with wildcard
	messages, err := store.Subscribe(ctx, "agent.*", time.Time{})
	require.NoError(t, err, "Subscribe failed")
	require.Len(t, messages, 2, "Subscribe returned %d messages, want 2", len(messages))

	payloads := make(map[string]bool)
	for _, m := range messages {
		payloads[m.Payload] = true
	}
	assert.True(t, payloads["build started"], "Missing expected payloads in %v", messages)
	assert.True(t, payloads["tests running"], "Missing expected payloads in %v", messages)
}

func TestMsgStore_SubscribeAll(t *testing.T) {
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err, "Open")
	defer func() { _ = database.Close() }()

	store := NewMessageStore(database, 0)
	ctx := context.Background()

	// Publish to multiple topics
	_ = store.Publish(ctx, messaging.Message{Topic: "topic1", Payload: "msg1"}, []string{"topic1"})
	_ = store.Publish(ctx, messaging.Message{Topic: "topic2", Payload: "msg2"}, []string{"topic2"})

	// Subscribe to all with empty pattern
	messages, err := store.Subscribe(ctx, "", time.Time{})
	require.NoError(t, err, "Subscribe failed")
	require.Len(t, messages, 2, "Subscribe returned %d messages, want 2", len(messages))

	// Subscribe to all with "*" pattern
	messages, err = store.Subscribe(ctx, "*", time.Time{})
	require.NoError(t, err, "Subscribe with * failed")
	require.Len(t, messages, 2, "Subscribe with * returned %d messages, want 2", len(messages))
}

func TestMsgStore_List(t *testing.T) {
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err, "Open")
	defer func() { _ = database.Close() }()

	store := NewMessageStore(database, 0)
	ctx := context.Background()

	_ = store.Publish(ctx, messaging.Message{Topic: "topic.a", Payload: "a"}, []string{"topic.a"})
	_ = store.Publish(ctx, messaging.Message{Topic: "topic.b", Payload: "b"}, []string{"topic.b"})

	topics, err := store.List(ctx)
	require.NoError(t, err, "List failed")
	require.Len(t, topics, 2, "List returned %d topics, want 2", len(topics))

	// Topics should be sorted
	assert.Equal(t, []string{"topic.a", "topic.b"}, topics)
}

func TestMsgStore_ListEmpty(t *testing.T) {
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err, "Open")
	defer func() { _ = database.Close() }()

	store := NewMessageStore(database, 0)
	ctx := context.Background()

	topics, err := store.List(ctx)
	require.NoError(t, err, "List failed")
	assert.Empty(t, topics, "List returned %v, want empty", topics)
}

func TestMsgStore_Retention(t *testing.T) {
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err, "Open")
	defer func() { _ = database.Close() }()

	store := NewMessageStore(database, 3)
	ctx := context.Background()

	// Publish 5 messages
	for i := range 5 {
		err := store.Publish(ctx, messaging.Message{
			Topic:   "test",
			Payload: fmt.Sprintf("msg%d", i),
		}, []string{"test"})
		require.NoError(t, err, "Publish %d failed", i)
	}

	messages, err := store.Subscribe(ctx, "test", time.Time{})
	require.NoError(t, err, "Subscribe failed")
	require.Len(t, messages, 3, "Subscribe returned %d messages, want 3", len(messages))
	assert.Equal(t, "msg2", messages[0].Payload)
	assert.Equal(t, "msg4", messages[2].Payload)
}

func TestMsgStore_RetentionBoundaries(t *testing.T) {
	ctx := context.Background()

	t.Run("exact limit", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err, "Open")
		defer func() { _ = database.Close() }()

		store := NewMessageStore(database, 3)

		// Publish exactly 3 messages
		for i := range 3 {
			err := store.Publish(ctx, messaging.Message{
				Topic:   "test",
				Payload: fmt.Sprintf("msg%d", i),
			}, []string{"test"})
			require.NoError(t, err, "Publish %d failed", i)
		}

		messages, err := store.Subscribe(ctx, "test", time.Time{})
		require.NoError(t, err, "Subscribe failed")
		assert.Len(t, messages, 3, "Expected 3 messages at exact limit, got %d", len(messages))
	})

	t.Run("single message limit", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err, "Open")
		defer func() { _ = database.Close() }()

		store := NewMessageStore(database, 1)

		// Publish 3 messages
		for i := range 3 {
			err := store.Publish(ctx, messaging.Message{
				Topic:   "test",
				Payload: fmt.Sprintf("msg%d", i),
			}, []string{"test"})
			require.NoError(t, err, "Publish %d failed", i)
		}

		messages, err := store.Subscribe(ctx, "test", time.Time{})
		require.NoError(t, err, "Subscribe failed")
		require.Len(t, messages, 1, "Expected 1 message with maxMessages=1, got %d", len(messages))
		assert.Equal(t, "msg2", messages[0].Payload, "Expected last message")
	})

	t.Run("unlimited retention", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err, "Open")
		defer func() { _ = database.Close() }()

		store := NewMessageStore(database, 0)

		// Publish 100 messages
		for i := range 100 {
			err := store.Publish(ctx, messaging.Message{
				Topic:   "test",
				Payload: fmt.Sprintf("msg%d", i),
			}, []string{"test"})
			require.NoError(t, err, "Publish %d failed", i)
		}

		messages, err := store.Subscribe(ctx, "test", time.Time{})
		require.NoError(t, err, "Subscribe failed")
		assert.Len(t, messages, 100, "Expected 100 messages with unlimited retention, got %d", len(messages))
	})
}

func TestMsgStore_Prune(t *testing.T) {
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err, "Open")
	defer func() { _ = database.Close() }()

	store := NewMessageStore(database, 0)
	ctx := context.Background()

	// Publish messages
	_ = store.Publish(ctx, messaging.Message{Topic: "events", Payload: "old"}, []string{"events"})
	time.Sleep(50 * time.Millisecond)
	_ = store.Publish(ctx, messaging.Message{Topic: "events", Payload: "new"}, []string{"events"})

	// Prune messages older than 25ms
	removed, err := store.Prune(ctx, 25*time.Millisecond)
	require.NoError(t, err, "Prune failed")
	assert.Equal(t, 1, removed, "Prune removed %d messages, want 1", removed)

	messages, _ := store.Subscribe(ctx, "events", time.Time{})
	require.Len(t, messages, 1, "Subscribe returned %d messages after prune, want 1", len(messages))
	assert.Equal(t, "new", messages[0].Payload)
}

func TestMsgStore_MessageOrdering(t *testing.T) {
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err, "Open")
	defer func() { _ = database.Close() }()

	store := NewMessageStore(database, 0)
	ctx := context.Background()

	// Publish messages with slight delays to ensure different timestamps
	for i := range 5 {
		_ = store.Publish(ctx, messaging.Message{
			Topic:   "ordered",
			Payload: fmt.Sprintf("msg%d", i),
		}, []string{"ordered"})
		time.Sleep(time.Millisecond)
	}

	messages, _ := store.Subscribe(ctx, "ordered", time.Time{})

	// Verify messages are in chronological order
	for i := 0; i < len(messages)-1; i++ {
		assert.True(t, messages[i].CreatedAt.Before(messages[i+1].CreatedAt), "Messages not in order: %v >= %v", messages[i].CreatedAt, messages[i+1].CreatedAt)
	}
}

func TestMsgStore_WildcardOrdering(t *testing.T) {
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err, "Open")
	defer func() { _ = database.Close() }()

	store := NewMessageStore(database, 0)
	ctx := context.Background()

	// Publish messages across topics with explicit ordering via delays
	_ = store.Publish(ctx, messaging.Message{Topic: "ns.a", Payload: "first"}, []string{"ns.a"})
	time.Sleep(5 * time.Millisecond)
	_ = store.Publish(ctx, messaging.Message{Topic: "ns.b", Payload: "second"}, []string{"ns.b"})
	time.Sleep(5 * time.Millisecond)
	_ = store.Publish(ctx, messaging.Message{Topic: "ns.a", Payload: "third"}, []string{"ns.a"})

	messages, _ := store.Subscribe(ctx, "ns.*", time.Time{})

	require.Len(t, messages, 3, "Subscribe returned %d messages, want 3", len(messages))

	// Should be chronologically sorted across all topics
	expected := []string{"first", "second", "third"}
	for i, msg := range messages {
		assert.Equal(t, expected[i], msg.Payload, "Message %d payload = %q, want %q", i, msg.Payload, expected[i])
	}
}

func TestMsgStore_Acknowledge(t *testing.T) {
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err, "Open")
	defer func() { _ = database.Close() }()

	store := NewMessageStore(database, 0)
	ctx := context.Background()

	// Publish messages
	msg1 := messaging.Message{Topic: "test.topic", Payload: "msg1"}
	msg2 := messaging.Message{Topic: "test.topic", Payload: "msg2"}
	_ = store.Publish(ctx, msg1, []string{"test.topic"})
	_ = store.Publish(ctx, msg2, []string{"test.topic"})

	// Get messages to retrieve their IDs
	messages, _ := store.Subscribe(ctx, "test.topic", time.Time{})
	require.Len(t, messages, 2, "Expected 2 messages, got %d", len(messages))

	// Acknowledge first message
	err = store.Acknowledge(ctx, "consumer-1", []string{messages[0].ID})
	require.NoError(t, err, "Acknowledge failed")

	// Consumer-1 should have 1 unread
	unread, err := store.GetUnread(ctx, "consumer-1", "test.topic")
	require.NoError(t, err, "GetUnread failed")
	require.Len(t, unread, 1, "Expected 1 unread message for consumer-1, got %d", len(unread))
	assert.Equal(t, "msg2", unread[0].Payload, "Expected unread message payload 'msg2'")

	// Consumer-2 should have 2 unread (never acknowledged anything)
	unread, err = store.GetUnread(ctx, "consumer-2", "test.topic")
	require.NoError(t, err, "GetUnread for consumer-2 failed")
	assert.Len(t, unread, 2, "Expected 2 unread messages for consumer-2, got %d", len(unread))
}

func TestMsgStore_Acknowledge_EmptyConsumer(t *testing.T) {
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err, "Open")
	defer func() { _ = database.Close() }()

	store := NewMessageStore(database, 0)
	ctx := context.Background()

	err = store.Acknowledge(ctx, "", []string{"msg-id"})
	assert.Error(t, err, "Expected error for empty consumer_id")
}

func TestMsgStore_GetUnread_EmptyConsumer(t *testing.T) {
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err, "Open")
	defer func() { _ = database.Close() }()

	store := NewMessageStore(database, 0)
	ctx := context.Background()

	_, err = store.GetUnread(ctx, "", "test.topic")
	assert.Error(t, err, "Expected error for empty consumer_id")
}

func TestMsgStore_GetUnread_Wildcard(t *testing.T) {
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err, "Open")
	defer func() { _ = database.Close() }()

	store := NewMessageStore(database, 0)
	ctx := context.Background()

	// Publish to multiple topics
	_ = store.Publish(ctx, messaging.Message{Payload: "inbox1"}, []string{"agent.a.inbox"})
	_ = store.Publish(ctx, messaging.Message{Payload: "inbox2"}, []string{"agent.b.inbox"})
	_ = store.Publish(ctx, messaging.Message{Payload: "other"}, []string{"other.topic"})

	// Get unread with wildcard pattern
	unread, err := store.GetUnread(ctx, "consumer-1", "agent.*.inbox")
	require.NoError(t, err, "GetUnread failed")
	require.Len(t, unread, 2, "Expected 2 unread messages, got %d", len(unread))

	payloads := make(map[string]bool)
	for _, m := range unread {
		payloads[m.Payload] = true
	}
	assert.True(t, payloads["inbox1"], "Missing expected payloads in %v", unread)
	assert.True(t, payloads["inbox2"], "Missing expected payloads in %v", unread)
	assert.False(t, payloads["other"], "Should not include messages from other.topic")
}

func TestMsgStore_GetUnread_NoMessages(t *testing.T) {
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err, "Open")
	defer func() { _ = database.Close() }()

	store := NewMessageStore(database, 0)
	ctx := context.Background()

	// No messages published
	unread, err := store.GetUnread(ctx, "consumer-1", "nonexistent.topic")
	require.NoError(t, err, "GetUnread failed")
	assert.Empty(t, unread, "Expected 0 unread messages, got %d", len(unread))
}

func TestMsgStore_PublishMultipleTopics(t *testing.T) {
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err, "Open")
	defer func() { _ = database.Close() }()

	store := NewMessageStore(database, 0)
	ctx := context.Background()

	// Publish to multiple topics at once
	msg := messaging.Message{Payload: "broadcast"}
	err = store.Publish(ctx, msg, []string{"topic.a", "topic.b", "topic.c"})
	require.NoError(t, err, "Publish failed")

	// Verify message exists in all topics
	for _, topic := range []string{"topic.a", "topic.b", "topic.c"} {
		messages, err := store.Subscribe(ctx, topic, time.Time{})
		if err != nil {
			require.NoError(t, err, "Subscribe to %s failed", topic)
			continue
		}
		require.Len(t, messages, 1, "Expected 1 message in %s, got %d", topic, len(messages))
		assert.Equal(t, "broadcast", messages[0].Payload, "Message in %s has payload %q, want 'broadcast'", topic, messages[0].Payload)
	}
}

func TestMsgStore_PublishWildcardExpansion(t *testing.T) {
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err, "Open")
	defer func() { _ = database.Close() }()

	store := NewMessageStore(database, 0)
	ctx := context.Background()

	// First, create some inbox topics
	_ = store.Publish(ctx, messaging.Message{Payload: "setup1"}, []string{"agent.a.inbox"})
	_ = store.Publish(ctx, messaging.Message{Payload: "setup2"}, []string{"agent.b.inbox"})
	_ = store.Publish(ctx, messaging.Message{Payload: "other"}, []string{"other.topic"})

	// Publish with wildcard pattern
	msg := messaging.Message{Payload: "broadcast to inboxes"}
	err = store.Publish(ctx, msg, []string{"agent.*.inbox"})
	require.NoError(t, err, "Publish with wildcard failed")

	// Verify broadcast reached both inboxes
	for _, topic := range []string{"agent.a.inbox", "agent.b.inbox"} {
		messages, _ := store.Subscribe(ctx, topic, time.Time{})
		found := false
		for _, m := range messages {
			if m.Payload == "broadcast to inboxes" {
				found = true
				break
			}
		}
		assert.True(t, found, "Broadcast message not found in %s", topic)
	}

	// Verify other.topic didn't get the broadcast
	messages, _ := store.Subscribe(ctx, "other.topic", time.Time{})
	for _, m := range messages {
		if m.Payload == "broadcast to inboxes" {
			assert.Fail(t, "Broadcast should not have reached other.topic")
		}
	}
}

func TestMsgStore_AcknowledgeIdempotent(t *testing.T) {
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err, "Open")
	defer func() { _ = database.Close() }()

	store := NewMessageStore(database, 0)
	ctx := context.Background()

	// Publish a message
	_ = store.Publish(ctx, messaging.Message{Payload: "test"}, []string{"test.topic"})
	messages, _ := store.Subscribe(ctx, "test.topic", time.Time{})

	// Acknowledge same message twice
	err = store.Acknowledge(ctx, "consumer-1", []string{messages[0].ID})
	require.NoError(t, err, "First Acknowledge failed")

	err = store.Acknowledge(ctx, "consumer-1", []string{messages[0].ID})
	require.NoError(t, err, "Second Acknowledge failed")

	// Should still show 0 unread
	unread, _ := store.GetUnread(ctx, "consumer-1", "test.topic")
	assert.Empty(t, unread, "Expected 0 unread after double-acknowledge, got %d", len(unread))
}
