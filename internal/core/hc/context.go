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
		fmt.Fprintf(&b, "## Open Tasks\n\n")
		for _, item := range c.AllOpenTasks {
			indent := strings.Repeat("  ", max(0, item.Depth-1))
			fmt.Fprintf(&b, "%s- [%s] %s (`%s`)\n", indent, item.Status, item.Title, item.ID)
		}
	}

	return b.String()
}
