package hive

import (
	"context"
	"fmt"
	"time"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/messaging"
	"github.com/colonyops/hive/pkg/randid"
)

// MessageService wraps messaging.Store with domain logic.
type MessageService struct {
	store  messaging.Store
	config *config.Config
}

// NewMessageService creates a new MessageService.
func NewMessageService(store messaging.Store, cfg *config.Config) *MessageService {
	return &MessageService{
		store:  store,
		config: cfg,
	}
}

// Publish adds a message to multiple topics.
func (m *MessageService) Publish(ctx context.Context, msg messaging.Message, topics []string) error {
	return m.store.Publish(ctx, msg, topics)
}

// Subscribe returns all messages for a topic, optionally filtered by since timestamp.
func (m *MessageService) Subscribe(ctx context.Context, topic string, since time.Time) ([]messaging.Message, error) {
	return m.store.Subscribe(ctx, topic, since)
}

// GetUnread returns messages not yet acknowledged by consumer.
func (m *MessageService) GetUnread(ctx context.Context, consumerID string, topic string) ([]messaging.Message, error) {
	return m.store.GetUnread(ctx, consumerID, topic)
}

// Acknowledge marks messages as read by a consumer.
func (m *MessageService) Acknowledge(ctx context.Context, consumerID string, messageIDs []string) error {
	return m.store.Acknowledge(ctx, consumerID, messageIDs)
}

// ListTopics returns all topic names.
func (m *MessageService) ListTopics(ctx context.Context) ([]string, error) {
	return m.store.List(ctx)
}

// Prune removes messages older than the given duration.
func (m *MessageService) Prune(ctx context.Context, olderThan time.Duration) (int, error) {
	return m.store.Prune(ctx, olderThan)
}

// GenerateTopic creates a new topic name using the configured prefix and a random suffix.
func (m *MessageService) GenerateTopic(prefix string) string {
	if prefix == "" {
		prefix = m.config.Messaging.TopicPrefix
	}
	return fmt.Sprintf("%s.%s", prefix, randid.Generate(4))
}
