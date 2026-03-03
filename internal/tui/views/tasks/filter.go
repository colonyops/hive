package tasks

import (
	"strings"

	"github.com/colonyops/hive/internal/core/hc"
	"github.com/colonyops/hive/internal/core/styles"
)

// StatusFilter represents a filter for task status in the tree view.
type StatusFilter int

const (
	FilterAll    StatusFilter = iota
	FilterOpen                // open + in_progress
	FilterActive              // in_progress only
	FilterDone                // done + cancelled
)

var filterLabels = []string{"All", "Open", "Active", "Done"}

const filterCount = 4

func (f StatusFilter) String() string {
	if int(f) < len(filterLabels) {
		return filterLabels[f]
	}
	return "unknown"
}

// filterItems returns items matching the given status filter.
func filterItems(items []hc.Item, filter StatusFilter) []hc.Item {
	if filter == FilterAll {
		return items
	}

	var result []hc.Item
	for _, item := range items {
		switch filter {
		case FilterAll:
			result = append(result, item)
		case FilterOpen:
			if item.Status == hc.StatusOpen || item.Status == hc.StatusInProgress {
				result = append(result, item)
			}
		case FilterActive:
			if item.Status == hc.StatusInProgress {
				result = append(result, item)
			}
		case FilterDone:
			if item.Status == hc.StatusDone || item.Status == hc.StatusCancelled {
				result = append(result, item)
			}
		}
	}
	return result
}

// renderFilterBar renders the status filter bar with the active filter highlighted.
func renderFilterBar(active StatusFilter) string {
	var parts []string
	for i, label := range filterLabels {
		if StatusFilter(i) == active {
			parts = append(parts, styles.TextPrimaryBoldStyle.Render(label))
		} else {
			parts = append(parts, styles.TextMutedStyle.Render(label))
		}
	}
	return strings.Join(parts, styles.TextMutedStyle.Render(" · "))
}
