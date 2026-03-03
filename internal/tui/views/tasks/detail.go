package tasks

import (
	"fmt"
	"strings"
	"time"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/glamour"
	"github.com/rs/zerolog/log"

	"github.com/colonyops/hive/internal/core/hc"
	"github.com/colonyops/hive/internal/core/styles"
)

// renderDetailHeader renders the static header portion above the viewport:
// properties bar + divider.
func renderDetailHeader(item *hc.Item, node *TreeNode, width int) string {
	if item == nil {
		return ""
	}

	var b strings.Builder

	b.WriteString(renderHeader(item, node))
	b.WriteString("\n")
	b.WriteString(styles.TextMutedStyle.Render(strings.Repeat("─", max(width, 1))))

	return b.String()
}

// renderDetailContent renders the scrollable content: title + description + comments.
func renderDetailContent(item *hc.Item, comments []hc.Comment, width int) string {
	if item == nil {
		return ""
	}

	var b strings.Builder

	// Title
	b.WriteString(styles.TextForegroundBoldStyle.Render(item.Title))
	b.WriteString("\n")

	// Description (rendered as markdown via glamour)
	if item.Desc != "" {
		b.WriteString("\n")
		b.WriteString(renderMarkdown(item.Desc, width))
	} else {
		b.WriteString("\n")
		b.WriteString(styles.TextMutedStyle.Render("No description"))
	}

	// Comments divider
	b.WriteString("\n\n")
	b.WriteString(styles.TextMutedStyle.Render("─── Comments ──────"))
	b.WriteString("\n")

	if len(comments) == 0 {
		b.WriteString("\n")
		b.WriteString(styles.TextMutedStyle.Render("No comments"))
	} else {
		pipe := styles.TextMutedStyle.Render("│")
		for _, c := range comments {
			b.WriteString("\n")

			ts := relativeTime(c.CreatedAt)
			isCheckpoint := strings.HasPrefix(c.Message, "CHECKPOINT")

			// Thread header: ┌─ <time> [CHECKPOINT]
			if isCheckpoint {
				fmt.Fprintf(&b, "%s %s  %s\n",
					styles.TextMutedStyle.Render("┌─"),
					styles.TextMutedStyle.Render(ts),
					styles.TextWarningStyle.Render("CHECKPOINT"))
			} else {
				b.WriteString(styles.TextMutedStyle.Render("┌─ "+ts) + "\n")
			}

			// Thread body: │  <message lines>
			// "│  " is 3 visual chars
			bodyWidth := max(width-3, 10)
			msg := c.Message
			if checkpoint, ok := strings.CutPrefix(msg, "CHECKPOINT:"); ok {
				msg = strings.TrimSpace(checkpoint)
			}
			wrapped := lipgloss.Wrap(msg, bodyWidth, "")
			for _, line := range strings.Split(wrapped, "\n") {
				fmt.Fprintf(&b, "%s  %s\n", pipe, line)
			}
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
		PaddingLeft(1).
		PaddingRight(1).
		Render(strings.ToTitle(string(item.Type)[:1]) + string(item.Type)[1:])
	parts = append(parts, typeBadge)

	// Status with icon
	icon := StatusIcon(item.Status)
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

// renderMarkdown renders text as styled markdown using glamour.
func renderMarkdown(text string, width int) string {
	style := styles.GlamourStyle()
	noMargin := uint(0)
	style.Document.Margin = &noMargin

	renderer, err := glamour.NewTermRenderer(
		glamour.WithStyles(style),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		log.Debug().Err(err).Msg("tasks: failed to create markdown renderer")
		return text
	}

	rendered, err := renderer.Render(text)
	if err != nil {
		log.Debug().Err(err).Msg("tasks: failed to render markdown")
		return text
	}

	return strings.TrimSpace(rendered)
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
