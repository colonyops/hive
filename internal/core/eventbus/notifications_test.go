package eventbus_test

import (
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/eventbus"
	"github.com/colonyops/hive/internal/core/eventbus/testbus"
	"github.com/colonyops/hive/internal/core/notify"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func latestNotificationPayload(tb *testbus.Bus, t *testing.T) eventbus.NotificationPublishedPayload {
	t.Helper()
	tb.AssertPublished(t, eventbus.EventNotificationPublished)

	var payload eventbus.NotificationPublishedPayload
	for _, e := range tb.Events() {
		if e.Event != eventbus.EventNotificationPublished {
			continue
		}
		p, ok := e.Payload.(eventbus.NotificationPublishedPayload)
		require.True(t, ok)
		payload = p
	}

	return payload
}

func TestNotificationRouter_SessionCorrupted(t *testing.T) {
	tb := testbus.New(t)
	eventbus.NewNotificationRouter(tb.EventBus).Register()

	tb.PublishSessionCorrupted(eventbus.SessionCorruptedPayload{Session: &session.Session{Name: "alpha"}})
	p := latestNotificationPayload(tb, t)

	assert.Equal(t, notify.LevelWarning, p.Level)
	assert.Contains(t, p.Message, "alpha")
}

func TestNotificationRouter_SessionDeleted(t *testing.T) {
	tb := testbus.New(t)
	eventbus.NewNotificationRouter(tb.EventBus).Register()

	tb.PublishSessionDeleted(eventbus.SessionDeletedPayload{SessionID: "sess-123"})
	p := latestNotificationPayload(tb, t)

	assert.Equal(t, notify.LevelInfo, p.Level)
	assert.Contains(t, p.Message, "sess-123")
}

func TestNotificationRouter_SessionRecycled(t *testing.T) {
	tb := testbus.New(t)
	eventbus.NewNotificationRouter(tb.EventBus).Register()

	tb.PublishSessionRecycled(eventbus.SessionRecycledPayload{Session: &session.Session{Name: "beta"}})
	p := latestNotificationPayload(tb, t)

	assert.Equal(t, notify.LevelInfo, p.Level)
	assert.Contains(t, p.Message, "beta")
}

func TestNotificationRouter_MessageReceived(t *testing.T) {
	tb := testbus.New(t)
	eventbus.NewNotificationRouter(tb.EventBus).Register()

	tb.PublishMessageReceived(eventbus.MessageReceivedPayload{Topic: "agent.test.inbox"})
	p := latestNotificationPayload(tb, t)

	assert.Equal(t, notify.LevelInfo, p.Level)
	assert.Contains(t, p.Message, "agent.test.inbox")
}

func TestNotificationRouter_AgentStatusMissing_publishesWarning(t *testing.T) {
	tb := testbus.New(t)
	eventbus.NewNotificationRouter(tb.EventBus).Register()

	tb.PublishAgentStatusChanged(eventbus.AgentStatusChangedPayload{
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
	eventbus.NewNotificationRouter(tb.EventBus).Register()

	tb.PublishAgentStatusChanged(eventbus.AgentStatusChangedPayload{
		Session:   &session.Session{Name: "agent-a"},
		OldStatus: terminal.StatusActive,
		NewStatus: terminal.StatusReady,
	})

	tb.AssertNotPublished(t, eventbus.EventNotificationPublished, 100*time.Millisecond)
}

func TestNotificationRouter_SessionCreated_doesNotPublish(t *testing.T) {
	tb := testbus.New(t)
	eventbus.NewNotificationRouter(tb.EventBus).Register()

	tb.PublishSessionCreated(eventbus.SessionCreatedPayload{Session: &session.Session{Name: "created"}})
	tb.AssertNotPublished(t, eventbus.EventNotificationPublished, 100*time.Millisecond)
}

func TestNotificationRouter_SessionRenamed_doesNotPublish(t *testing.T) {
	tb := testbus.New(t)
	eventbus.NewNotificationRouter(tb.EventBus).Register()

	tb.PublishSessionRenamed(eventbus.SessionRenamedPayload{Session: &session.Session{Name: "new-name"}, OldName: "old-name"})
	tb.AssertNotPublished(t, eventbus.EventNotificationPublished, 100*time.Millisecond)
}
