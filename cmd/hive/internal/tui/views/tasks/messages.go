package tasks

import (
	"github.com/colonyops/hive/internal/core/action"
	"github.com/colonyops/hive/internal/core/hc"
)

// RefreshTasksMsg signals the tasks view to reload its data.
type RefreshTasksMsg struct{}

// itemsLoadedMsg carries the result of loading items from the store.
type itemsLoadedMsg struct {
	items []hc.Item
	err   error
}

// commentsLoadedMsg carries the result of loading comments for an item.
type commentsLoadedMsg struct {
	comments []hc.Comment
	itemID   string
	err      error
}

// blockersLoadedMsg carries the result of loading explicit blockers for an item.
type blockersLoadedMsg struct {
	blockers []hc.Item
	itemID   string
	partial  bool // true when some blocker items could not be fetched
	err      error
}

// contentRenderedMsg carries pre-rendered detail content produced off the event loop.
type contentRenderedMsg struct {
	key     string // cache key: "itemID:commentCount:blockerCount:width"
	content string
}

// ActionRequestMsg requests the parent to execute a resolved action.
type ActionRequestMsg struct {
	Action action.Action
}

// TaskActionCompleteMsg carries the result of a task mutation (status change, delete, prune).
type TaskActionCompleteMsg struct {
	Err error
}

// CommandPaletteRequestMsg requests the parent to open the command palette.
type CommandPaletteRequestMsg struct{}
