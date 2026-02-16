package eventbus_test

import (
	"context"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/eventbus"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/rs/zerolog"
)

func TestRegisterDebugLogger(t *testing.T) {
	bus := eventbus.New(64)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go bus.Start(ctx)

	// Register with a nop logger — verifies no panic.
	eventbus.RegisterDebugLogger(bus, zerolog.Nop())

	// Publish a few events to exercise all subscriber paths.
	bus.PublishSessionCreated(eventbus.SessionCreatedPayload{
		Session: &session.Session{ID: "test", Name: "test"},
	})
	bus.PublishTuiStarted(eventbus.TUIStartedPayload{})
	bus.PublishAgentStatusChanged(eventbus.AgentStatusChangedPayload{})

	// Give the bus time to dispatch.
	time.Sleep(100 * time.Millisecond)
}
