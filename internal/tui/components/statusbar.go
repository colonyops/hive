package components

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/colonyops/hive/internal/core/styles"
)

// Common help hint fragments for StatusBar footers. Combine with HelpSep to
// build context-specific help lines while keeping wording consistent.
const (
	HelpNav    = "j/k navigate"
	HelpFilter = "/ filter"
	HelpHelp   = "? help"
	HelpSep    = " • "
)

// StatusBar renders full-width bar lines. Callers pre-style their own text;
// the bar only handles padding, fill, and layout.
type StatusBar struct {
	Width int
}

// Render produces a single padded line. left is typically help/keybinding
// text; right is optional metadata (may be empty).
func (s StatusBar) Render(left, right string) string {
	style := styles.StatusBarStyle.Width(s.Width)

	if right == "" {
		return style.Render(left)
	}

	leftWidth := ansi.StringWidth(left)
	rightWidth := ansi.StringWidth(right)

	// PaddingLeft(1) and PaddingRight(1) consume 2 chars of the total width.
	innerWidth := s.Width - 2
	gap := innerWidth - leftWidth - rightWidth
	if gap < 1 {
		gap = 1
	}

	content := left + strings.Repeat(" ", gap) + right
	return style.Render(content)
}

// Rule renders a full-width horizontal rule using a thin line character.
func (s StatusBar) Rule() string {
	if s.Width <= 0 {
		return ""
	}
	return styles.TextMutedStyle.Render(strings.Repeat("─", s.Width))
}
