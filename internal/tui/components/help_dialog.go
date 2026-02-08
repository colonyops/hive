// Package components provides reusable TUI components.
package components

import (
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/hay-kot/hive/internal/core/styles"
)

// HelpEntry represents a single keyboard shortcut entry.
type HelpEntry struct {
	Key  string
	Desc string
}

// HelpDialogSection groups related help entries under a title.
type HelpDialogSection struct {
	Title   string
	Entries []HelpEntry
}

// HelpDialog displays all available keyboard shortcuts.
type HelpDialog struct {
	title    string
	sections []HelpDialogSection
	width    int
	height   int
}

// NewHelpDialog creates a new help dialog with the given sections.
func NewHelpDialog(title string, sections []HelpDialogSection, width, height int) *HelpDialog {
	return &HelpDialog{
		title:    title,
		sections: sections,
		width:    width,
		height:   height,
	}
}

// View renders the help dialog.
func (h *HelpDialog) View() string {
	title := styles.TextForegroundBoldStyle.Render(h.title)

	var lines []string
	separator := styles.TextMutedStyle.Render("─────────────────────────")

	for i, section := range h.sections {
		// Add section header if present
		if section.Title != "" {
			// Add spacing before subsequent sections
			if i > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, styles.HelpDialogSectionStyle.Render(section.Title))
			lines = append(lines, separator)
		}

		for _, entry := range section.Entries {
			lines = append(lines, formatKeyDesc(entry.Key, entry.Desc))
		}
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		strings.Join(lines, "\n"),
	)

	help := styles.HelpDialogHelpStyle.Render("esc/? close")
	content = lipgloss.JoinVertical(lipgloss.Left, content, help)

	return styles.HelpDialogModalStyle.Render(content)
}

// Overlay renders the help dialog as a layer over the given background.
func (h *HelpDialog) Overlay(background string, width, height int) string {
	modal := h.View()

	bgLayer := lipgloss.NewLayer(background)
	modalLayer := lipgloss.NewLayer(modal)

	// Center the modal
	modalW := lipgloss.Width(modal)
	modalH := lipgloss.Height(modal)
	centerX := (width - modalW) / 2
	centerY := (height - modalH) / 2
	modalLayer.X(centerX).Y(centerY).Z(1)

	compositor := lipgloss.NewCompositor(bgLayer, modalLayer)
	return compositor.Render()
}

// formatKeyDesc formats a key-description pair with consistent alignment.
func formatKeyDesc(key, desc string) string {
	const keyWidth = 12

	// Pad key to fixed width for alignment using display width (handles Unicode)
	displayWidth := lipgloss.Width(key)
	paddedKey := key + Pad(keyWidth-displayWidth)

	return styles.TextPrimaryBoldStyle.Render(paddedKey) + styles.TextForegroundStyle.Render(desc)
}
