package dispatch

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/messaging"
	"github.com/colonyops/hive/internal/hive/events"
	msgsvc "github.com/colonyops/hive/internal/hive/messaging"
	"github.com/colonyops/hive/internal/store"
	coredb "github.com/colonyops/hive/internal/store/db"
)

type messageServiceTest struct {
	message messaging.Message
	topics  []string
	err     error
	actual  []string
}

func (m *messageServiceTest) Publish(_ context.Context, message messaging.Message, topics []string) (messaging.PublishResult, error) {
	m.message, m.topics = message, topics
	if m.err != nil {
		return messaging.PublishResult{}, m.err
	}
	if m.actual != nil {
		return messaging.PublishResult{Topics: m.actual}, nil
	}
	return messaging.PublishResult{Topics: topics}, nil
}

func messagesOf(service MessageService) func() MessageService {
	return func() MessageService { return service }
}

func publishMessageAction() actions.Action {
	return actions.Action{ID: "notify", Type: "publish-message", Config: &actions.PublishMessageConfig{Topic: "agent.session.inbox", MessageTemplate: "hello {{ .Payload.name }}"}}
}

func TestPublishMessageExecutor_RendersPayloadToConfiguredLiteralTopic(t *testing.T) {
	service := &messageServiceTest{}
	result, err := NewPublishMessageExecutor(messagesOf(service)).Execute(t.Context(), publishMessageAction(), OutputData{Payload: map[string]any{"name": "Ada"}}, ActionInvocationInput{})
	require.NoError(t, err)
	assert.Equal(t, messaging.Message{Payload: "hello Ada", Sender: "hive-desktop"}, service.message)
	assert.Equal(t, []string{"agent.session.inbox"}, service.topics)
	require.NotNil(t, result.Outcome)
	assert.Equal(t, &MessageExecutionOutcome{Topic: "agent.session.inbox", Sender: "hive-desktop"}, result.Outcome.Message)
}

func TestPublishMessageExecutor_PropagatesPublisherFailure(t *testing.T) {
	service := &messageServiceTest{err: errors.New("store unavailable")}
	_, err := NewPublishMessageExecutor(messagesOf(service)).Execute(t.Context(), publishMessageAction(), OutputData{Payload: map[string]any{"name": "Ada"}}, ActionInvocationInput{})
	require.ErrorIs(t, err, service.err)
}

func TestPublishMessageExecutor_RejectsWrongConfigOrMissingPublisher(t *testing.T) {
	_, err := NewPublishMessageExecutor(messagesOf(&messageServiceTest{})).Execute(t.Context(), actions.Action{ID: "x", Type: "publish-message", Config: &actions.ShellConfig{}}, OutputData{}, ActionInvocationInput{})
	require.Error(t, err)
	_, err = NewPublishMessageExecutor(nil).Execute(t.Context(), publishMessageAction(), OutputData{Payload: map[string]any{"name": "Ada"}}, ActionInvocationInput{})
	require.Error(t, err)
}

func TestPublishMessageExecutor_RejectsTopicMismatch(t *testing.T) {
	service := &messageServiceTest{actual: []string{"expanded.topic"}}
	_, err := NewPublishMessageExecutor(messagesOf(service)).Execute(t.Context(), publishMessageAction(), OutputData{Payload: map[string]any{"name": "Ada"}}, ActionInvocationInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected topic")
}

func TestPublishMessageExecutor_PersistsThroughCoreSQLiteReopen(t *testing.T) {
	dir := t.TempDir()
	first, err := coredb.Open(dir, coredb.DefaultOpenOptions())
	require.NoError(t, err)
	service := msgsvc.NewService(store.NewMessageStore(first, 0), &config.Config{}, events.New(8))
	const topic = "agent.session.inbox"
	action := actions.Action{ID: "notify", Type: "publish-message", Config: &actions.PublishMessageConfig{Topic: topic, MessageTemplate: "hello from desktop"}}
	_, err = NewPublishMessageExecutor(func() MessageService { return service }).Execute(t.Context(), action, OutputData{}, ActionInvocationInput{})
	require.NoError(t, err)
	require.NoError(t, first.Close())

	reopened, err := coredb.Open(dir, coredb.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })
	reopenedService := msgsvc.NewService(store.NewMessageStore(reopened, 0), &config.Config{}, events.New(8))
	messages, err := reopenedService.Subscribe(t.Context(), topic, time.Time{})
	require.NoError(t, err)
	require.Len(t, messages, 1)
	assert.Equal(t, "hello from desktop", messages[0].Payload)
	assert.Equal(t, "hive-desktop", messages[0].Sender)
	assert.Empty(t, messages[0].SessionID)
	assert.Equal(t, topic, messages[0].Topic)
}
