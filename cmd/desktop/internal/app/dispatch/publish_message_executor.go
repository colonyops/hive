package dispatch

import (
	"context"
	"fmt"
	"strings"

	"github.com/colonyops/hive/pkg/tmpl"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/internal/platform/observe"
)

type MessagePublisher interface {
	PublishMessage(ctx context.Context, payload, topic string) (string, error)
}

type PublishMessageExecutor struct{ publisher MessagePublisher }

func NewPublishMessageExecutor(publisher MessagePublisher) *PublishMessageExecutor {
	return &PublishMessageExecutor{publisher: publisher}
}

func (e *PublishMessageExecutor) Execute(ctx context.Context, action actions.Action, data OutputData, _ ActionInvocationInput) (ExecutionResult, error) {
	cfg, ok := action.Config.(*actions.PublishMessageConfig)
	if !ok {
		return ExecutionResult{}, fmt.Errorf("publish-message executor: action %q has config type %T", action.ID, action.Config)
	}
	if e.publisher == nil {
		return ExecutionResult{}, fmt.Errorf("publish-message executor: no message publisher configured")
	}
	payload, err := tmpl.New(tmpl.Config{}).Render(cfg.MessageTemplate, data)
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("publish-message: message_template: %w", err)
	}
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return ExecutionResult{}, fmt.Errorf("publish-message: message_template rendered blank payload")
	}
	topic, err := e.publish(ctx, payload, cfg.Topic)
	if err != nil {
		return ExecutionResult{Attempted: true}, err
	}
	if topic != cfg.Topic {
		return ExecutionResult{Attempted: true}, fmt.Errorf("publish-message: expected topic %q, got %q", cfg.Topic, topic)
	}
	return ExecutionResult{Attempted: true, Outcome: &ExecutionOutcome{Message: &MessageExecutionOutcome{Topic: topic, Sender: "hive-desktop"}}}, nil
}

// The broker is another process, and it answers with the topic it published
// on, which is what the caller checks.
func (e *PublishMessageExecutor) publish(ctx context.Context, payload, topic string) (published string, err error) {
	ctx, span := observe.StartConditionalSpan(ctx, tracer, "dispatch.publish-message", trace.WithAttributes(
		attribute.String(attrTopic, topic),
	))
	defer observe.End(span, &err)

	return e.publisher.PublishMessage(ctx, payload, topic)
}
