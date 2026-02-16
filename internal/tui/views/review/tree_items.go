package review

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

// TreeItemsDocuments yields only document items (not headers) together with
// their index.
func TreeItemsDocuments(items []list.Item) iter.Seq2[int, TreeItem] {
	return func(yield func(int, TreeItem) bool) {
		for i, item := range items {
			ti, ok := item.(TreeItem)
			if !ok || !ti.IsDocument() {
				continue
			}
			if !yield(i, ti) {
				return
			}
		}
	}
}

// TreeItemsFirstDocument returns the index of the first document item, or -1
// if the list contains no documents.
func TreeItemsFirstDocument(items []list.Item) int {
	for i, ti := range TreeItemsDocuments(items) {
		_ = ti
		return i
	}
	return -1
}
