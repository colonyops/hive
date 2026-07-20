package pipeline

import (
	"context"
	"fmt"

	"github.com/colonyops/hive/internal/desktop/pipeline/actions"
)

// EventPublisher publishes a msg payload to a topic on Hive's internal event
// bus. Implementations must return an error when they cannot enqueue the
// event, so the output worker can retain the command for retry.
type EventPublisher interface {
	Publish(ctx context.Context, topic string, payload []byte) error
}

// PublishEventExecutor publishes a publish-event action's triggering msg
// payload to its configured topic.
type PublishEventExecutor struct {
	publisher EventPublisher
}

// NewPublishEventExecutor builds a PublishEventExecutor over publisher. A nil
// publisher leaves the executor unavailable rather than acknowledging an
// action without publishing its event.
func NewPublishEventExecutor(publisher EventPublisher) *PublishEventExecutor {
	return &PublishEventExecutor{publisher: publisher}
}

func (e *PublishEventExecutor) Execute(ctx context.Context, action actions.Action, data OutputData) error {
	cfg, ok := action.Config.(*actions.PublishEventConfig)
	if !ok {
		return fmt.Errorf("publish-event executor: action %q has config type %T", action.ID, action.Config)
	}
	if e.publisher == nil {
		return fmt.Errorf("publish-event executor: no event publisher configured")
	}
	return e.publisher.Publish(ctx, cfg.Topic, data.Raw)
}
