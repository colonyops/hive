package tasks

import (
	"fmt"
	"strings"
	"time"

	"github.com/colonyops/hive/internal/core/hc"
	"github.com/colonyops/hive/internal/core/styles"
)

// renderProperties renders the right sidebar for a selected item.
func renderProperties(item *hc.Item, node *TreeNode) string {
	if item == nil {
		return ""
	}

	var b strings.Builder

	// Type
	writeProperty(&b, "Type", string(item.Type))

	// Status with icon
	icon := StatusIcon(item.Status, item.Blocked)
	writeProperty(&b, "Status", icon+" "+string(item.Status))

	// ID (truncated)
	id := item.ID
	if len(id) > 12 {
		id = id[:12]
	}
	writeProperty(&b, "ID", id)

	// Created
	writeProperty(&b, "Created", relativeTime(item.CreatedAt))

	// Updated
	writeProperty(&b, "Updated", relativeTime(item.UpdatedAt))

	// Type-specific fields
	if item.Type == hc.ItemTypeEpic && node != nil {
		b.WriteString("\n")
		b.WriteString(styles.TextForegroundBoldStyle.Render("Progress"))
		b.WriteString("\n")

		done, total := countByStatus(node.Children)
		open := 0
		active := 0
		cancelled := 0
		countStatuses(node.Children, &open, &active, &cancelled)

		writeProperty(&b, "Done", fmt.Sprintf("%d/%d", done, total))
		writeProperty(&b, "Open", fmt.Sprintf("%d", open))
		writeProperty(&b, "Active", fmt.Sprintf("%d", active))
		writeProperty(&b, "Cancelled", fmt.Sprintf("%d", cancelled))
	} else if item.Type == hc.ItemTypeTask {
		if item.SessionID != "" {
			b.WriteString("\n")
			sid := item.SessionID
			if len(sid) > 12 {
				sid = sid[:12]
			}
			writeProperty(&b, "Session", sid)
		}
		if item.EpicID != "" {
			writeProperty(&b, "Epic", item.EpicID)
		}
	}

	return b.String()
}

// writeProperty writes a label: value pair.
func writeProperty(b *strings.Builder, label, value string) {
	b.WriteString(styles.TextMutedStyle.Render(label + ":"))
	b.WriteString(" ")
	b.WriteString(styles.TextForegroundStyle.Render(value))
	b.WriteString("\n")
}

// countStatuses counts open, active, and cancelled leaf descendants.
func countStatuses(children []*TreeNode, open, active, cancelled *int) {
	for _, child := range children {
		if len(child.Children) > 0 {
			countStatuses(child.Children, open, active, cancelled)
		} else {
			switch child.Item.Status {
			case hc.StatusOpen:
				*open++
			case hc.StatusInProgress:
				*active++
			case hc.StatusCancelled:
				*cancelled++
			case hc.StatusDone:
				// counted by countByStatus
			}
		}
	}
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
