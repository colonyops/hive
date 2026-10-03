package hive

import (
	"testing"
	"time"

	"github.com/colonyops/hive/internal/domain/notify"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/domain/terminal"
	"github.com/colonyops/hive/internal/hive/events"
	"github.com/colonyops/hive/internal/hive/events/testbus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func latestNotificationPayload(tb *testbus.Bus, t *testing.T) events.NotificationPublishedPayload {
	t.Helper()
	tb.AssertPublished(t, events.EventNotificationPublished)

	var payload events.NotificationPublishedPayload
	for _, e := range tb.Events() {
		if e.Event != events.EventNotificationPublished {
			continue
		}
		p, ok := e.Payload.(events.NotificationPublishedPayload)
		require.True(t, ok)
		payload = p
	}

	return payload
}

func TestNotificationRouter_SessionCorrupted(t *testing.T) {
	tb := testbus.New(t)
	NewNotificationRouter(tb.EventBus).Register()

	tb.PublishSessionCorrupted(events.SessionCorruptedPayload{Session: &session.Session{Name: "alpha"}})
	p := latestNotificationPayload(tb, t)

	assert.Equal(t, notify.LevelWarning, p.Level)
	assert.Contains(t, p.Message, "alpha")
}

func TestNotificationRouter_SessionDeleted(t *testing.T) {
	tb := testbus.New(t)
	NewNotificationRouter(tb.EventBus).Register()

	tb.PublishSessionDeleted(events.SessionDeletedPayload{SessionID: "sess-123"})
	p := latestNotificationPayload(tb, t)

	assert.Equal(t, notify.LevelInfo, p.Level)
	assert.Contains(t, p.Message, "sess-123")
}

func TestNotificationRouter_SessionRecycled(t *testing.T) {
	tb := testbus.New(t)
	NewNotificationRouter(tb.EventBus).Register()

	tb.PublishSessionRecycled(events.SessionRecycledPayload{Session: &session.Session{Name: "beta"}})
	p := latestNotificationPayload(tb, t)

	assert.Equal(t, notify.LevelInfo, p.Level)
	assert.Contains(t, p.Message, "beta")
}

func TestNotificationRouter_MessageReceived(t *testing.T) {
	tb := testbus.New(t)
	NewNotificationRouter(tb.EventBus).Register()

	tb.PublishMessageReceived(events.MessageReceivedPayload{Topic: "agent.test.inbox"})
	p := latestNotificationPayload(tb, t)

	assert.Equal(t, notify.LevelInfo, p.Level)
	assert.Contains(t, p.Message, "agent.test.inbox")
}

func TestNotificationRouter_AgentStatusMissing_publishesWarning(t *testing.T) {
	tb := testbus.New(t)
	NewNotificationRouter(tb.EventBus).Register()

	tb.PublishAgentStatusChanged(events.AgentStatusChangedPayload{
		Session:   &session.Session{Name: "agent-a"},
		OldStatus: terminal.StatusActive,
		NewStatus: terminal.StatusMissing,
	})
	p := latestNotificationPayload(tb, t)

	assert.Equal(t, notify.LevelWarning, p.Level)
	assert.Contains(t, p.Message, "agent-a")
}

func TestNotificationRouter_AgentStatusReady_doesNotPublish(t *testing.T) {
	tb := testbus.New(t)
	NewNotificationRouter(tb.EventBus).Register()

	tb.PublishAgentStatusChanged(events.AgentStatusChangedPayload{
		Session:   &session.Session{Name: "agent-a"},
		OldStatus: terminal.StatusActive,
		NewStatus: terminal.StatusReady,
	})

	tb.AssertNotPublished(t, events.EventNotificationPublished, 100*time.Millisecond)
}

func TestNotificationRouter_SessionCreated_doesNotPublish(t *testing.T) {
	tb := testbus.New(t)
	NewNotificationRouter(tb.EventBus).Register()

	tb.PublishSessionCreated(events.SessionCreatedPayload{Session: &session.Session{Name: "created"}})
	tb.AssertNotPublished(t, events.EventNotificationPublished, 100*time.Millisecond)
}

func TestNotificationRouter_SessionRenamed_doesNotPublish(t *testing.T) {
	tb := testbus.New(t)
	NewNotificationRouter(tb.EventBus).Register()

	tb.PublishSessionRenamed(events.SessionRenamedPayload{Session: &session.Session{Name: "new-name"}, OldName: "old-name"})
	tb.AssertNotPublished(t, events.EventNotificationPublished, 100*time.Millisecond)
}
