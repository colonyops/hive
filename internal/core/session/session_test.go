package session

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSession_CanRecycle(t *testing.T) {
	tests := []struct {
		name  string
		state State
		want  bool
	}{
		{
			name:  "active session can be recycled",
			state: StateActive,
			want:  true,
		},
		{
			name:  "recycled session cannot be recycled",
			state: StateRecycled,
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Session{State: tt.state}
			assert.Equal(t, tt.want, s.CanRecycle())
		})
	}
}

func TestSession_MarkRecycled(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	s := Session{
		ID:        "test-id",
		State:     StateActive,
		UpdatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	s.MarkRecycled(now)

	assert.Equal(t, StateRecycled, s.State)
	assert.Equal(t, now, s.UpdatedAt)
}

func TestSession_InboxTopic(t *testing.T) {
	s := Session{ID: "abc123"}
	assert.Equal(t, "agent.abc123.inbox", s.InboxTopic())
}

func TestSession_GroupAccessors(t *testing.T) {
	t.Run("empty by default", func(t *testing.T) {
		s := Session{}
		assert.Empty(t, s.Group())
	})

	t.Run("set and get", func(t *testing.T) {
		s := Session{}
		s.SetGroup("backend")
		assert.Equal(t, "backend", s.Group())
	})

	t.Run("clear with empty string", func(t *testing.T) {
		s := Session{}
		s.SetGroup("backend")
		s.SetGroup("")
		assert.Empty(t, s.Group())
		_, exists := s.Metadata[MetaGroup]
		assert.False(t, exists, "group key should be removed from metadata")
	})

	t.Run("clear on nil metadata is safe", func(t *testing.T) {
		s := Session{}
		s.SetGroup("")
		assert.Empty(t, s.Group())
	})
}

func TestSlugify(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"simple", "My Session", "my-session"},
		{"multiple spaces", "My   Session   Name", "my-session-name"},
		{"special chars", "Feature: Add Login!", "feature-add-login"},
		{"already slug", "my-session", "my-session"},
		{"leading/trailing spaces", "  My Session  ", "my-session"},
		{"numbers", "Session 123", "session-123"},
		{"underscores", "my_session_name", "my-session-name"},
		{"mixed case", "MySessionName", "mysessionname"},
		{"empty after trim", "   ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Slugify(tt.in))
		})
	}
}
