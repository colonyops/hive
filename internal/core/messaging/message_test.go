package messaging

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewMessage(t *testing.T) {
	t.Run("valid message", func(t *testing.T) {
		m, err := NewMessage("test.topic", "hello world")
		assert.NoError(t, err)
		assert.Equal(t, "test.topic", m.Topic)
		assert.Equal(t, "hello world", m.Payload)
	})

	t.Run("empty topic", func(t *testing.T) {
		_, err := NewMessage("", "payload")
		assert.ErrorIs(t, err, ErrEmptyTopic)
	})

	t.Run("empty payload is valid", func(t *testing.T) {
		m, err := NewMessage("topic", "")
		assert.NoError(t, err)
		assert.Equal(t, "", m.Payload)
	})

	t.Run("payload at max size", func(t *testing.T) {
		payload := strings.Repeat("x", MaxPayloadSize)
		m, err := NewMessage("topic", payload)
		assert.NoError(t, err)
		assert.Len(t, m.Payload, MaxPayloadSize)
	})

	t.Run("payload exceeds max size", func(t *testing.T) {
		payload := strings.Repeat("x", MaxPayloadSize+1)
		_, err := NewMessage("topic", payload)
		assert.ErrorIs(t, err, ErrPayloadTooLarge)
	})
}

func TestMessage_Validate(t *testing.T) {
	t.Run("valid message", func(t *testing.T) {
		m := Message{Topic: "topic", Payload: "data"}
		assert.NoError(t, m.Validate())
	})

	t.Run("empty topic", func(t *testing.T) {
		m := Message{Topic: "", Payload: "data"}
		assert.ErrorIs(t, m.Validate(), ErrEmptyTopic)
	})

	t.Run("payload too large", func(t *testing.T) {
		m := Message{Topic: "topic", Payload: strings.Repeat("x", MaxPayloadSize+1)}
		assert.ErrorIs(t, m.Validate(), ErrPayloadTooLarge)
	})
}
