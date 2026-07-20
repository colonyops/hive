package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/eventbus"
	"github.com/colonyops/hive/internal/core/messaging"
	coredb "github.com/colonyops/hive/internal/data/db"
	"github.com/colonyops/hive/internal/data/stores"
	hivesvc "github.com/colonyops/hive/internal/hive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type durableMessageServiceTest struct {
	message messaging.Message
	topics  []string
}

func (s *durableMessageServiceTest) Publish(_ context.Context, message messaging.Message, topics []string) (messaging.PublishResult, error) {
	s.message, s.topics = message, topics
	return messaging.PublishResult{Topics: topics}, nil
}

func TestHiveMessagePublisherUsesFixedSenderEmptySessionAndLiteralTopic(t *testing.T) {
	service := &durableMessageServiceTest{}
	topic, err := NewHiveMessagePublisher(service).PublishMessage(t.Context(), "hello", "agent.session.inbox")
	require.NoError(t, err)
	assert.Equal(t, "agent.session.inbox", topic)
	assert.Equal(t, messaging.Message{Payload: "hello", Sender: "hive-desktop", SessionID: ""}, service.message)
	assert.Equal(t, []string{"agent.session.inbox"}, service.topics)
}

func TestHiveMessagePublisherPersistsThroughCoreSQLiteReopen(t *testing.T) {
	dir := t.TempDir()
	first, err := coredb.Open(dir, coredb.DefaultOpenOptions())
	require.NoError(t, err)
	service := hivesvc.NewMessageService(stores.NewMessageStore(first, 0), &config.Config{}, eventbus.New(8))
	publisher := NewHiveMessagePublisher(service)
	const topic = "agent.session.inbox"
	published, err := publisher.PublishMessage(t.Context(), "hello from desktop", topic)
	require.NoError(t, err)
	assert.Equal(t, topic, published)
	require.NoError(t, first.Close())

	reopened, err := coredb.Open(dir, coredb.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })
	reopenedService := hivesvc.NewMessageService(stores.NewMessageStore(reopened, 0), &config.Config{}, eventbus.New(8))
	messages, err := reopenedService.Subscribe(t.Context(), topic, time.Time{})
	require.NoError(t, err)
	require.Len(t, messages, 1)
	assert.Equal(t, "hello from desktop", messages[0].Payload)
	assert.Equal(t, "hive-desktop", messages[0].Sender)
	assert.Empty(t, messages[0].SessionID)
	assert.Equal(t, topic, messages[0].Topic)
}
