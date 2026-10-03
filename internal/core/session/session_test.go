package session

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestValidateName(t *testing.T) {
	valid := []string{
		"my-feature",
		"My Feature",
		"feature-123",
		"ABC",
		"a",
		"fix auth bug",
		"my_feature",
		"JIRA-123: fix auth",
		"v1.2.3",
		"dev/test-thing",
		"release/1.0",
	}
	for _, name := range valid {
		t.Run("valid/"+name, func(t *testing.T) {
			assert.NoError(t, ValidateName(name))
		})
	}

	invalid := []struct {
		input  string
		errMsg string
	}{
		{"", "cannot be empty"},
		{"   ", "cannot be empty"},
		{"my~feature", "invalid session name"},
		{"fix^1", "invalid session name"},
		{"feat*", "invalid session name"},
		{"feat?", "invalid session name"},
		{"feat[0]", "invalid session name"},
		{`feat\branch`, "invalid session name"},
		{"-starts-with-hyphen", "invalid session name"},
		{strings.Repeat("a", MaxNameLength+1), "the maximum is"},
	}
	for _, tt := range invalid {
		t.Run("invalid/"+tt.input, func(t *testing.T) {
			err := ValidateName(tt.input)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
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
		{"accents fold", "Café Crème", "cafe-creme"},
		{"undecomposable letters fold", "Straße Ørsted", "strasse-orsted"},
		{"no ascii letters", "修正 🚀", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Slugify(tt.in))
		})
	}
}

func TestToSessionName(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		fallbacks []string
		want      string
	}{
		{"slugifies", "Fix: the Bug!", nil, "fix-the-bug"},
		{"caps on a word boundary", "gh-1234-" + strings.Repeat("word-", 20), nil, "gh-1234-word-word-word-word-word-word-word-word-word-word"},
		{"cuts a single long word", strings.Repeat("a", 80), nil, strings.Repeat("a", MaxNameLength)},
		{"falls back past empty candidates", "修正 🚀", []string{"", "item 7"}, "item-7"},
		{"empty when nothing survives", "!!!", []string{"???"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToSessionName(tt.raw, tt.fallbacks...)
			assert.Equal(t, tt.want, got)
			if got != "" {
				assert.NoError(t, ValidateName(got))
			}
		})
	}
}

func TestNameWithSuffix(t *testing.T) {
	assert.Equal(t, "fix-bug-2", NameWithSuffix("fix-bug", "2"))
	assert.Equal(t, "Fix Bug-rerun-3", NameWithSuffix("Fix Bug", "rerun-3"))

	long := ToSessionName(strings.Repeat("word ", 20))
	got := NameWithSuffix(long, "rerun-12345")
	assert.LessOrEqual(t, len(got), MaxNameLength)
	assert.True(t, strings.HasSuffix(got, "-word-rerun-12345"), got)
	assert.NoError(t, ValidateName(got))
}
