package form

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/hay-kot/hive/internal/core/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testSessions() []session.Session {
	return []session.Session{
		{ID: "s1", Name: "alpha", Path: "/tmp/alpha", Remote: "git@github.com:org/alpha.git"},
		{ID: "s2", Name: "beta", Path: "/tmp/beta", Remote: "git@github.com:org/beta.git"},
		{ID: "s3", Name: "gamma", Path: "/tmp/gamma", Remote: "git@github.com:org/gamma.git"},
	}
}

func TestSessionSelectorField_Single(t *testing.T) {
	sessions := testSessions()

	t.Run("returns session.Session", func(t *testing.T) {
		f := NewSessionSelectorField("Pick one", sessions, false)
		// Default selection is the first item
		val, ok := f.Value().(session.Session)
		require.True(t, ok, "expected session.Session, got %T", f.Value())
		assert.Equal(t, "s1", val.ID)
		assert.Equal(t, "alpha", val.Name)
	})

	t.Run("label is set", func(t *testing.T) {
		f := NewSessionSelectorField("Pick one", sessions, false)
		assert.Equal(t, "Pick one", f.Label())
	})

	t.Run("focus and blur delegate", func(t *testing.T) {
		f := NewSessionSelectorField("Pick one", sessions, false)
		assert.False(t, f.Focused())
		f.Focus()
		assert.True(t, f.Focused())
		f.Blur()
		assert.False(t, f.Focused())
	})

	t.Run("navigate and select second session", func(t *testing.T) {
		f := NewSessionSelectorField("Pick one", sessions, false)
		f.Focus()
		// Move down to second item
		f.Update(tea.KeyPressMsg(tea.Key{Code: 'j'}))
		val, ok := f.Value().(session.Session)
		require.True(t, ok)
		assert.Equal(t, "s2", val.ID)
		assert.Equal(t, "beta", val.Name)
	})

	t.Run("empty sessions returns zero Session", func(t *testing.T) {
		f := NewSessionSelectorField("Pick one", []session.Session{}, false)
		val, ok := f.Value().(session.Session)
		require.True(t, ok)
		assert.Equal(t, session.Session{}, val)
	})

	t.Run("view renders without panic", func(t *testing.T) {
		f := NewSessionSelectorField("Pick one", sessions, false)
		view := f.View()
		assert.Contains(t, view, "Pick one")
	})
}

func TestSessionSelectorField_Multi(t *testing.T) {
	sessions := testSessions()

	t.Run("returns []session.Session", func(t *testing.T) {
		f := NewSessionSelectorField("Pick many", sessions, true)
		val, ok := f.Value().([]session.Session)
		require.True(t, ok, "expected []session.Session, got %T", f.Value())
		assert.Empty(t, val)
	})

	t.Run("toggle selection returns sessions", func(t *testing.T) {
		f := NewSessionSelectorField("Pick many", sessions, true)
		f.Focus()

		// Toggle first item
		f.Update(tea.KeyPressMsg(tea.Key{Code: ' '}))
		val, ok := f.Value().([]session.Session)
		require.True(t, ok)
		require.Len(t, val, 1)
		assert.Equal(t, "s1", val[0].ID)

		// Move down and toggle second
		f.Update(tea.KeyPressMsg(tea.Key{Code: 'j'}))
		f.Update(tea.KeyPressMsg(tea.Key{Code: ' '}))
		val, ok = f.Value().([]session.Session)
		require.True(t, ok)
		require.Len(t, val, 2)
		assert.Equal(t, "s1", val[0].ID)
		assert.Equal(t, "s2", val[1].ID)
	})

	t.Run("empty sessions returns empty slice", func(t *testing.T) {
		f := NewSessionSelectorField("Pick many", []session.Session{}, true)
		val, ok := f.Value().([]session.Session)
		require.True(t, ok)
		assert.Empty(t, val)
	})

	t.Run("view renders without panic", func(t *testing.T) {
		f := NewSessionSelectorField("Pick many", sessions, true)
		view := f.View()
		assert.Contains(t, view, "Pick many")
	})
}
