package review

import (
	"fmt"
	"strings"

	"github.com/colonyops/hive/internal/core/styles"
)

// renderDocTree renders the flattened document tree into a styled string.
// It mirrors the tasks renderTree function.
func renderDocTree(flatNodes []DocFlatNode, cursor, scrollOffset, viewHeight int) string {
	if len(flatNodes) == 0 {
		return styles.TextMutedStyle.Render("  No documents found")
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

		line := renderDocNode(fn, isSelected)
		b.WriteString(prefix + line)
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

// renderDocNode renders a single document tree node.
func renderDocNode(fn DocFlatNode, isSelected bool) string {
	node := fn.Node

	if node.Doc == nil {
		// Directory node
		return renderDocDirNode(fn, isSelected)
	}

	// File node
	return renderDocFileNode(fn, isSelected)
}

// renderDocDirNode renders a directory node with expand/collapse icon.
func renderDocDirNode(fn DocFlatNode, isSelected bool) string {
	node := fn.Node

	// Expand/collapse icon
	var expandIcon string
	if node.Expanded {
		expandIcon = IconExpanded
	} else {
		expandIcon = IconCollapsed
	}
	expandIcon = styles.TextMutedStyle.Render(expandIcon)

	// Dir icon
	var dirIcon string
	if node.Expanded {
		dirIcon = IconDirOpen
	} else {
		dirIcon = IconDir
	}

	// Title style
	var name string
	if isSelected {
		name = styles.TextPrimaryStyle.Render(fmt.Sprintf("%s %s", dirIcon, node.Name))
	} else {
		name = styles.TextMutedStyle.Render(fmt.Sprintf("%s %s", dirIcon, node.Name))
	}

	// Indent based on depth
	indent := strings.Repeat("  ", fn.Depth)

	return fmt.Sprintf("%s%s %s", indent, expandIcon, name)
}

// renderDocFileNode renders a file node with tree connectors and file icon.
func renderDocFileNode(fn DocFlatNode, isSelected bool) string {
	node := fn.Node

	// Indent: 2 spaces per depth level (depth-1 because connector fills some space at parent level)
	var indent string
	if fn.Depth > 1 {
		indent = strings.Repeat("  ", fn.Depth-1)
	}

	// Tree connector
	var connector string
	if fn.IsLast {
		connector = treeLast
	} else {
		connector = treeBranch
	}
	connector = styles.TextMutedStyle.Render(connector)

	// File name with icon
	label := fmt.Sprintf("%s %s", IconFile, node.Name)
	var name string
	if isSelected {
		name = styles.TextPrimaryStyle.Render(label)
	} else {
		name = styles.TextMutedStyle.Render(label)
	}

	if fn.Depth == 0 {
		// Root-level file: no indent, no connector
		return name
	}

	return fmt.Sprintf("%s%s %s", indent, connector, name)
}
