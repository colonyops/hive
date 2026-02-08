// Package tuitest provides testing utilities for TUI components.
package tuitest

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// StripANSI removes ANSI escape codes and trailing whitespace for cleaner golden files.
// This makes golden files human-readable and less fragile to style changes.
func StripANSI(s string) string {
	s = ansi.Strip(s)
	lines := strings.Split(s, "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimRight(line, " ")
		result = append(result, trimmed)
	}
	return strings.TrimRight(strings.Join(result, "\n"), "\n")
}

// KeyPress creates a key press message for a single rune.
func KeyPress(key rune) tea.Msg {
	return tea.KeyPressMsg(tea.Key{Code: key})
}

// KeyPressString creates a key press message for a string.
// Note: In Bubbletea v2, use individual KeyPress calls for multi-char input.
func KeyPressString(s string) tea.Msg {
	if len(s) > 0 {
		return tea.KeyPressMsg(tea.Key{Code: rune(s[0])})
	}
	return nil
}

// KeyDown creates a down arrow key press message.
func KeyDown() tea.Msg {
	return tea.KeyPressMsg(tea.Key{Code: tea.KeyDown})
}

// KeyUp creates an up arrow key press message.
func KeyUp() tea.Msg {
	return tea.KeyPressMsg(tea.Key{Code: tea.KeyUp})
}

// KeyEnter creates an enter key press message.
func KeyEnter() tea.Msg {
	return tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
}

// WindowSize creates a window size message.
func WindowSize(w, h int) tea.WindowSizeMsg {
	return tea.WindowSizeMsg{Width: w, Height: h}
}
