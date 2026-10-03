// Package messaging publishes and reads inter-agent messages.
package messaging

import (
	"context"
	"fmt"
	"time"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/messaging"
	"github.com/colonyops/hive/internal/hive/events"
	"github.com/colonyops/hive/pkg/randid"
)

// Service wraps messaging.Store with domain logic.
type Service struct {
	store  messaging.Store
	config *config.Config
	bus    *events.EventBus
}

// NewService creates a new Service.
func NewService(store messaging.Store, cfg *config.Config, bus *events.EventBus) *Service {
	return &Service{
		store:  store,
		config: cfg,
		bus:    bus,
	}
}

// Publish adds a message to multiple topics.
// Returns the resolved topics after wildcard expansion.
func (m *Service) Publish(ctx context.Context, msg messaging.Message, topics []string) (messaging.PublishResult, error) {
	result, err := m.store.Publish(ctx, msg, topics)
	if err != nil {
		return messaging.PublishResult{}, err
	}

	for _, topic := range result.Topics {
		m.bus.PublishMessageReceived(events.MessageReceivedPayload{
			Topic:   topic,
			Message: &msg,
		})
	}

	return result, nil
}

// Subscribe returns all messages for a topic, optionally filtered by since timestamp.
func (m *Service) Subscribe(ctx context.Context, topic string, since time.Time) ([]messaging.Message, error) {
	return m.store.Subscribe(ctx, topic, since)
}

// GetUnread returns messages not yet acknowledged by consumer.
func (m *Service) GetUnread(ctx context.Context, consumerID string, topic string) ([]messaging.Message, error) {
	return m.store.GetUnread(ctx, consumerID, topic)
}

// Acknowledge marks messages as read by a consumer.
func (m *Service) Acknowledge(ctx context.Context, consumerID string, messageIDs []string) error {
	return m.store.Acknowledge(ctx, consumerID, messageIDs)
}

// ListTopics returns all topic names.
func (m *Service) ListTopics(ctx context.Context) ([]string, error) {
	return m.store.List(ctx)
}

// Prune removes messages older than the given duration.
func (m *Service) Prune(ctx context.Context, olderThan time.Duration) (int, error) {
	return m.store.Prune(ctx, olderThan)
}

// GenerateTopic creates a new topic name using the configured prefix and a random suffix.
func (m *Service) GenerateTopic(prefix string) string {
	if prefix == "" {
		prefix = m.config.Messaging.TopicPrefix
	}
	return fmt.Sprintf("%s.%s", prefix, randid.Generate(4))
}
