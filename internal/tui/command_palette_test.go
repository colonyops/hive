package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/colonyops/hive/internal/core/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCommandPalette(t *testing.T) {
	cmds := map[string]config.UserCommand{
		"review": {Sh: "send-claude {{ .Name }} /review", Help: "Send to Claude for review"},
		"open":   {Sh: "open {{ .Path }}"},
	}

	p := NewCommandPalette(cmds, nil, 80, 24, ViewSessions)

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

	p := NewCommandPalette(cmds, nil, 80, 24, ViewSessions)

	// Initially no selection
	_, _, ok := p.SelectedCommand()
	assert.False(t, ok)
	assert.False(t, p.Cancelled())

	// Press enter to select
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	entry, args, ok := p.SelectedCommand()
	assert.True(t, ok)
	assert.Equal(t, "test", entry.Name)
	assert.Empty(t, args)
}

func TestCommandPalette_Cancel(t *testing.T) {
	cmds := map[string]config.UserCommand{
		"test": {Sh: "echo test"},
	}

	p := NewCommandPalette(cmds, nil, 80, 24, ViewSessions)
	assert.False(t, p.Cancelled())

	// Press escape to cancel
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	assert.True(t, p.Cancelled())
}

func TestCommandPalette_View(t *testing.T) {
	cmds := map[string]config.UserCommand{
		"test": {Sh: "echo test", Help: "Test command"},
	}

	p := NewCommandPalette(cmds, nil, 80, 24, ViewSessions)
	view := p.View()

	assert.Contains(t, view, "Command Palette")
	assert.Contains(t, view, "test")
	assert.Contains(t, view, "Test command")
}

func TestCommandPalette_ViewWithLongHelpText(t *testing.T) {
	cmds := map[string]config.UserCommand{
		"review": {
			Sh:   "send-claude {{ .Name }} /review",
			Help: "Send to Claude for review with detailed analysis and feedback on code quality",
		},
	}

	p := NewCommandPalette(cmds, nil, 80, 24, ViewSessions)
	view := p.View()

	assert.Contains(t, view, "Command Palette")
	assert.Contains(t, view, "review")
	// Help text should be truncated with "..." if too long
	assert.Contains(t, view, "Send to Claude for review")
	// May be truncated, so just check it's present in some form
	assert.Regexp(t, "Send to Claude for review.*", view)
}

func TestCommandPalette_WithArgs(t *testing.T) {
	cmds := map[string]config.UserCommand{
		"deploy": {Sh: "deploy {{ index .Args 0 }}"},
	}

	p := NewCommandPalette(cmds, nil, 80, 24, ViewSessions)

	// Type command with args
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: 'd', Text: "d"}))
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: 'e', Text: "e"}))
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: 'p', Text: "p"}))
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: 'l', Text: "l"}))
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: 'o', Text: "o"}))
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: 'y', Text: "y"}))
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: ' ', Text: " "}))
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: 's', Text: "s"}))
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: 't', Text: "t"}))
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: 'a', Text: "a"}))
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: 'g', Text: "g"}))
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: 'i', Text: "i"}))
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: 'n', Text: "n"}))
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: 'g', Text: "g"}))

	// Press enter to select
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	entry, args, ok := p.SelectedCommand()
	assert.True(t, ok)
	assert.Equal(t, "deploy", entry.Name)
	assert.Equal(t, []string{"staging"}, args)
}

func TestCommandPalette_Filtering(t *testing.T) {
	cmds := map[string]config.UserCommand{
		"deploy":  {Sh: "deploy"},
		"delete":  {Sh: "delete"},
		"restart": {Sh: "restart"},
	}

	p := NewCommandPalette(cmds, nil, 80, 24, ViewSessions)

	// Initially all commands visible
	assert.Len(t, p.filteredList, 3)

	// Type 'd' - should match deploy and delete (order by fuzzy score)
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: 'd', Text: "d"}))
	assert.Len(t, p.filteredList, 2)
	names := []string{p.filteredList[0].Name, p.filteredList[1].Name}
	assert.ElementsMatch(t, []string{"delete", "deploy"}, names)

	// Type 'e' - should match delete and deploy
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: 'e', Text: "e"}))
	assert.Len(t, p.filteredList, 2)

	// Type 'p' - should match only deploy
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: 'p', Text: "p"}))
	assert.Len(t, p.filteredList, 1)
	assert.Equal(t, "deploy", p.filteredList[0].Name)
}

