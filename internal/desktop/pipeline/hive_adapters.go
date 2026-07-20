package pipeline

import (
	"context"
	"fmt"

	"github.com/colonyops/hive/internal/core/eventbus"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/hive"
)

// SessionCreator is the subset of Hive's session service needed by a
// launch-session action.
type SessionCreator interface {
	CreateSession(context.Context, hive.CreateOptions) (*session.Session, error)
}

// HiveSessionLauncher adapts Hive's session service to SessionLauncher.
type HiveSessionLauncher struct {
	sessions SessionCreator
}

func NewHiveSessionLauncher(sessions SessionCreator) *HiveSessionLauncher {
	return &HiveSessionLauncher{sessions: sessions}
}

func (l *HiveSessionLauncher) LaunchSession(ctx context.Context, req LaunchSessionRequest) error {
	if l.sessions == nil {
		return fmt.Errorf("launch session: hive session service is unavailable")
	}
	_, err := l.sessions.CreateSession(ctx, hive.CreateOptions{
		Name:       req.Name,
		Prompt:     req.Prompt,
		Remote:     req.Repo,
		AgentKey:   req.Agent,
		Background: true,
	})
	if err != nil {
		return fmt.Errorf("create hive session: %w", err)
	}
	return nil
}

// InternalEventBus is the subset of Hive's in-process event bus needed by a
// publish-event action.
type InternalEventBus interface {
	Publish(eventbus.Event, any) error
}

// EventBusPublisher adapts Hive's in-process event bus to EventPublisher.
type EventBusPublisher struct {
	bus InternalEventBus
}

func NewEventBusPublisher(bus InternalEventBus) *EventBusPublisher {
	return &EventBusPublisher{bus: bus}
}

func (p *EventBusPublisher) Publish(_ context.Context, topic string, payload []byte) error {
	if p.bus == nil {
		return fmt.Errorf("publish event: hive event bus is unavailable")
	}
	// The worker owns the command payload; retain an independent copy after
	// Execute returns so callers cannot mutate an event already queued.
	return p.bus.Publish(eventbus.Event(topic), append([]byte(nil), payload...))
}
