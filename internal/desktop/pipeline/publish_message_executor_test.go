package pipeline

import (
	"context"
	"errors"
	"testing"

	"github.com/colonyops/hive/internal/desktop/pipeline/actions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type messagePublisherTest struct {
	payload string
	topic   string
	err     error
	actual  string
}

func (m *messagePublisherTest) PublishMessage(_ context.Context, payload, topic string) (string, error) {
	m.payload, m.topic = payload, topic
	if m.actual != "" {
		return m.actual, m.err
	}
	return topic, m.err
}

func publishMessageAction() actions.Action {
	return actions.Action{ID: "notify", Type: "publish-message", Config: &actions.PublishMessageConfig{Topic: "agent.session.inbox", MessageTemplate: "hello {{ .Payload.name }}"}}
}

func TestPublishMessageExecutor_RendersPayloadToConfiguredLiteralTopic(t *testing.T) {
	publisher := &messagePublisherTest{}
	result, err := NewPublishMessageExecutor(publisher).Execute(t.Context(), publishMessageAction(), OutputData{Payload: map[string]any{"name": "Ada"}}, ActionInvocationInput{})
	require.NoError(t, err)
	assert.Equal(t, "hello Ada", publisher.payload)
	assert.Equal(t, "agent.session.inbox", publisher.topic)
	require.NotNil(t, result.Outcome)
	assert.Equal(t, "agent.session.inbox", result.Outcome.Message.Topic)
}

func TestPublishMessageExecutor_PropagatesPublisherFailure(t *testing.T) {
	publisher := &messagePublisherTest{err: errors.New("store unavailable")}
	_, err := NewPublishMessageExecutor(publisher).Execute(t.Context(), publishMessageAction(), OutputData{Payload: map[string]any{"name": "Ada"}}, ActionInvocationInput{})
	require.ErrorIs(t, err, publisher.err)
}

func TestPublishMessageExecutor_RejectsWrongConfigOrMissingPublisher(t *testing.T) {
	_, err := NewPublishMessageExecutor(&messagePublisherTest{}).Execute(t.Context(), actions.Action{ID: "x", Type: "publish-message", Config: &actions.ShellConfig{}}, OutputData{}, ActionInvocationInput{})
	require.Error(t, err)
	_, err = NewPublishMessageExecutor(nil).Execute(t.Context(), publishMessageAction(), OutputData{Payload: map[string]any{"name": "Ada"}}, ActionInvocationInput{})
	require.Error(t, err)
}

func TestPublishMessageExecutor_RejectsTopicMismatch(t *testing.T) {
	publisher := &messagePublisherTest{actual: "expanded.topic"}
	_, err := NewPublishMessageExecutor(publisher).Execute(t.Context(), publishMessageAction(), OutputData{Payload: map[string]any{"name": "Ada"}}, ActionInvocationInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected topic")
}
