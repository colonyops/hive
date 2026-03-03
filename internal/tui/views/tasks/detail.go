package tasks

import (
	"fmt"
	"strings"

	"github.com/colonyops/hive/internal/core/hc"
	"github.com/colonyops/hive/internal/core/styles"
)

// renderDetail renders the center content pane for a selected item.
func renderDetail(item *hc.Item, comments []hc.Comment, width int) string {
	if item == nil {
		return ""
	}

	var b strings.Builder

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
