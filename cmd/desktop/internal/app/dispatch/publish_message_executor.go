package dispatch

import (
	"context"
	"fmt"
	"strings"

	"github.com/colonyops/hive/pkg/tmpl"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/internal/domain/messaging"
	"github.com/colonyops/hive/internal/platform/observe"
)

const MessageSender = "hive-desktop"

// MessageService is satisfied by *msgsvc.Service.
type MessageService interface {
	Publish(context.Context, messaging.Message, []string) (messaging.PublishResult, error)
}

// PublishMessageExecutor calls messages once per execution, so a hive config
// reload (a changed messaging.max_messages) reaches the next publish.
type PublishMessageExecutor struct{ messages func() MessageService }

func NewPublishMessageExecutor(messages func() MessageService) *PublishMessageExecutor {
	return &PublishMessageExecutor{messages: messages}
}

func (e *PublishMessageExecutor) Execute(ctx context.Context, action actions.Action, data OutputData, _ ActionInvocationInput) (ExecutionResult, error) {
	cfg, ok := action.Config.(*actions.PublishMessageConfig)
	if !ok {
		return ExecutionResult{}, fmt.Errorf("publish-message executor: action %q has config type %T", action.ID, action.Config)
	}
	if e.messages == nil {
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
	runLog := RunLogFrom(ctx)
	runLog.Systemf("Publishing message to %s as %s", cfg.Topic, MessageSender)
	runLog.Systemf("%s", indentLines(payload))
	topic, err := e.publish(ctx, payload, cfg.Topic)
	if err != nil {
		return ExecutionResult{Attempted: true}, err
	}
	runLog.Systemf("Published to %s", topic)
	return ExecutionResult{Attempted: true, Outcome: &ExecutionOutcome{Message: &MessageExecutionOutcome{Topic: topic, Sender: MessageSender}}}, nil
}

// The topic is literal: a wildcard the store expands would fan one action out
// to topics nobody configured, so anything but the exact topic back is an
// error.
func (e *PublishMessageExecutor) publish(ctx context.Context, payload, topic string) (published string, err error) {
	ctx, span := observe.StartConditionalSpan(ctx, tracer, "dispatch.publish-message", trace.WithAttributes(
		attribute.String(attrTopic, topic),
	))
	defer observe.End(span, &err)

	result, err := e.messages().Publish(ctx, messaging.Message{Payload: payload, Sender: MessageSender}, []string{topic})
	if err != nil {
		return "", fmt.Errorf("publish message: %w", err)
	}
	if len(result.Topics) != 1 || result.Topics[0] != topic {
		return "", fmt.Errorf("publish-message: expected topic %q, got %v", topic, result.Topics)
	}
	return topic, nil
}
