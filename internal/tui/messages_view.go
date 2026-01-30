package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/hay-kot/hive/internal/core/messaging"
)

// MessageItem wraps a message for the list component.
type MessageItem struct {
	Message messaging.Message
}

// FilterValue returns the value used for filtering.
func (i MessageItem) FilterValue() string {
	return fmt.Sprintf("%s %s %s", i.Message.Topic, i.Message.Sender, i.Message.Payload)
}

// MessageDelegate handles rendering of message items in the list.
type MessageDelegate struct {
	Styles  MessageDelegateStyles
	Focused bool // Whether this pane is focused (controls selection bar visibility)
}

// MessageDelegateStyles defines the styles for the message delegate.
type MessageDelegateStyles struct {
	Normal   lipgloss.Style
	Selected lipgloss.Style
	Topic    lipgloss.Style
	Sender   lipgloss.Style
	Time     lipgloss.Style
	Payload  lipgloss.Style
	Session  lipgloss.Style
}

// DefaultMessageDelegateStyles returns the default styles.
func DefaultMessageDelegateStyles() MessageDelegateStyles {
	return MessageDelegateStyles{
		Normal:   normalStyle,
		Selected: selectedStyle,
		Topic:    msgTopicStyle,
		Sender:   msgSenderStyle,
		Time:     msgTimestampStyle,
		Payload:  msgPayloadStyle,
		Session:  msgSessionStyle,
	}
}

// NewMessageDelegate creates a new message delegate with default styles.
func NewMessageDelegate() MessageDelegate {
	return MessageDelegate{
		Styles: DefaultMessageDelegateStyles(),
	}
}

// Height returns the height of each item.
func (d MessageDelegate) Height() int {
	return 2
}

// Spacing returns the spacing between items.
func (d MessageDelegate) Spacing() int {
	return 1
}

// Update handles item updates.
func (d MessageDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}

// Render renders a single message item.
// Line 1: [Topic] Sender • 15:04:05
// Line 2: Payload (truncated to fit)
func (d MessageDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	msgItem, ok := item.(MessageItem)
	if !ok {
		return
	}

	msg := msgItem.Message
	isSelected := index == m.Index()
	width := m.Width()
	if width <= 0 {
		width = 80
	}

	// Get available width for content (accounting for border)
	contentWidth := width - 4

	// Determine styles based on selection
	var nameStyle lipgloss.Style
	if isSelected {
		nameStyle = d.Styles.Selected
	} else {
		nameStyle = d.Styles.Normal
	}

	// Line 1: [Topic] Sender • 15:04:05
	topicTag := d.Styles.Topic.Render(fmt.Sprintf("[%s]", msg.Topic))
	sender := msg.Sender
	if sender == "" {
		sender = "unknown"
	}
	senderStr := d.Styles.Sender.Render(sender)
	timeStr := d.Styles.Time.Render(msg.CreatedAt.Format("15:04:05"))
	line1 := fmt.Sprintf("%s %s %s %s", topicTag, senderStr, iconDot, timeStr)

	// Line 2: Payload (truncated)
	payload := strings.ReplaceAll(msg.Payload, "\n", " ")
	payloadRunes := []rune(payload)
	if len(payloadRunes) > contentWidth {
		payload = string(payloadRunes[:contentWidth-3]) + "..."
	}
	line2 := d.Styles.Payload.Render(payload)

	// Apply selection styling with left border (only in focused pane)
	var border string
	if isSelected && d.Focused {
		border = selectedBorderStyle.Render("┃") + " "
	} else {
		border = "  "
	}

	// Write to output with left border
	_, _ = fmt.Fprintf(w, "%s%s\n", border, nameStyle.Render(line1))
	_, _ = fmt.Fprintf(w, "%s%s", border, line2)
}
