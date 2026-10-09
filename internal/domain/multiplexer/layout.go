package multiplexer

// SplitDirection controls the orientation of a pane split.
type SplitDirection string

const (
	SplitVertical   SplitDirection = "vertical"
	SplitHorizontal SplitDirection = "horizontal"
)

// PaneSpec is a fully rendered pane definition.
type PaneSpec struct {
	Command          string
	WorkingDirectory string
	Size             string
	Split            SplitDirection
}

// WindowSpec is a fully rendered window definition.
type WindowSpec struct {
	Name             string
	Command          string
	WorkingDirectory string
	Focus            bool
	Panes            []PaneSpec
}

// LaunchResult describes a session open or launch. Completed is true only when
// every pane completed successfully and the launch left no terminal to attach to.
type LaunchResult struct {
	Created   bool
	Completed bool
}

// SessionSpec describes a session and its initial windows.
type SessionSpec struct {
	Target           Target
	WorkingDirectory string
	Windows          []WindowSpec
	Background       bool
}
