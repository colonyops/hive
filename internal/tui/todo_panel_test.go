package tui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/colonyops/hive/internal/core/todo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testItems() []todo.Item {
	return []todo.Item{
		{ID: "1", Type: todo.ItemTypeFileChange, Title: "plan.md", SessionID: "sess-1", RepoRemote: "https://github.com/org/repo", CreatedAt: time.Now().Add(-5 * time.Minute)},
		{ID: "2", Type: todo.ItemTypeCustom, Title: "Review PR #42", SessionID: "sess-2", RepoRemote: "https://github.com/org/repo", CreatedAt: time.Now().Add(-2 * time.Hour)},
		{ID: "3", Type: todo.ItemTypeFileChange, Title: "notes.md", RepoRemote: "https://github.com/org/other", CreatedAt: time.Now().Add(-1 * time.Minute)},
	}
}

func TestTodoActionPanel_Construction(t *testing.T) {
	items := testItems()
	panel := NewTodoActionPanel(items, 80, 24)

	require.NotNil(t, panel)
	assert.False(t, panel.Cancelled())
	assert.Nil(t, panel.Result())
	assert.Equal(t, 0, panel.cursor)
}

func TestTodoActionPanel_EmptyState(t *testing.T) {
	panel := NewTodoActionPanel(nil, 80, 24)

	view := panel.View()
	assert.Contains(t, view, "No pending TODO items")
	assert.False(t, panel.Cancelled())
}

func TestTodoActionPanel_Navigation(t *testing.T) {
	items := testItems()
	panel := NewTodoActionPanel(items, 80, 24)

	// Move down
	panel, _ = panel.Update(tea.KeyPressMsg{Code: -1, Text: "j"})
	assert.Equal(t, 1, panel.cursor)

	// Move down again
	panel, _ = panel.Update(tea.KeyPressMsg{Code: -1, Text: "j"})
	assert.Equal(t, 2, panel.cursor)

	// Move down past end - should stay
	panel, _ = panel.Update(tea.KeyPressMsg{Code: -1, Text: "j"})
	assert.Equal(t, 2, panel.cursor)

	// Move up
	panel, _ = panel.Update(tea.KeyPressMsg{Code: -1, Text: "k"})
	assert.Equal(t, 1, panel.cursor)

	// Move up past beginning - should stay
	panel, _ = panel.Update(tea.KeyPressMsg{Code: -1, Text: "k"})
	assert.Equal(t, 0, panel.cursor)
	panel, _ = panel.Update(tea.KeyPressMsg{Code: -1, Text: "k"})
	assert.Equal(t, 0, panel.cursor)
}

func TestTodoActionPanel_Select(t *testing.T) {
	items := testItems()
	panel := NewTodoActionPanel(items, 80, 24)

	panel, _ = panel.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, panel.Result())
	assert.Equal(t, TodoPanelSelect, panel.Result().Action)
	assert.Equal(t, "1", panel.Result().Item.ID)
}

func TestTodoActionPanel_Dismiss(t *testing.T) {
	items := testItems()
	panel := NewTodoActionPanel(items, 80, 24)

	// Move to second item and dismiss
	panel, _ = panel.Update(tea.KeyPressMsg{Code: -1, Text: "j"})
	panel, _ = panel.Update(tea.KeyPressMsg{Code: -1, Text: "d"})
	require.NotNil(t, panel.Result())
	assert.Equal(t, TodoPanelDismiss, panel.Result().Action)
	assert.Equal(t, "2", panel.Result().Item.ID)
}

func TestTodoActionPanel_Complete(t *testing.T) {
	items := testItems()
	panel := NewTodoActionPanel(items, 80, 24)

	panel, _ = panel.Update(tea.KeyPressMsg{Code: -1, Text: "c"})
	require.NotNil(t, panel.Result())
	assert.Equal(t, TodoPanelComplete, panel.Result().Action)
	assert.Equal(t, "1", panel.Result().Item.ID)
}

func TestTodoActionPanel_Cancel(t *testing.T) {
	items := testItems()
	panel := NewTodoActionPanel(items, 80, 24)

	panel, _ = panel.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.True(t, panel.Cancelled())
}

func TestTodoActionPanel_CancelWithQ(t *testing.T) {
	items := testItems()
	panel := NewTodoActionPanel(items, 80, 24)

	panel, _ = panel.Update(tea.KeyPressMsg{Code: -1, Text: "q"})
	assert.True(t, panel.Cancelled())
}

func TestTodoActionPanel_EmptySelectNoop(t *testing.T) {
	panel := NewTodoActionPanel(nil, 80, 24)

	panel, _ = panel.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Nil(t, panel.Result())

	panel, _ = panel.Update(tea.KeyPressMsg{Code: -1, Text: "d"})
	assert.Nil(t, panel.Result())
}

func TestTodoActionPanel_ViewRenders(t *testing.T) {
	items := testItems()
	panel := NewTodoActionPanel(items, 80, 24)

	view := panel.View()
	assert.Contains(t, view, "TODO Items")
	assert.Contains(t, view, "plan.md")
	assert.Contains(t, view, "Review PR #42")
	assert.Contains(t, view, "notes.md")
}

func TestTodoActionPanel_Overlay(t *testing.T) {
	items := testItems()
	panel := NewTodoActionPanel(items, 80, 24)

	overlay := panel.Overlay("background content", 80, 24)
	assert.NotEmpty(t, overlay)
	assert.Contains(t, overlay, "TODO Items")
}

func TestShortRemote(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://github.com/org/repo.git", "org/repo"},
		{"https://github.com/org/repo", "org/repo"},
		{"git@github.com:org/repo.git", "org/repo"},
		{"git@github.com:org/repo", "org/repo"},
		{"unknown", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, shortRemote(tt.input))
		})
	}
}

func TestTruncate(t *testing.T) {
	assert.Equal(t, "hello", truncate("hello", 10))
	assert.Equal(t, "hel...", truncate("hello world", 6))
	assert.Equal(t, "he", truncate("hello", 2))
}

func TestTodoFormatAge(t *testing.T) {
	assert.Equal(t, "30s", todoFormatAge(time.Now().Add(-30*time.Second)))
	assert.Equal(t, "5m", todoFormatAge(time.Now().Add(-5*time.Minute)))
	assert.Equal(t, "3h", todoFormatAge(time.Now().Add(-3*time.Hour)))
	assert.Equal(t, "2d", todoFormatAge(time.Now().Add(-48*time.Hour)))
	assert.Empty(t, todoFormatAge(time.Time{}))
}