func TestCommandPalette_FuzzyMatching(t *testing.T) {
	cmds := map[string]config.UserCommand{
		"deploy-staging": {Sh: "deploy-staging"},
		"deploy-prod":    {Sh: "deploy-prod"},
		"delete-session": {Sh: "delete-session"},
		"restart":        {Sh: "restart"},
	}

	p := NewCommandPalette(cmds, nil, 80, 24, ViewSessions)

	// Fuzzy match: 'dpl' should match 'deploy-staging' and 'deploy-prod'
	for _, r := range "dpl" {
		p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
	}
	assert.Len(t, p.filteredList, 2)
	names := []string{p.filteredList[0].Name, p.filteredList[1].Name}
	assert.ElementsMatch(t, []string{"deploy-prod", "deploy-staging"}, names)

	// Reset and try another fuzzy pattern
	p = NewCommandPalette(cmds, nil, 80, 24, ViewSessions)

	// 'dpst' should match 'deploy-staging' (d-p-st)
	for _, r := range "dpst" {
		p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
	}
	assert.Len(t, p.filteredList, 1)
	assert.Equal(t, "deploy-staging", p.filteredList[0].Name)

	// Reset and try 'rst' - should match 'restart'
	p = NewCommandPalette(cmds, nil, 80, 24, ViewSessions)
	for _, r := range "rst" {
		p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
	}
	assert.Len(t, p.filteredList, 1)
	assert.Equal(t, "restart", p.filteredList[0].Name)
}

func TestCommandPalette_Navigation(t *testing.T) {
	cmds := map[string]config.UserCommand{
		"first":  {Sh: "first"},
		"second": {Sh: "second"},
		"third":  {Sh: "third"},
	}

	p := NewCommandPalette(cmds, nil, 80, 24, ViewSessions)

	// Initially at index 0
	assert.Equal(t, 0, p.selectedIdx)

	// Press down - should move to index 1
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	assert.Equal(t, 1, p.selectedIdx)

	// Press down again - should move to index 2
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	assert.Equal(t, 2, p.selectedIdx)

	// Press down again - should stay at index 2 (last)
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	assert.Equal(t, 2, p.selectedIdx)

	// Press up - should move to index 1
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	assert.Equal(t, 1, p.selectedIdx)

	// Press up again - should move to index 0
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	assert.Equal(t, 0, p.selectedIdx)

	// Press up again - should stay at index 0 (first)
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	assert.Equal(t, 0, p.selectedIdx)
}

func TestCommandPalette_MultipleArgs(t *testing.T) {
	cmds := map[string]config.UserCommand{
		"cmd": {Sh: "cmd {{ index .Args 0 }} {{ index .Args 1 }}"},
	}

	p := NewCommandPalette(cmds, nil, 80, 24, ViewSessions)

	// Type: cmd arg1 arg2
	for _, ch := range "cmd arg1 arg2" {
		p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: ch, Text: string(ch)}))
	}

	// Press enter to select
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	entry, args, ok := p.SelectedCommand()
	assert.True(t, ok)
	assert.Equal(t, "cmd", entry.Name)
	assert.Equal(t, []string{"arg1", "arg2"}, args)
}

