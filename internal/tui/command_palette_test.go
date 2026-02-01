package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/hay-kot/hive/internal/core/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCommandPalette(t *testing.T) {
	cmds := map[string]config.UserCommand{
		"review": {Sh: "send-claude {{ .Name }} /review", Help: "Send to Claude for review"},
		"open":   {Sh: "open {{ .Path }}"},
	}

	p := NewCommandPalette(cmds, nil, 80, 24)

	require.NotNil(t, p)
	assert.Len(t, p.commands, 2)
	// Commands should be sorted by name
	assert.Equal(t, "open", p.commands[0].Name)
	assert.Equal(t, "review", p.commands[1].Name)
}

func TestCommandPalette_Selection(t *testing.T) {
	cmds := map[string]config.UserCommand{
		"test": {Sh: "echo test"},
	}

	p := NewCommandPalette(cmds, nil, 80, 24)

	// Initially no selection
	_, ok := p.SelectedCommand()
	assert.False(t, ok)
	assert.False(t, p.Cancelled())

	// Press enter to select
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	entry, ok := p.SelectedCommand()
	assert.True(t, ok)
	assert.Equal(t, "test", entry.Name)
}

func TestCommandPalette_Cancel(t *testing.T) {
	cmds := map[string]config.UserCommand{
		"test": {Sh: "echo test"},
	}

	p := NewCommandPalette(cmds, nil, 80, 24)
	assert.False(t, p.Cancelled())

	// Press escape to cancel
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	assert.True(t, p.Cancelled())
}

func TestCommandPalette_View(t *testing.T) {
	cmds := map[string]config.UserCommand{
		"test": {Sh: "echo test", Help: "Test command"},
	}

	p := NewCommandPalette(cmds, nil, 80, 24)
	view := p.View()

	assert.Contains(t, view, "Command Palette")
	assert.Contains(t, view, "test")
}

func TestCommandEntry_FilterValue(t *testing.T) {
	entry := CommandEntry{Name: "my-command", Command: config.UserCommand{Sh: "echo"}}
	assert.Equal(t, "my-command", entry.FilterValue())
}
