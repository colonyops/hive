package tui

import (
	"iter"

	"charm.land/bubbles/v2/list"
)

// TreeItemsAll yields every TreeItem in items together with its index.
// Non-TreeItem elements are silently skipped.
func TreeItemsAll(items []list.Item) iter.Seq2[int, TreeItem] {
	return func(yield func(int, TreeItem) bool) {
		for i, item := range items {
			ti, ok := item.(TreeItem)
			if !ok {
				continue
			}
			if !yield(i, ti) {
				return
			}
		}
	}
}

// TreeItemsSessions yields only regular session items (not headers, recycled
// placeholders, or window sub-items) together with their index.
func TreeItemsSessions(items []list.Item) iter.Seq2[int, TreeItem] {
	return func(yield func(int, TreeItem) bool) {
		for i, item := range items {
			ti, ok := item.(TreeItem)
			if !ok || !ti.IsSession() {
				continue
			}
			if !yield(i, ti) {
				return
			}
		}
	}
}
