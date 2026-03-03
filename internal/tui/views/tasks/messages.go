package tasks

import "github.com/colonyops/hive/internal/core/hc"

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

// contentRenderedMsg carries pre-rendered detail content produced off the event loop.
type contentRenderedMsg struct {
	key     string // cache key: "itemID:commentCount:width"
	content string
}
