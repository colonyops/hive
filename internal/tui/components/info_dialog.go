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
	infoModalMaxHeight = 30
	infoModalMargin    = 4
	infoModalChrome    = 6 // title + divider + help + spacing
	infoModalMinWidth  = 50
)

// InfoStatus represents the status of an info item.
type InfoStatus int

const (
	InfoStatusNone InfoStatus = iota
	InfoStatusPass
	InfoStatusWarn
	InfoStatusFail
)

// InfoItem is a single labeled row in an info section.
type InfoItem struct {
	Label  string
	Value  string
	Status InfoStatus
}

// InfoSection groups related info items under a section title.
type InfoSection struct {
	Title string
	Items []InfoItem
}

// InfoDialog displays structured, optionally status-annotated information.
type InfoDialog struct {
	title    string
	sections []InfoSection
	footer   string
	helpText string
	viewport viewport.Model
}

// NewInfoDialog creates a new info dialog.
func NewInfoDialog(title string, sections []InfoSection, footer, helpText string, width, height int) *InfoDialog {
	modalWidth := min(max(int(float64(width)*0.65), infoModalMinWidth), width-infoModalMargin)
	modalHeight := min(height-infoModalMargin, infoModalMaxHeight)
	contentHeight := modalHeight - infoModalChrome

	vp := viewport.New(
		viewport.WithWidth(modalWidth-4),
		viewport.WithHeight(contentHeight),
	)

	d := &InfoDialog{
		title:    title,
		sections: sections,
		footer:   footer,
		helpText: helpText,
		viewport: vp,
	}
	d.viewport.SetContent(d.renderContent(modalWidth))
	return d
}

func (d *InfoDialog) renderContent(modalWidth int) string {
	separator := styles.TextSurfaceStyle.Render(strings.Repeat("─", max(modalWidth-6, 1)))
	lines := make([]string, 0)

	for i, section := range d.sections {
		if i > 0 {
			lines = append(lines, "")
		}
		if section.Title != "" {
			lines = append(lines, styles.HelpDialogSectionStyle.Render(section.Title))
			lines = append(lines, separator)
		}
		for _, item := range section.Items {
			lines = append(lines, formatInfoItem(item))
		}
	}

	if d.footer != "" {
		lines = append(lines, "", d.footer)
	}

	return strings.Join(lines, "\n")
}

func formatInfoItem(item InfoItem) string {
	label := styles.TextForegroundBoldStyle.Render(item.Label)
	value := styles.TextMutedStyle.Render(item.Value)

	if icon := statusIcon(item.Status); icon != "" {
		return fmt.Sprintf("%s %s  %s", icon, label, value)
	}
	return fmt.Sprintf("%s  %s", label, value)
}

func statusIcon(s InfoStatus) string {
	switch s {
	case InfoStatusPass:
		return styles.TextSuccessStyle.Render("✔")
	case InfoStatusWarn:
		return styles.TextWarningStyle.Render("●")
	case InfoStatusFail:
		return styles.TextErrorStyle.Render("✘")
	default:
		return ""
	}
}

// ScrollUp scrolls the viewport up.
func (d *InfoDialog) ScrollUp() {
	d.viewport.ScrollUp(1)
}

// ScrollDown scrolls the viewport down.
func (d *InfoDialog) ScrollDown() {
	d.viewport.ScrollDown(1)
}

// Overlay renders the dialog centered over the provided background.
func (d *InfoDialog) Overlay(background string, width, height int) string {
	modalWidth := min(max(int(float64(width)*0.65), infoModalMinWidth), width-infoModalMargin)
	modalHeight := min(height-infoModalMargin, infoModalMaxHeight)

	scrollInfo := ""
	if d.viewport.TotalLineCount() > d.viewport.VisibleLineCount() {
		scrollInfo = styles.TextMutedStyle.Render(
			fmt.Sprintf(" (%.0f%%)", d.viewport.ScrollPercent()*100),
		)
	}

	divider := styles.TextSurfaceStyle.Render(strings.Repeat("─", max(modalWidth-6, 1)))
	modalContent := lipgloss.JoinVertical(
		lipgloss.Left,
		styles.ModalTitleStyle.Render(d.title+scrollInfo),
		divider,
		d.viewport.View(),
		styles.ModalHelpStyle.Render(d.helpText),
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
