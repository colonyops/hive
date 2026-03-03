package tasks

import (
	"fmt"
	"strings"
	"time"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/colonyops/hive/internal/core/hc"
	"github.com/colonyops/hive/internal/core/styles"
)

// renderDetail renders the detail pane for a selected item, including
// a compact header bar with properties and the item content below.
func renderDetail(item *hc.Item, node *TreeNode, comments []hc.Comment, width int) string {
	if item == nil {
		return ""
	}

	var b strings.Builder

	// Header: properties bar
	b.WriteString(renderHeader(item, node))
	b.WriteString("\n")
	b.WriteString(styles.TextMutedStyle.Render(strings.Repeat("─", max(width, 1))))
	b.WriteString("\n")

	// Title
	b.WriteString(styles.TextForegroundBoldStyle.Render(item.Title))
	b.WriteString("\n")

	// Description
	if item.Desc != "" {
		b.WriteString("\n")
		b.WriteString(wrapText(item.Desc, width))
	} else {
		b.WriteString("\n")
		b.WriteString(styles.TextMutedStyle.Render("No description"))
	}

	// Comments divider
	b.WriteString("\n\n")
	divider := styles.TextMutedStyle.Render("─── Comments ──────")
	b.WriteString(divider)
	b.WriteString("\n")

	if len(comments) == 0 {
		b.WriteString("\n")
		b.WriteString(styles.TextMutedStyle.Render("No comments"))
	} else {
		for _, c := range comments {
			b.WriteString("\n")
			ts := styles.TextMutedStyle.Render(relativeTime(c.CreatedAt))

			msg := c.Message
			if checkpoint, ok := strings.CutPrefix(msg, "CHECKPOINT:"); ok {
				prefix := styles.TextWarningStyle.Render("CHECKPOINT:")
				msg = prefix + checkpoint
			}

			fmt.Fprintf(&b, "%s  %s", ts, msg)
		}
	}

	return b.String()
}

// renderHeader renders a compact properties header bar for the detail pane.
func renderHeader(item *hc.Item, node *TreeNode) string {
	var parts []string

	// Type badge with background
	typeBadge := lipgloss.NewStyle().
		Background(styles.ColorSurface).
		Foreground(styles.ColorForeground).
		Bold(true).
		PaddingRight(1).
		Render(strings.ToTitle(string(item.Type)[:1]) + string(item.Type)[1:])
	parts = append(parts, typeBadge)

	// Status with icon
	icon := StatusIcon(item.Status, item.Blocked)
	parts = append(parts, icon+" "+string(item.Status))

	// ID (truncated)
	id := item.ID
	if len(id) > 12 {
		id = id[:12]
	}
	parts = append(parts, styles.TextMutedStyle.Render(id))

	// Type-specific: epic progress or task session
	if item.Type == hc.ItemTypeEpic && node != nil {
		done, total := countByStatus(node.Children)
		parts = append(parts, styles.TextMutedStyle.Render(fmt.Sprintf("[%d/%d]", done, total)))
	} else if item.Type == hc.ItemTypeTask && item.SessionID != "" {
		sid := item.SessionID
		if len(sid) > 12 {
			sid = sid[:12]
		}
		assignStyle := lipgloss.NewStyle().Foreground(styles.ColorMuted).Italic(true)
		parts = append(parts, assignStyle.Render("→ "+sid))
	}

	return strings.Join(parts, "  ")
}

// wrapText wraps text to the given width using simple word wrapping.
func wrapText(text string, width int) string {
	if width <= 0 {
		return text
	}

	var result strings.Builder
	for paragraph := range strings.SplitSeq(text, "\n") {
		if result.Len() > 0 {
			result.WriteString("\n")
		}

		words := strings.Fields(paragraph)
		if len(words) == 0 {
			continue
		}

		lineLen := 0
		for i, word := range words {
			wLen := len(word)
			switch {
			case i == 0:
				result.WriteString(word)
				lineLen = wLen
			case lineLen+1+wLen > width:
				result.WriteString("\n")
				result.WriteString(word)
				lineLen = wLen
			default:
				result.WriteString(" ")
				result.WriteString(word)
				lineLen += 1 + wLen
			}
		}
	}
	return result.String()
}

// relativeTime formats a time as a human-readable relative duration.
func relativeTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}

	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		return fmt.Sprintf("%dh ago", h)
	default:
		days := int(d.Hours() / 24)
		return fmt.Sprintf("%dd ago", days)
	}
}
