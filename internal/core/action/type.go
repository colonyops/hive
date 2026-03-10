package action

// Type identifies the kind of action a keybinding or command triggers.
//
// ENUM(
//
//	None
//	Recycle
//	Delete
//	Shell
//	TmuxOpen
//	TmuxStart
//	FilterAll
//	FilterActive
//	FilterApproval
//	FilterReady
//	DocReview
//	NewSession
//	SetTheme
//	Notifications
//	RenameSession
//	NextActive
//	PrevActive
//	DeleteRecycledBatch
//	SpawnWindows
//	HiveInfo
//	HiveDoctor
//	GroupSet
//	GroupToggle
//	TodoPanel
//	TasksRefresh
//	TasksFilter
//	TasksCopyID
//	TasksTogglePreview
//	TasksSelectRepo
//	TasksSetOpen
//	TasksSetInProgress
//	TasksSetDone
//	TasksSetCancelled
//	TasksDelete
//	TasksPrune
//	ViewTasks
//	DocsCopyPath
//	DocsCopyRelPath
//	DocsCopyContents
//	DocsOpen
//	DocsTogglePreview
//	DocsToggleTree
//	DocsSelectRepo
//
// )
type Type string
