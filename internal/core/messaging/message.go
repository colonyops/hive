package messaging

import (
	"errors"
	"time"
)

// Validation errors for Message.
var (
	ErrEmptyTopic      = errors.New("topic is required")
	ErrPayloadTooLarge = errors.New("payload exceeds maximum size")
)

// MaxPayloadSize is the maximum allowed payload size in bytes (1MB).
const MaxPayloadSize = 1 << 20

// Message represents a single message published to a topic.
type Message struct {
	ID        string    `json:"id"`
	Topic     string    `json:"topic"`
	Payload   string    `json:"payload"`
	Sender    string    `json:"sender,omitempty"`
	SessionID string    `json:"session_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// NewMessage creates a new Message with the given topic and payload.
// Returns an error if validation fails.
func NewMessage(topic, payload string) (Message, error) {
	m := Message{
		Topic:   topic,
		Payload: payload,
	}
	if err := m.Validate(); err != nil {
		return Message{}, err
	}
	return m, nil
}

// Validate checks that the message meets all constraints.
func (m *Message) Validate() error {
	if m.Topic == "" {
		return ErrEmptyTopic
	}
	if len(m.Payload) > MaxPayloadSize {
		return ErrPayloadTooLarge
	}
	return nil
}

// Topic represents a named channel for messages.
type Topic struct {
	Name      string    `json:"name"`
	Messages  []Message `json:"messages"`
	UpdatedAt time.Time `json:"updated_at"`
}
