package form

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testRepos() []Repo {
	return []Repo{
		{Name: "hive", Path: "/home/user/hive", Remote: "git@github.com:org/hive.git"},
		{Name: "tools", Path: "/home/user/tools", Remote: "git@github.com:org/tools.git"},
		{Name: "docs", Path: "/home/user/docs", Remote: "git@github.com:org/docs.git"},
	}
}

func TestProjectSelectorField_Single(t *testing.T) {
	repos := testRepos()

	t.Run("returns Repo", func(t *testing.T) {
		f := NewProjectSelectorField("Pick project", repos, false)
		val, ok := f.Value().(Repo)
		require.True(t, ok, "expected Repo, got %T", f.Value())
		assert.Equal(t, "hive", val.Name)
		assert.Equal(t, "/home/user/hive", val.Path)
	})

	t.Run("label is set", func(t *testing.T) {
		f := NewProjectSelectorField("Pick project", repos, false)
		assert.Equal(t, "Pick project", f.Label())
	})

	t.Run("focus and blur delegate", func(t *testing.T) {
		f := NewProjectSelectorField("Pick project", repos, false)
		assert.False(t, f.Focused())
		f.Focus()
		assert.True(t, f.Focused())
		f.Blur()
		assert.False(t, f.Focused())
	})

	t.Run("navigate and select second repo", func(t *testing.T) {
		f := NewProjectSelectorField("Pick project", repos, false)
		f.Focus()
		// Move down to second item
		f.Update(tea.KeyPressMsg(tea.Key{Code: 'j'}))
		val, ok := f.Value().(Repo)
		require.True(t, ok)
		assert.Equal(t, "tools", val.Name)
		assert.Equal(t, "/home/user/tools", val.Path)
	})

	t.Run("empty repos returns zero Repo", func(t *testing.T) {
		f := NewProjectSelectorField("Pick project", []Repo{}, false)
		val, ok := f.Value().(Repo)
		require.True(t, ok)
		assert.Equal(t, Repo{}, val)
	})

	t.Run("view renders without panic", func(t *testing.T) {
		f := NewProjectSelectorField("Pick project", repos, false)
		view := f.View()
		assert.Contains(t, view, "Pick project")
	})
}

func TestProjectSelectorField_Multi(t *testing.T) {
	repos := testRepos()

	t.Run("returns []Repo", func(t *testing.T) {
		f := NewProjectSelectorField("Pick projects", repos, true)
		val, ok := f.Value().([]Repo)
		require.True(t, ok, "expected []Repo, got %T", f.Value())
		assert.Empty(t, val)
	})

	t.Run("toggle selection returns repos", func(t *testing.T) {
		f := NewProjectSelectorField("Pick projects", repos, true)
		f.Focus()

		// Toggle first item
		f.Update(tea.KeyPressMsg(tea.Key{Code: ' '}))
		val, ok := f.Value().([]Repo)
		require.True(t, ok)
		require.Len(t, val, 1)
		assert.Equal(t, "hive", val[0].Name)

		// Move down and toggle second
		f.Update(tea.KeyPressMsg(tea.Key{Code: 'j'}))
		f.Update(tea.KeyPressMsg(tea.Key{Code: ' '}))
		val, ok = f.Value().([]Repo)
		require.True(t, ok)
		require.Len(t, val, 2)
		assert.Equal(t, "hive", val[0].Name)
		assert.Equal(t, "tools", val[1].Name)
	})

	t.Run("empty repos returns empty slice", func(t *testing.T) {
		f := NewProjectSelectorField("Pick projects", []Repo{}, true)
		val, ok := f.Value().([]Repo)
		require.True(t, ok)
		assert.Empty(t, val)
	})

	t.Run("view renders without panic", func(t *testing.T) {
		f := NewProjectSelectorField("Pick projects", repos, true)
		view := f.View()
		assert.Contains(t, view, "Pick projects")
	})
}
