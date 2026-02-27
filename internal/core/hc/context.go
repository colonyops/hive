package hc

// ContextBlock is the assembled context view for an epic.
type ContextBlock struct {
	Epic         Item
	Counts       TaskCounts
	MyTasks      []TaskWithCheckpoint
	AllOpenTasks []Item
}

// TaskCounts holds counts of items by status.
type TaskCounts struct {
	Open       int
	InProgress int
	Done       int
	Cancelled  int
}

// TaskWithCheckpoint pairs an item with its latest checkpoint activity.
type TaskWithCheckpoint struct {
	Item             Item
	LatestCheckpoint Activity
}
