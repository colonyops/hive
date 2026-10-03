package multiplexer

// Pane describes a discovered multiplexer pane.
type Pane struct {
	// Target is the session:window.pane address at discovery time. Window and
	// pane indexes can shift when neighbours close, so it is not stable.
	Target Target
	// NativeID is the multiplexer's own stable pane identifier (tmux %N). It
	// survives window and pane renumbering, unlike Target.
	NativeID string
	// PID is the process ID of the pane's root process (usually the shell).
	PID int64
	// WindowName is the display name of the window that holds the pane.
	WindowName string
	// Title is the pane title, which agents often set to their own name.
	Title string
	// WorkingDirectory is the current directory of the pane's root process.
	WorkingDirectory string
	// Activity is the Unix timestamp of the last output in the pane's window.
	Activity int64
	// HiveSession is the Hive slug tagged on the pane when Hive created it
	// (tmux @hive-session). It lets Hive find a session after a user renames
	// the multiplexer session.
	HiveSession string
	// InMode reports whether the pane is in copy or view mode, so its visible
	// content is scrollback rather than live output.
	InMode bool
}