func TestCommandPalette_NoMatch(t *testing.T) {
	cmds := map[string]config.UserCommand{
		"deploy": {Sh: "deploy"},
	}

	p := NewCommandPalette(cmds, nil, 80, 24, ViewSessions)

	// Type something that doesn't match
	for _, ch := range "xyz" {
		p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: ch, Text: string(ch)}))
	}

	// Should have no filtered results
	assert.Empty(t, p.filteredList)

	// Pressing enter should not select anything
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	_, _, ok := p.SelectedCommand()
	assert.False(t, ok)
}

func TestCommandPalette_TabAutoFill(t *testing.T) {
	cmds := map[string]config.UserCommand{
		"deploy":  {Sh: "deploy"},
		"delete":  {Sh: "delete"},
		"restart": {Sh: "restart"},
	}

	p := NewCommandPalette(cmds, nil, 80, 24, ViewSessions)

	// Type 'd' to filter to deploy and delete
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: 'd', Text: "d"}))
	assert.Len(t, p.filteredList, 2)
	assert.Equal(t, 0, p.selectedIdx)

	// Remember which command is first in fuzzy results
	firstCmd := p.filteredList[0].Name
	secondCmd := p.filteredList[1].Name

	// Press tab - should auto-fill with the first result
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	assert.Equal(t, firstCmd, p.input.Value())

	// Press down to select second command
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	assert.Equal(t, 1, p.selectedIdx)

	// Press tab - should auto-fill with second command
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	assert.Equal(t, secondCmd, p.input.Value())
}

func TestCommandPalette_TabAutoFillPreservesArgs(t *testing.T) {
	cmds := map[string]config.UserCommand{
		"deploy": {Sh: "deploy {{ index .Args 0 }}"},
		"delete": {Sh: "delete {{ index .Args 0 }}"},
	}

	p := NewCommandPalette(cmds, nil, 80, 24, ViewSessions)

	// Type "d staging" to have args
	for _, ch := range "d staging" {
		p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: ch, Text: string(ch)}))
	}

	// Should filter to deploy and delete, with args "staging"
	assert.Len(t, p.filteredList, 2)

	// Remember the fuzzy ordering
	firstCmd := p.filteredList[0].Name
	secondCmd := p.filteredList[1].Name

	// Press tab - should auto-fill command but preserve args
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	assert.Equal(t, firstCmd+" staging", p.input.Value())

	// Navigate down and tab again
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	assert.Equal(t, secondCmd+" staging", p.input.Value())
}

func TestCommandPalette_Scrolling(t *testing.T) {
	// Create more than 12 commands to test scrolling
	cmds := make(map[string]config.UserCommand)
	for i := 1; i <= 20; i++ {
		name := ""
		if i < 10 {
			name = "cmd0" + string(rune('0'+i))
		} else {
			name = "cmd" + string(rune('0'+(i/10))) + string(rune('0'+(i%10)))
		}
		cmds[name] = config.UserCommand{Sh: "echo " + name}
	}

	p := NewCommandPalette(cmds, nil, 80, 24, ViewSessions)

	// Initially at top, no scroll
	assert.Equal(t, 0, p.selectedIdx)
	assert.Equal(t, 0, p.scrollOffset)

	// Navigate down 11 times - should stay at scrollOffset 0
	for i := 0; i < 11; i++ {
		p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	}
	assert.Equal(t, 11, p.selectedIdx)
	assert.Equal(t, 0, p.scrollOffset)

	// Navigate down once more - should scroll
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	assert.Equal(t, 12, p.selectedIdx)
	assert.Equal(t, 1, p.scrollOffset)

	// Continue navigating down
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	assert.Equal(t, 13, p.selectedIdx)
	assert.Equal(t, 2, p.scrollOffset)

	// Navigate to the end
	for i := 0; i < 6; i++ {
		p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	}
	assert.Equal(t, 19, p.selectedIdx)
	assert.Equal(t, 8, p.scrollOffset)

	// Navigate up - scroll doesn't change until we reach top of viewport
	for i := 0; i < 11; i++ {
		p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	}
	// Now at index 8, which is at the top of the visible window [8, 19]
	assert.Equal(t, 8, p.selectedIdx)
	assert.Equal(t, 8, p.scrollOffset)

	// Navigate up once more - should scroll up
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	assert.Equal(t, 7, p.selectedIdx)
	assert.Equal(t, 7, p.scrollOffset)

	// Continue to top
	for i := 0; i < 7; i++ {
		p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	}
	assert.Equal(t, 0, p.selectedIdx)
	assert.Equal(t, 0, p.scrollOffset)
}

