package messages

import (
	"errors"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/messaging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadMessages_NilService(t *testing.T) {
	cmd := loadMessages(nil, "*", time.Time{})
	require.NotNil(t, cmd, "loadMessages with nil service returns a cmd")
	msg := cmd()
	loaded, ok := msg.(messagesLoadedMsg)
	require.True(t, ok)
	require.NoError(t, loaded.err, "nil service returns no error")
	assert.Nil(t, loaded.messages)
}

func TestHandleMessagesLoaded_WithError(t *testing.T) {
	v := New(nil, "*", "")
	err := errors.New("store unavailable")
	msg := messagesLoadedMsg{err: err}
	cmd := v.handleMessagesLoaded(msg)
	assert.Nil(t, cmd, "error case returns nil cmd")
	// View state should be unchanged (no messages appended)
	assert.Equal(t, 0, v.ctrl.Len(), "no messages appended on error")
}

func TestHandleMessagesLoaded_WithMessages(t *testing.T) {
	v := New(nil, "*", "")
	msgs := []messaging.Message{
		{Topic: "t", Sender: "s", Payload: "hello", CreatedAt: time.Now()},
	}
	msg := messagesLoadedMsg{messages: msgs}
	cmd := v.handleMessagesLoaded(msg)
	assert.Nil(t, cmd)
	assert.Equal(t, 1, v.ctrl.Len())
	assert.False(t, v.lastPollTime.IsZero(), "lastPollTime updated on success")
}

func TestHandleMessagesLoaded_EmptyMessages(t *testing.T) {
	v := New(nil, "*", "")
	msg := messagesLoadedMsg{messages: nil}
	cmd := v.handleMessagesLoaded(msg)
	assert.Nil(t, cmd)
	assert.Equal(t, 0, v.ctrl.Len(), "zero messages appended when list is empty")
}

func TestHandleMessagesLoaded_ErrorLeavesStateUnchanged(t *testing.T) {
	v := New(nil, "*", "")
	// Pre-load a message
	v.ctrl.Append([]messaging.Message{
		{Topic: "t", Sender: "s", Payload: "existing", CreatedAt: time.Now()},
	})

	// Simulate load failure
	err := errors.New("network error")
	v.handleMessagesLoaded(messagesLoadedMsg{err: err})

	// Original message should still be there
	assert.Equal(t, 1, v.ctrl.Len(), "existing messages preserved on error")
}
