package tasks

import (
	"fmt"
	"strings"
	"time"

	lipgloss "charm.land/lipgloss/v2"

	"github.com/colonyops/hive/internal/core/hc"
	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/tui/views/shared"
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

// renderDetailContent renders the scrollable content: title + description + blockers + comments.
func renderDetailContent(item *hc.Item, comments []hc.Comment, blockers []hc.Item, width int) string {
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
		b.WriteString(shared.RenderMarkdown(item.Desc, width))
	} else {
		b.WriteString("\n")
		b.WriteString(styles.TextMutedStyle.Render("No description"))
	}

	// Blockers section
	if len(blockers) > 0 {
		b.WriteString("\n")
		b.WriteString(styles.TextMutedStyle.Render(fmt.Sprintf("Blockers (%d)", len(blockers))))
		b.WriteString("\n")
		for _, blocker := range blockers {
			icon := StatusIcon(blocker.Status)
			fmt.Fprintf(&b, "  %s %s  %s\n",
				icon,
				styles.TextForegroundStyle.Render(blocker.Title),
				styles.TextMutedStyle.Render("("+blocker.ID+")"))
		}
	} else if item.Blocked {
		// item is blocked by open children (not explicit blockers)
		b.WriteString("\n")
		b.WriteString(styles.TextMutedStyle.Render("Blocked by open children"))
		b.WriteString("\n")
	}

	b.WriteString("\n")

	if len(comments) == 0 {
		b.WriteString("\n")
		b.WriteString(styles.TextMutedStyle.Render("No comments"))
	} else {
		for _, c := range comments {
			b.WriteString("\n")

			ts := relativeTime(c.CreatedAt)
			isCheckpoint := strings.HasPrefix(c.Message, "CHECKPOINT")

			// Header: — <time> [· CHECKPOINT]
			if isCheckpoint {
				fmt.Fprintf(&b, "%s %s\n",
					styles.TextMutedStyle.Render(styles.IconComment+" "+ts+" ·"),
					styles.TextWarningStyle.Render("CHECKPOINT"))
			} else {
				b.WriteString(styles.TextMutedStyle.Render(styles.IconComment+" "+ts) + "\n")
			}

			// Body: indented 2 spaces
			bodyWidth := max(width-2, 10)
			msg := c.Message
			if checkpoint, ok := strings.CutPrefix(msg, "CHECKPOINT:"); ok {
				msg = strings.TrimSpace(checkpoint)
			}
			rendered := shared.RenderMarkdown(msg, bodyWidth)
			for _, line := range strings.Split(rendered, "\n") {
				fmt.Fprintf(&b, "   %s\n", line)
			}
		}
	}

	return b.String()
}

// renderHeader renders a compact properties header bar for the detail pane.
func renderHeader(item *hc.Item, node *TreeNode) string {
	var parts []string

	// Type badge with background — epic uses primary, task uses surface
	badgeBg := styles.ColorSurface
	badgeFg := styles.ColorForeground
	if item.Type == hc.ItemTypeEpic {
		badgeBg = styles.ColorPrimary
		badgeFg = styles.ColorBackground
	}

	typeBadge := lipgloss.NewStyle().
		Background(badgeBg).
		Foreground(badgeFg).
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
