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

func TestSession_UpdateLastInboxRead(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	s := Session{
		ID:        "test-id",
		UpdatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	s.UpdateLastInboxRead(now)

	assert.NotNil(t, s.LastInboxRead)
	assert.Equal(t, now, *s.LastInboxRead)
	assert.Equal(t, now, s.UpdatedAt)
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

func TestNewSession(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	t.Run("valid session", func(t *testing.T) {
		s, err := NewSession("test-id", "My Session", "/path/to/session", "https://github.com/test/repo", now)
		assert.NoError(t, err)
		assert.Equal(t, "test-id", s.ID)
		assert.Equal(t, "My Session", s.Name)
		assert.Equal(t, "my-session", s.Slug)
		assert.Equal(t, "/path/to/session", s.Path)
		assert.Equal(t, "https://github.com/test/repo", s.Remote)
		assert.Equal(t, StateActive, s.State)
		assert.Equal(t, now, s.CreatedAt)
		assert.Equal(t, now, s.UpdatedAt)
	})

	t.Run("empty id", func(t *testing.T) {
		_, err := NewSession("", "My Session", "/path", "https://example.com", now)
		assert.ErrorIs(t, err, ErrEmptyID)
	})

	t.Run("empty name", func(t *testing.T) {
		_, err := NewSession("id", "", "/path", "https://example.com", now)
		assert.ErrorIs(t, err, ErrEmptyName)
	})

	t.Run("empty path", func(t *testing.T) {
		_, err := NewSession("id", "name", "", "https://example.com", now)
		assert.ErrorIs(t, err, ErrEmptyPath)
	})

	t.Run("empty remote", func(t *testing.T) {
		_, err := NewSession("id", "name", "/path", "", now)
		assert.ErrorIs(t, err, ErrEmptyRemote)
	})
}

func TestSession_Validate(t *testing.T) {
	validSession := Session{
		ID:     "test-id",
		Name:   "Test",
		Path:   "/path",
		Remote: "https://example.com",
		State:  StateActive,
	}

	t.Run("valid session passes", func(t *testing.T) {
		s := validSession
		assert.NoError(t, s.Validate())
	})

	t.Run("invalid state fails", func(t *testing.T) {
		s := validSession
		s.State = "invalid"
		assert.ErrorIs(t, s.Validate(), ErrInvalidState)
	})
}

func TestState_IsValid(t *testing.T) {
	tests := []struct {
		state State
		want  bool
	}{
		{StateActive, true},
		{StateRecycled, true},
		{StateCorrupted, true},
		{"invalid", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			assert.Equal(t, tt.want, tt.state.IsValid())
		})
	}
}
