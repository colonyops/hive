package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/hay-kot/hive/internal/core/messaging"
	"github.com/hay-kot/hive/internal/store/sqlite/sqlc"
)

// MessageStore implements messaging.Store using SQLite.
type MessageStore struct {
	db          *DB
	maxMessages int
}

var _ messaging.Store = (*MessageStore)(nil)

// NewMessageStore creates a new SQLite-backed message store.
// maxMessages controls retention per topic (0 = unlimited).
func NewMessageStore(db *DB, maxMessages int) *MessageStore {
	return &MessageStore{
		db:          db,
		maxMessages: maxMessages,
	}
}

// Publish adds a message to a topic, creating the topic if it doesn't exist.
// Enforces retention limit by deleting oldest messages if needed.
func (m *MessageStore) Publish(ctx context.Context, msg messaging.Message) error {
	// Start transaction for atomic publish + retention
	return m.db.WithTx(ctx, func(q *sqlc.Queries) error {
		// Insert message
		err := q.PublishMessage(ctx, sqlc.PublishMessageParams{
			ID:        msg.ID,
			Topic:     msg.Topic,
			Payload:   msg.Payload,
			Sender:    toNullString(msg.Sender),
			SessionID: toNullString(msg.SessionID),
			CreatedAt: msg.CreatedAt.UnixNano(),
		})
		if err != nil {
			return fmt.Errorf("failed to publish message: %w", err)
		}

		// Enforce retention limit if configured
		if m.maxMessages > 0 {
			count, err := q.CountMessagesInTopic(ctx, msg.Topic)
			if err != nil {
				return fmt.Errorf("failed to count messages: %w", err)
			}

			// Delete oldest messages if over limit
			if count > int64(m.maxMessages) {
				toDelete := count - int64(m.maxMessages)
				err = q.DeleteOldestMessagesInTopic(ctx, sqlc.DeleteOldestMessagesInTopicParams{
					Topic: msg.Topic,
					Limit: toDelete,
				})
				if err != nil {
					return fmt.Errorf("failed to enforce retention: %w", err)
				}
			}
		}

		return nil
	})
}

// Subscribe returns all messages for a topic, optionally filtered by since timestamp.
// Returns ErrTopicNotFound if the topic doesn't exist.
func (m *MessageStore) Subscribe(ctx context.Context, topic string, since time.Time) ([]messaging.Message, error) {
	rows, err := m.db.queries.SubscribeToTopic(ctx, sqlc.SubscribeToTopicParams{
		Topic:     topic,
		CreatedAt: since.UnixNano(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to topic: %w", err)
	}

	// If no messages found, check if topic exists
	if len(rows) == 0 {
		// Check if any messages exist for this topic
		count, err := m.db.queries.CountMessagesInTopic(ctx, topic)
		if err != nil {
			return nil, fmt.Errorf("failed to check topic existence: %w", err)
		}
		if count == 0 {
			return nil, messaging.ErrTopicNotFound
		}
	}

	messages := make([]messaging.Message, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, rowToMessage(row))
	}

	return messages, nil
}

// List returns all topic names.
func (m *MessageStore) List(ctx context.Context) ([]string, error) {
	topics, err := m.db.queries.ListTopics(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list topics: %w", err)
	}
	return topics, nil
}

// Prune removes messages older than the given duration across all topics.
// Returns the number of messages removed.
func (m *MessageStore) Prune(ctx context.Context, olderThan time.Duration) (int, error) {
	cutoff := time.Now().Add(-olderThan).UnixNano()

	// Count messages to be pruned
	count, err := m.db.queries.CountPrunableMessages(ctx, cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to count prunable messages: %w", err)
	}

	// Delete messages
	err = m.db.queries.PruneMessages(ctx, cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to prune messages: %w", err)
	}

	return int(count), nil
}

// rowToMessage converts a sqlc.Message to a messaging.Message.
func rowToMessage(row sqlc.Message) messaging.Message {
	return messaging.Message{
		ID:        row.ID,
		Topic:     row.Topic,
		Payload:   row.Payload,
		Sender:    fromNullString(row.Sender),
		SessionID: fromNullString(row.SessionID),
		CreatedAt: time.Unix(0, row.CreatedAt),
	}
}

// toNullString converts a string to sql.NullString.
func toNullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// fromNullString converts a sql.NullString to a string.
func fromNullString(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}
