package messaging

import (
	"context"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/messaging"
	"github.com/colonyops/hive/internal/hive/events"
	"github.com/colonyops/hive/internal/hive/events/testbus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockMsgStore struct {
	published []struct {
		msg    messaging.Message
		topics []string
	}
}

func (m *mockMsgStore) Publish(_ context.Context, msg messaging.Message, topics []string) (messaging.PublishResult, error) {
	m.published = append(m.published, struct {
		msg    messaging.Message
		topics []string
	}{msg: msg, topics: topics})
	return messaging.PublishResult{Topics: topics}, nil
}

func (m *mockMsgStore) Subscribe(context.Context, string, time.Time) ([]messaging.Message, error) {
	return nil, nil
}

func (m *mockMsgStore) Acknowledge(context.Context, string, []string) error { return nil }

func (m *mockMsgStore) GetUnread(context.Context, string, string) ([]messaging.Message, error) {
	return nil, nil
}

func (m *mockMsgStore) List(context.Context) ([]string, error) { return nil, nil }

func (m *mockMsgStore) Prune(context.Context, time.Duration) (int, error) { return 0, nil }

func TestMessageService_PublishEmitsEvent(t *testing.T) {
	store := &mockMsgStore{}
	tb := testbus.New(t)
	cfg := &config.Config{}

	svc := NewService(store, cfg, tb.EventBus)

	msg := messaging.Message{
		ID:      "msg1",
		Payload: "hello",
	}

	_, err := svc.Publish(context.Background(), msg, []string{"topic.a", "topic.b"})
	require.NoError(t, err)

	tb.AssertPublished(t, events.EventMessageReceived)

	recorded := tb.Events()
	var count int
	for _, e := range recorded {
		if e.Event == events.EventMessageReceived {
			p := e.Payload.(events.MessageReceivedPayload)
			assert.Equal(t, "msg1", p.Message.ID)
			count++
		}
	}
	assert.Equal(t, 2, count, "should emit one event per topic")
}

var _ messaging.Store = (*mockMsgStore)(nil)
