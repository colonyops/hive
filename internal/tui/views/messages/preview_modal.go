package messages

import (
	"fmt"
	"regexp"
	"strings"

	"charm.land/bubbles/v2/viewport"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/glamour"
	"github.com/rs/zerolog/log"

	"github.com/colonyops/hive/internal/core/messaging"
	"github.com/colonyops/hive/internal/core/styles"
)

const (
	iconDot               = "•"
	unknownSender         = "unknown"
	previewModalMaxWidth  = 100
	previewModalMaxHeight = 30
	previewModalMargin    = 4
	previewModalChrome    = 8
	previewModalPadding   = 4
)

// PreviewModal displays a message with markdown rendering.
type PreviewModal struct {
	message    messaging.Message
	viewport   viewport.Model
	copyStatus string
}

// NewPreviewModal creates a new preview modal for the given message.
func NewPreviewModal(msg messaging.Message, width, height int) PreviewModal {
	modalWidth := min(width-previewModalMargin, previewModalMaxWidth)
	modalHeight := min(height-previewModalMargin, previewModalMaxHeight)
	contentHeight := modalHeight - previewModalChrome

	vp := viewport.New(
		viewport.WithWidth(modalWidth-previewModalPadding),
		viewport.WithHeight(contentHeight),
	)

	m := PreviewModal{
		message:  msg,
		viewport: vp,
	}

	m.renderContent(modalWidth - previewModalPadding)
	return m
}

func (m *PreviewModal) renderContent(width int) {
	style := styles.GlamourStyle()
	noMargin := uint(0)
	style.Document.Margin = &noMargin

	renderer, err := glamour.NewTermRenderer(
		glamour.WithStyles(style),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		log.Debug().Err(err).Msg("failed to create markdown renderer, showing raw content")
		m.viewport.SetContent(m.message.Payload)
		return
	}

	rendered, err := renderer.Render(m.message.Payload)
	if err != nil {
		log.Debug().Err(err).Msg("failed to render markdown, showing raw content")
		m.viewport.SetContent(m.message.Payload)
		return
	}

	content := strings.TrimSpace(rendered)
	content = stripLeadingDecorative(content)
	content = stripTrailingDecorative(content)
	m.viewport.SetContent(content)
}

// UpdateViewport updates the viewport with a message (for scrolling).
func (m *PreviewModal) UpdateViewport(msg any) {
	m.viewport, _ = m.viewport.Update(msg)
}

// ScrollUp scrolls the viewport up.
func (m *PreviewModal) ScrollUp() {
	m.viewport.ScrollUp(1)
}

// ScrollDown scrolls the viewport down.
func (m *PreviewModal) ScrollDown() {
	m.viewport.ScrollDown(1)
}

// Payload returns the raw message payload for copying.
func (m *PreviewModal) Payload() string {
	return m.message.Payload
}

// SetCopyStatus sets the copy feedback message.
func (m *PreviewModal) SetCopyStatus(status string) {
	m.copyStatus = status
}

// ClearCopyStatus clears the copy feedback message.
func (m *PreviewModal) ClearCopyStatus() {
	m.copyStatus = ""
}

// Overlay renders the preview modal centered over the background.
func (m PreviewModal) Overlay(background string, width, height int) string {
	modalWidth := min(width-previewModalMargin, previewModalMaxWidth)
	modalHeight := min(height-previewModalMargin, previewModalMaxHeight)

	sender := m.message.Sender
	if sender == "" {
		sender = unknownSender
	}
	topicStr := styles.PreviewTopicStyle.Render(fmt.Sprintf("[%s]", m.message.Topic))
	senderStr := styles.TextSuccessStyle.Render(sender)
	timeStr := styles.TextMutedStyle.Render(m.message.CreatedAt.Format("2006-01-02 15:04:05"))
	metadata := fmt.Sprintf("%s %s %s %s", topicStr, senderStr, iconDot, timeStr)

	if m.message.SessionID != "" {
		sessionStr := styles.PreviewSessionStyle.Render(fmt.Sprintf("session: %s", m.message.SessionID))
		metadata = fmt.Sprintf("%s\n%s", metadata, sessionStr)
	}

	scrollInfo := ""
	if m.viewport.TotalLineCount() > m.viewport.VisibleLineCount() {
		scrollInfo = styles.TextMutedStyle.Render(fmt.Sprintf(" (%.0f%%)", m.viewport.ScrollPercent()*100))
	}

	helpText := "[↑/↓/j/k] scroll  [c] copy  [enter/esc] close"
	if m.copyStatus != "" {
		helpText = styles.TextSuccessStyle.Render(m.copyStatus)
	}

	divider := styles.TextSurfaceStyle.Render("────────────────────────────────────────")
	modalContent := lipgloss.JoinVertical(
		lipgloss.Left,
		styles.ModalTitleStyle.Render("Message Preview"+scrollInfo),
		"",
		metadata,
		divider,
		m.viewport.View(),
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
	centerX := (width - modalW) / 2
	centerY := (height - modalH) / 2
	modalLayer.X(centerX).Y(centerY).Z(1)

	compositor := lipgloss.NewCompositor(bgLayer, modalLayer)
	return compositor.Render()
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func isDecorativeLine(line string) bool {
	stripped := ansiPattern.ReplaceAllString(line, "")
	stripped = strings.TrimSpace(stripped)
	if stripped == "" {
		return true
	}
	for _, r := range stripped {
		if r != '─' && r != '━' && r != '-' && r != '=' {
			return false
		}
	}
	return true
}

func stripLeadingDecorative(content string) string {
	lines := strings.Split(content, "\n")
	start := 0
	for start < len(lines) && isDecorativeLine(lines[start]) {
		start++
	}
	if start > 0 {
		return strings.Join(lines[start:], "\n")
	}
	return content
}

func stripTrailingDecorative(content string) string {
	lines := strings.Split(content, "\n")
	end := len(lines)
	for end > 0 && isDecorativeLine(lines[end-1]) {
		end--
	}
	if end < len(lines) {
		return strings.Join(lines[:end], "\n")
	}
	return content
}
