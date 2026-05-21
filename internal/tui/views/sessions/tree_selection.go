package sessions

// TreeSelection captures the identity of a selected tree item and can find
// the best matching item after a list rebuild. Operates purely on TreeItem
// slices so it can be tested without the full TUI model.
type TreeSelection struct {
	sessionID    string // session ID (or parent session ID for window/pane items)
	windowName   string // non-empty for window sub-items
	windowIndex  string // fallback for window sub-items
	paneID       string // non-empty for pane sub-items
	recycledRepo string // non-empty for recycled placeholders
	index        int    // raw list index as last-resort fallback
}

// SaveTreeSelection captures the identity of the selected item at the given index.
// Pass nil if nothing is selected.
func SaveTreeSelection(item *TreeItem, index int) TreeSelection {
	sel := TreeSelection{index: index}
	if item == nil {
		return sel
	}
	switch {
	case item.IsPaneItem:
		sel.sessionID = item.ParentSession.ID
		sel.windowIndex = item.ParentWindow
		sel.paneID = item.PaneID
	case item.IsWindowItem:
		sel.sessionID = item.ParentSession.ID
		sel.windowName = item.WindowName
		sel.windowIndex = item.WindowIndex
	case item.IsRecycledPlaceholder:
		sel.recycledRepo = item.RepoPrefix
	case !item.IsHeader:
		sel.sessionID = item.Session.ID
	}
	return sel
}

// restore returns the best matching index in items for the saved selection.
//
// Priority:
//  1. Window sub-item by session ID + window index (unique within session)
//  2. Window sub-item by session ID + window name
//  3. Recycled placeholder by repo prefix
//  4. Session by ID
//  5. Original index (clamped to bounds)
//  6. First non-header item
func (s TreeSelection) Restore(items []TreeItem) int {
	// 1. Pane by ID
	if s.paneID != "" {
		for i, ti := range items {
			if ti.IsPaneItem && ti.ParentSession.ID == s.sessionID && ti.PaneID == s.paneID {
				return i
			}
		}
	}

	// 2. Window by index
	if s.windowIndex != "" {
		for i, ti := range items {
			if ti.IsWindowItem &&
				ti.ParentSession.ID == s.sessionID && ti.WindowIndex == s.windowIndex {
				return i
			}
		}
	}

	// 3. Window by name
	if s.windowName != "" {
		for i, ti := range items {
			if ti.IsWindowItem &&
				ti.ParentSession.ID == s.sessionID && ti.WindowName == s.windowName {
				return i
			}
		}
	}

	// 4. Recycled placeholder by repo
	if s.recycledRepo != "" {
		for i, ti := range items {
			if ti.IsRecycledPlaceholder && ti.RepoPrefix == s.recycledRepo {
				return i
			}
		}
	}

	// 5. Session by ID
	if s.sessionID != "" {
		for i, ti := range items {
			if ti.IsSession() && ti.Session.ID == s.sessionID {
				return i
			}
		}
	}

	// 6. Original index clamped to bounds
	if s.index >= 0 && s.index < len(items) {
		return s.index
	}

	// 7. First non-header
	for i, ti := range items {
		if !ti.IsHeader {
			return i
		}
	}
	return 0
}
