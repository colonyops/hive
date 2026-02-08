package tui

import (
	"testing"
	"time"

	"github.com/hay-kot/hive/internal/core/messaging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestMessages(n int) []messaging.Message {
	msgs := make([]messaging.Message, n)
	for i := range n {
		msgs[i] = messaging.Message{
			Topic:     "test.topic",
			Sender:    "agent-" + itoa(i),
			Payload:   "payload " + itoa(i),
			CreatedAt: time.Now(),
		}
	}
	return msgs
}

func TestMessagesView_Navigation(t *testing.T) {
	v := NewMessagesView()
	v.SetSize(120, 40)
	v.SetMessages(newTestMessages(5))

	// Starts at top
	assert.Equal(t, 0, v.cursor)

	// Move down
	v.MoveDown()
	assert.Equal(t, 1, v.cursor)

	// Move up back to 0
	v.MoveUp()
	assert.Equal(t, 0, v.cursor)

	// Move up at top stays at 0
	v.MoveUp()
	assert.Equal(t, 0, v.cursor)

	// Move to end
	for range 10 {
		v.MoveDown()
	}
	assert.Equal(t, 4, v.cursor) // clamped to last
}

func TestMessagesView_Filtering(t *testing.T) {
	v := NewMessagesView()
	v.SetSize(120, 40)
	v.SetMessages([]messaging.Message{
		{Topic: "build.status", Sender: "ci", Payload: "passed"},
		{Topic: "deploy.prod", Sender: "cd", Payload: "deployed"},
		{Topic: "build.logs", Sender: "ci", Payload: "compiling"},
	})

	// All visible initially
	assert.Len(t, v.filteredAt, 3)

	// Start filter and type "deploy"
	v.StartFilter()
	assert.True(t, v.IsFiltering())
	for _, r := range "deploy" {
		v.AddFilterRune(r)
	}
	assert.Len(t, v.filteredAt, 1)

	// Confirm filter
	v.ConfirmFilter()
	assert.False(t, v.IsFiltering())
	assert.Len(t, v.filteredAt, 1)

	// Cancel clears filter
	v.StartFilter()
	v.CancelFilter()
	assert.Len(t, v.filteredAt, 3)
}

func TestMessagesView_DeleteFilterRune(t *testing.T) {
	v := NewMessagesView()
	v.SetSize(120, 40)
	v.SetMessages(newTestMessages(3))

	v.StartFilter()
	v.AddFilterRune('a')
	v.AddFilterRune('b')
	assert.Equal(t, "ab", v.filter)

	v.DeleteFilterRune()
	assert.Equal(t, "a", v.filter)

	v.DeleteFilterRune()
	assert.Empty(t, v.filter)

	// Delete on empty is safe
	v.DeleteFilterRune()
	assert.Empty(t, v.filter)
}

func TestMessagesView_SelectedMessage(t *testing.T) {
	v := NewMessagesView()
	v.SetSize(120, 40)

	// No messages → nil
	assert.Nil(t, v.SelectedMessage())

	msgs := newTestMessages(3)
	v.SetMessages(msgs)

	// First message selected
	selected := v.SelectedMessage()
	require.NotNil(t, selected)
	assert.Equal(t, "agent-0", selected.Sender)

	// Navigate to second
	v.MoveDown()
	selected = v.SelectedMessage()
	require.NotNil(t, selected)
	assert.Equal(t, "agent-1", selected.Sender)
}

func TestMessagesView_MatchesFilter(t *testing.T) {
	v := NewMessagesView()

	tests := []struct {
		name   string
		msg    messaging.Message
		filter string
		want   bool
	}{
		{
			name:   "matches topic",
			msg:    messaging.Message{Topic: "build.status"},
			filter: "build",
			want:   true,
		},
		{
			name:   "matches sender",
			msg:    messaging.Message{Sender: "agent-1"},
			filter: "agent",
			want:   true,
		},
		{
			name:   "matches payload",
			msg:    messaging.Message{Payload: "hello world"},
			filter: "world",
			want:   true,
		},
		{
			name:   "case insensitive",
			msg:    messaging.Message{Topic: "Build.Status"},
			filter: "build",
			want:   true,
		},
		{
			name:   "no match",
			msg:    messaging.Message{Topic: "x", Sender: "y", Payload: "z"},
			filter: "nope",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, v.matchesFilter(&tt.msg, tt.filter))
		})
	}
}
