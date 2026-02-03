package messaging

import (
	"context"
	"errors"
	"time"
)

var ErrTopicNotFound = errors.New("topic not found")

// Store defines the interface for message persistence.
type Store interface {
	// Publish adds a message to multiple topics.
	// Wildcards are expanded before publishing.
	Publish(ctx context.Context, msg Message, topics []string) error

	// Subscribe returns all messages for a topic, optionally filtered by since timestamp.
	// Returns ErrTopicNotFound if the topic doesn't exist.
	Subscribe(ctx context.Context, topic string, since time.Time) ([]Message, error)

	// Acknowledge marks messages as read by a consumer.
	Acknowledge(ctx context.Context, consumerID string, messageIDs []string) error

	// GetUnread returns messages not yet acknowledged by consumer.
	// Supports wildcard topic patterns.
	GetUnread(ctx context.Context, consumerID string, topic string) ([]Message, error)

	// List returns all topic names.
	List(ctx context.Context) ([]string, error)

	// Prune removes messages older than the given duration across all topics.
	// Returns the number of messages removed.
	Prune(ctx context.Context, olderThan time.Duration) (int, error)
}