func TestCommandPalette_ScrollingWithFiltering(t *testing.T) {
	// Create more than 12 commands
	cmds := make(map[string]config.UserCommand)
	for i := 1; i <= 20; i++ {
		name := ""
		if i < 10 {
			name = "deploy0" + string(rune('0'+i))
		} else {
			name = "deploy" + string(rune('0'+(i/10))) + string(rune('0'+(i%10)))
		}
		cmds[name] = config.UserCommand{Sh: "echo " + name}
	}
	cmds["restart"] = config.UserCommand{Sh: "restart"}

	p := NewCommandPalette(cmds, nil, 80, 24, ViewSessions)

	// Navigate down to scroll
	for i := 0; i < 15; i++ {
		p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	}
	assert.Equal(t, 15, p.selectedIdx)
	assert.Equal(t, 4, p.scrollOffset)

	// Filter commands - should reset scroll
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: 'd', Text: "d"}))
	assert.Equal(t, 0, p.scrollOffset)
	assert.Equal(t, 0, p.selectedIdx)
	// Should have 20 deploy commands
	assert.Len(t, p.filteredList, 20)
}

func TestCommandPalette_ViewWithScrolling(t *testing.T) {
	// Create 15 commands
	cmds := make(map[string]config.UserCommand)
	for i := 1; i <= 15; i++ {
		name := ""
		if i < 10 {
			name = "cmd0" + string(rune('0'+i))
		} else {
			name = "cmd" + string(rune('0'+(i/10))) + string(rune('0'+(i%10)))
		}
		cmds[name] = config.UserCommand{Sh: "echo " + name}
	}

	p := NewCommandPalette(cmds, nil, 80, 24, ViewSessions)

	// View at top should show "... and more"
	view := p.View()
	assert.Contains(t, view, "... and more")
	assert.Contains(t, view, "cmd01")

	// Navigate down past visible window
	for i := 0; i < 13; i++ {
		p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	}

	// Should now be scrolled, showing cmd14 (index 13) and still showing "... and more"
	view = p.View()
	assert.Contains(t, view, "cmd14")
	assert.Contains(t, view, "... and more")

	// Navigate to last item
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))

	// Should not show "... and more" at the end
	view = p.View()
	assert.Contains(t, view, "cmd15")
	assert.NotContains(t, view, "... and more")
}

func TestWrapText(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		maxWidth int
		maxLines int
		expected []string
	}{
		{
			name:     "short text single line",
			text:     "Short text",
			maxWidth: 50,
			maxLines: 2,
			expected: []string{"Short text"},
		},
		{
			name:     "text that wraps to two lines",
			text:     "This is a longer text that should wrap to two lines",
			maxWidth: 30,
			maxLines: 2,
			expected: []string{"This is a longer text that", "should wrap to two lines"},
		},
		{
			name:     "text that exceeds max lines",
			text:     "This is a very long text that will definitely exceed the maximum number of lines allowed and should be truncated",
			maxWidth: 30,
			maxLines: 2,
			expected: []string{"This is a very long text that", "maximum number of lines..."},
		},
		{
			name:     "empty text",
			text:     "",
			maxWidth: 50,
			maxLines: 2,
			expected: nil,
		},
		{
			name:     "single word longer than width",
			text:     "Supercalifragilisticexpialidocious",
			maxWidth: 20,
			maxLines: 2,
			expected: []string{"Supercalifragilisticexpialidocious"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wrapText(tt.text, tt.maxWidth, tt.maxLines)
			assert.Equal(t, tt.expected, result)
		})
	}
}
