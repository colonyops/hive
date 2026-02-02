package stores

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/hay-kot/hive/internal/core/messaging"
	"github.com/hay-kot/hive/internal/data/db"
	"github.com/hay-kot/hive/pkg/randid"
)

// MessageStore implements messaging.Store using SQLite.
type MessageStore struct {
	db          *db.DB
	maxMessages int
}

var _ messaging.Store = (*MessageStore)(nil)

// NewMessageStore creates a new SQLite-backed message store.
// maxMessages controls retention per topic (0 = unlimited).
func NewMessageStore(db *db.DB, maxMessages int) *MessageStore {
	return &MessageStore{
		db:          db,
		maxMessages: maxMessages,
	}
}

// Publish adds a message to a topic, creating the topic if it doesn't exist.
// Enforces retention limit by deleting oldest messages if needed.
func (m *MessageStore) Publish(ctx context.Context, msg messaging.Message) error {
	// Auto-generate ID and timestamp if not set
	if msg.ID == "" {
		msg.ID = randid.Generate(8)
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}

	// Start transaction for atomic publish + retention
	return m.db.WithTx(ctx, func(q *db.Queries) error {
		// Insert message
		err := q.PublishMessage(ctx, db.PublishMessageParams{
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
				err = q.DeleteOldestMessagesInTopic(ctx, db.DeleteOldestMessagesInTopicParams{
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

// Subscribe returns all messages for a topic pattern, optionally filtered by since timestamp.
// The topic parameter supports wildcards:
//   - "*" or "" returns messages from all topics
//   - "prefix.*" matches topics starting with "prefix."
//
// Returns ErrTopicNotFound if no matching topics exist.
func (m *MessageStore) Subscribe(ctx context.Context, topic string, since time.Time) ([]messaging.Message, error) {
	// Get all topics
	allTopics, err := m.db.Queries().ListTopics(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list topics: %w", err)
	}

	// Match topics based on pattern
	var matchedTopics []string
	switch {
	case topic == "" || topic == "*":
		matchedTopics = allTopics
	case strings.HasSuffix(topic, ".*"):
		// Wildcard match
		prefix := strings.TrimSuffix(topic, ".*") + "."
		for _, t := range allTopics {
			if strings.HasPrefix(t, prefix) {
				matchedTopics = append(matchedTopics, t)
			}
		}
	default:
		// Exact match
		for _, t := range allTopics {
			if t == topic {
				matchedTopics = append(matchedTopics, topic)
				break
			}
		}
	}

	if len(matchedTopics) == 0 {
		return nil, messaging.ErrTopicNotFound
	}

	// Collect messages from all matched topics
	var messages []messaging.Message
	for _, t := range matchedTopics {
		rows, err := m.db.Queries().SubscribeToTopic(ctx, db.SubscribeToTopicParams{
			Topic:     t,
			CreatedAt: since.UnixNano(),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to subscribe to topic %s: %w", t, err)
		}

		for _, row := range rows {
			messages = append(messages, rowToMessage(row))
		}
	}

	// Sort messages by timestamp (chronological order)
	// Messages are already sorted per-topic, but need sorting across topics
	for i := 0; i < len(messages)-1; i++ {
		for j := i + 1; j < len(messages); j++ {
			if messages[i].CreatedAt.After(messages[j].CreatedAt) {
				messages[i], messages[j] = messages[j], messages[i]
			}
		}
	}

	return messages, nil
}

// List returns all topic names.
func (m *MessageStore) List(ctx context.Context) ([]string, error) {
	topics, err := m.db.Queries().ListTopics(ctx)
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
	count, err := m.db.Queries().CountPrunableMessages(ctx, cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to count prunable messages: %w", err)
	}

	// Delete messages
	err = m.db.Queries().PruneMessages(ctx, cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to prune messages: %w", err)
	}

	return int(count), nil
}

// rowToMessage converts a db.Message to a messaging.Message.
func rowToMessage(row db.Message) messaging.Message {
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
