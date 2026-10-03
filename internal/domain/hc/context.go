package hc

import (
	"fmt"
	"strings"
)

// ContextBlock is the assembled context view for an epic.
type ContextBlock struct {
	Epic         Item              `json:"epic"`
	Counts       TaskCounts        `json:"counts"`
	MyTasks      []TaskWithComment `json:"my_tasks"`
	AllOpenTasks []Item            `json:"all_open_tasks"`
}

// TaskCounts holds counts of items by status.
type TaskCounts struct {
	Open       int `json:"open"`
	InProgress int `json:"in_progress"`
	Done       int `json:"done"`
	Cancelled  int `json:"cancelled"`
}

// TaskWithComment pairs an item with its latest comment.
type TaskWithComment struct {
	Item          Item    `json:"item"`
	LatestComment Comment `json:"latest_comment"`
}

// String returns a markdown representation of the context block suitable
// for display in the CLI or consumption by an AI agent.
func (c ContextBlock) String() string {
	var b strings.Builder

	fmt.Fprintf(&b, "# %s\n\n", c.Epic.Title)
	if c.Epic.Desc != "" {
		fmt.Fprintf(&b, "%s\n\n", c.Epic.Desc)
	}

	fmt.Fprintf(&b, "**Progress:** %d open · %d in progress · %d done · %d cancelled\n\n",
		c.Counts.Open, c.Counts.InProgress, c.Counts.Done, c.Counts.Cancelled)

	if len(c.MyTasks) > 0 {
		fmt.Fprintf(&b, "## My Tasks\n\n")
		for _, t := range c.MyTasks {
			fmt.Fprintf(&b, "- [%s] **%s** (`%s`)\n", t.Item.Status, t.Item.Title, t.Item.ID)
			if t.LatestComment.Message != "" {
				fmt.Fprintf(&b, "  > %s\n", t.LatestComment.Message)
			}
		}
		fmt.Fprintf(&b, "\n")
	}

	if len(c.AllOpenTasks) > 0 {
		fmt.Fprintf(&b, "## Other Open Tasks\n\n")
		for _, item := range treeOrder(c.AllOpenTasks) {
			indent := strings.Repeat("  ", max(0, item.Depth-1))
			fmt.Fprintf(&b, "%s- [%s] %s (`%s`)\n", indent, item.Status, item.Title, item.ID)
		}
	}

	return b.String()
}

// treeOrder returns items sorted in parent-before-children traversal order,
// preserving sibling order from the input slice.
func treeOrder(items []Item) []Item {
	byID := make(map[string]Item, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}

	children := make(map[string][]Item)
	var roots []Item
	for _, item := range items {
		if _, ok := byID[item.ParentID]; !ok {
			roots = append(roots, item)
		} else {
			children[item.ParentID] = append(children[item.ParentID], item)
		}
	}

	result := make([]Item, 0, len(items))
	var walk func(item Item)
	walk = func(item Item) {
		result = append(result, item)
		for _, child := range children[item.ID] {
			walk(child)
		}
	}
	for _, root := range roots {
		walk(root)
	}
	return result
}
