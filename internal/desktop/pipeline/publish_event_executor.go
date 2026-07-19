package pipeline

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/desktop/pipeline/actions"
)

// EventPublisher publishes a msg payload to a topic.
//
// This is deliberately injectable rather than wired to an existing hive
// event system: internal/core/eventbus is a TUI-owned, code-generated
// (gobusgen) registry of fixed, statically-typed events — adding a
// dynamically-configured actions.yml topic there would mean editing
// generated code per topic, the opposite of what actions.yml is for.
// internal/core/messaging is the CLI's inter-agent inbox system; publishing
// through it requires resolving a hive session (messaging.SessionDetector
// needs a session.Store), which is exactly the session/core machinery the
// desktop app deliberately does not wire (see internal/desktop/desktop.go's
// package doc). Both are poor fits, so PublishEventExecutor talks to this
// narrow interface instead; a real event bus (or a bridge to messaging, once
// the desktop wires its own session concept) can implement it later without
// this package changing. LoggingEventPublisher is the default stub.
type EventPublisher interface {
	Publish(ctx context.Context, topic string, payload []byte) error
}

// LoggingEventPublisher is the default EventPublisher: a stub that logs what
// it would have published. See EventPublisher's doc for why this is the
// default rather than a real implementation.
type LoggingEventPublisher struct {
	Logger zerolog.Logger
}

func (p LoggingEventPublisher) Publish(_ context.Context, topic string, payload []byte) error {
	p.Logger.Info().
		Str("topic", topic).
		RawJSON("payload", payload).
		Msg("publish-event action (stub): would publish to topic")
	return nil
}

// PublishEventExecutor publishes a publish-event action's triggering msg
// payload to its configured topic.
type PublishEventExecutor struct {
	publisher EventPublisher
}

// NewPublishEventExecutor builds a PublishEventExecutor over publisher. A
// nil publisher defaults to LoggingEventPublisher.
func NewPublishEventExecutor(publisher EventPublisher) *PublishEventExecutor {
	if publisher == nil {
		publisher = LoggingEventPublisher{Logger: zerolog.Nop()}
	}
	return &PublishEventExecutor{publisher: publisher}
}

func (e *PublishEventExecutor) Execute(ctx context.Context, action actions.Action, data OutputData) error {
	cfg, ok := action.Config.(*actions.PublishEventConfig)
	if !ok {
		return fmt.Errorf("publish-event executor: action %q has config type %T", action.ID, action.Config)
	}
	return e.publisher.Publish(ctx, cfg.Topic, data.Raw)
}
