package tasks

import (
	"fmt"
	"strings"

	"github.com/colonyops/hive/internal/core/hc"
	"github.com/colonyops/hive/internal/core/styles"
)

// Tree connector characters.
const (
	treeBranch = "├─"
	treeLast   = "└─"
)

// renderTree renders flattened tree nodes into styled strings.
func renderTree(flatNodes []FlatNode, cursor, scrollOffset, viewHeight int) string {
	if len(flatNodes) == 0 {
		return styles.TextMutedStyle.Render("  No matching tasks")
	}

	var b strings.Builder

	end := min(scrollOffset+viewHeight, len(flatNodes))

	for i := scrollOffset; i < end; i++ {
		fn := flatNodes[i]
		isSelected := i == cursor

		// Selection indicator
		var prefix string
		if isSelected {
			prefix = styles.TextPrimaryStyle.Render("┃") + " "
		} else {
			prefix = "  "
		}

		line := renderNode(fn, isSelected)
		b.WriteString(prefix + line)
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

// renderNode renders a single tree node.
func renderNode(fn FlatNode, isSelected bool) string {
	node := fn.Node

	if node.Item.Type == hc.ItemTypeEpic && fn.Depth == 0 {
		return renderEpicNode(fn, isSelected)
	}

	return renderTaskNode(fn, isSelected)
}

// renderEpicNode renders an epic (root) node with expand/collapse icon and progress.
func renderEpicNode(fn FlatNode, isSelected bool) string {
	node := fn.Node

	// Expand/collapse icon
	var expandIcon string
	if node.Expanded {
		expandIcon = IconExpanded
	} else {
		expandIcon = IconCollapsed
	}
	expandIcon = styles.TextMutedStyle.Render(expandIcon)

	// Title
	titleStyle := styles.TextForegroundBoldStyle
	if isSelected {
		titleStyle = styles.TextPrimaryBoldStyle
	}
	title := titleStyle.Render(node.Item.Title)

	// Status icon
	icon := StatusIcon(node.Item.Status)

	// Progress count
	done, total := countByStatus(node.Children)
	progress := styles.TextMutedStyle.Render(fmt.Sprintf("[%d/%d]", done, total))

	return fmt.Sprintf("%s %s %s %s", expandIcon, icon, title, progress)
}

// renderTaskNode renders a task (child) node with tree connectors and status icon.
func renderTaskNode(fn FlatNode, isSelected bool) string {
	node := fn.Node

	// Build indent with tree connectors
	indent := strings.Repeat("  ", fn.Depth)

	// Tree connector
	var connector string
	if fn.IsLast {
		connector = treeLast
	} else {
		connector = treeBranch
	}
	connector = styles.TextMutedStyle.Render(connector)

	// Status icon
	icon := StatusIcon(node.Item.Status)

	// Title
	titleStyle := styles.TextForegroundStyle
	if isSelected {
		titleStyle = styles.TextPrimaryStyle
	}
	if node.Item.Status == hc.StatusDone {
		titleStyle = styles.TextMutedStyle
	}
	title := titleStyle.Render(node.Item.Title)

	// If this is an epic at depth > 0 (nested epic), show expand/collapse and progress
	if node.Item.Type == hc.ItemTypeEpic {
		var expandIcon string
		if node.Expanded {
			expandIcon = IconExpanded
		} else {
			expandIcon = IconCollapsed
		}
		expandIcon = styles.TextMutedStyle.Render(expandIcon)

		nestedIcon := StatusIcon(node.Item.Status)
		done, total := countByStatus(node.Children)
		progress := styles.TextMutedStyle.Render(fmt.Sprintf("[%d/%d]", done, total))
		return fmt.Sprintf("%s%s %s %s %s %s", indent, connector, expandIcon, nestedIcon, title, progress)
	}

	line := fmt.Sprintf("%s%s %s %s", indent, connector, icon, title)
	if node.Item.Blocked {
		line += " " + styles.TextWarningStyle.Render(BadgeBlocked)
	}
	return line
}
