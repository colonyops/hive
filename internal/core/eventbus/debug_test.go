package eventbus_test

import (
	"testing"

	"github.com/colonyops/hive/internal/core/eventbus"
	"github.com/colonyops/hive/internal/core/eventbus/testbus"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/rs/zerolog"
)

func TestRegisterDebugLogger(t *testing.T) {
	tb := testbus.New(t)

	// Register with a nop logger — verifies no panic.
	eventbus.RegisterDebugLogger(tb.EventBus, zerolog.Nop())

	// Publish a few events to exercise all subscriber paths.
	tb.PublishSessionCreated(eventbus.SessionCreatedPayload{
		Session: &session.Session{ID: "test", Name: "test"},
	})
	tb.PublishTuiStarted(eventbus.TUIStartedPayload{})
	tb.PublishAgentStatusChanged(eventbus.AgentStatusChangedPayload{
		Session: &session.Session{ID: "agent-test"},
	})

	// Wait for last event to confirm all dispatched without panic.
	tb.AssertPublished(t, eventbus.EventAgentStatusChanged)
}
