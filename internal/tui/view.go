package tui

// ViewType represents which view is active in narrow/tab mode.
type ViewType int

const (
	ViewSessions ViewType = iota
	ViewMessages
)

// FocusedPane represents which pane has focus in split mode.
type FocusedPane int

const (
	PaneSessions FocusedPane = iota
	PaneMessages
)

// splitWidthThreshold is the minimum terminal width for split layout.
const splitWidthThreshold = 140
