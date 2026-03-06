package review

import "github.com/colonyops/hive/internal/core/action"

// ActionRequestMsg requests the parent to execute a resolved action.
type ActionRequestMsg struct {
	Action action.Action
}

// docPreviewRenderedMsg carries the result of an async document render for the
// tree-pane preview. The path is used to discard stale results if the user has
// already moved to a different file.
type docPreviewRenderedMsg struct {
	path    string
	content string
}
