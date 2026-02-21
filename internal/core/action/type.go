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
//
// )
type Type string
