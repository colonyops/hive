package tasks

import "github.com/colonyops/hive/internal/core/hc"

// RefreshTasksMsg signals the tasks view to reload its data.
type RefreshTasksMsg struct{}

// itemsLoadedMsg carries the result of loading items from the store.
type itemsLoadedMsg struct {
	items []hc.Item
	err   error
}
