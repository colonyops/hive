package events_test

import (
	"testing"

	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive/events"
	"github.com/colonyops/hive/internal/hive/events/testbus"
	"github.com/rs/zerolog"
)

func TestRegisterDebugLogger(t *testing.T) {
	tb := testbus.New(t)

	// Register with a nop logger — verifies no panic.
	events.RegisterDebugLogger(tb.EventBus, zerolog.Nop())

	// Publish a few events to exercise all subscriber paths.
	tb.PublishSessionCreated(events.SessionCreatedPayload{
		Session: &session.Session{ID: "test", Name: "test"},
	})
	tb.PublishAgentStatusChanged(events.AgentStatusChangedPayload{
		Session: &session.Session{ID: "agent-test"},
	})

	// Wait for last event to confirm all dispatched without panic.
	tb.AssertPublished(t, events.EventAgentStatusChanged)
}
