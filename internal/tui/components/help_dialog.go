// Package components provides reusable TUI components.
package components

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/colonyops/hive/internal/core/styles"
)

const (
	helpModalMaxHeight = 30
	helpModalMargin    = 4
	helpModalChrome    = 6 // title + divider + help + spacing
	helpModalMinWidth  = 45
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
	viewport viewport.Model
}

// NewHelpDialog creates a new help dialog with the given sections.
func NewHelpDialog(title string, sections []HelpDialogSection, width, height int) *HelpDialog {
	modalWidth := min(max(int(float64(width)*0.5), helpModalMinWidth), width-helpModalMargin)
	modalHeight := min(height-helpModalMargin, helpModalMaxHeight)
	contentHeight := modalHeight - helpModalChrome

	vp := viewport.New(
		viewport.WithWidth(modalWidth-4),
		viewport.WithHeight(contentHeight),
	)

	d := &HelpDialog{
		title:    title,
		sections: sections,
		width:    width,
		height:   height,
		viewport: vp,
	}
	d.viewport.SetContent(d.renderContent(modalWidth))
	return d
}

func (d *HelpDialog) renderContent(modalWidth int) string {
	separator := styles.TextSurfaceStyle.Render(strings.Repeat("─", max(modalWidth-6, 1)))
	var lines []string

	for i, section := range d.sections {
		if section.Title != "" {
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

	return strings.Join(lines, "\n")
}

// ScrollUp scrolls the viewport up.
func (d *HelpDialog) ScrollUp() {
	d.viewport.ScrollUp(1)
}

// ScrollDown scrolls the viewport down.
func (d *HelpDialog) ScrollDown() {
	d.viewport.ScrollDown(1)
}

// Overlay renders the help dialog as a layer over the given background.
func (d *HelpDialog) Overlay(background string, width, height int) string {
	modalWidth := min(max(int(float64(width)*0.5), helpModalMinWidth), width-helpModalMargin)
	modalHeight := min(height-helpModalMargin, helpModalMaxHeight)

	scrollInfo := ""
	if d.viewport.TotalLineCount() > d.viewport.VisibleLineCount() {
		scrollInfo = styles.TextMutedStyle.Render(
			fmt.Sprintf(" (%.0f%%)", d.viewport.ScrollPercent()*100),
		)
	}

	divider := styles.TextSurfaceStyle.Render(strings.Repeat("─", max(modalWidth-6, 1)))

	helpText := "esc/? close"
	if d.viewport.TotalLineCount() > d.viewport.VisibleLineCount() {
		helpText = "[j/k] scroll  esc/? close"
	}

	modalContent := lipgloss.JoinVertical(
		lipgloss.Left,
		styles.ModalTitleStyle.Render(d.title+scrollInfo),
		divider,
		d.viewport.View(),
		styles.ModalHelpStyle.Render(helpText),
	)

	modal := styles.ModalStyle.
		Width(modalWidth).
		Height(modalHeight).
		Render(modalContent)

	bgLayer := lipgloss.NewLayer(background)
	modalLayer := lipgloss.NewLayer(modal)

	modalW := lipgloss.Width(modal)
	modalH := lipgloss.Height(modal)
	centerX := max((width-modalW)/2, 0)
	centerY := max((height-modalH)/2, 0)
	modalLayer.X(centerX).Y(centerY).Z(1)

	return lipgloss.NewCompositor(bgLayer, modalLayer).Render()
}

// formatKeyDesc formats a key-description pair with consistent alignment.
func formatKeyDesc(key, desc string) string {
	const keyWidth = 10

	// Pad key to fixed width for alignment using display width (handles Unicode)
	displayWidth := lipgloss.Width(key)
	paddedKey := key + Pad(keyWidth-displayWidth)

	return styles.TextPrimaryBoldStyle.Render(paddedKey) + styles.TextForegroundStyle.Render(desc)
}
