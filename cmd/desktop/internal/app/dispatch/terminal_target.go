package dispatch

// TerminalTarget identifies the terminal session — and optionally the window
// inside it — an action was invoked from. It carries identity only: every
// value a template can read is resolved from the session record at invocation
// time, so a client cannot hand an executor a checkout path of its choosing.
type TerminalTarget struct {
	Slug string `json:"slug"`
	// WindowID is set when the invocation came from a window row. It is what
	// separates a window invocation from a session one, and it is the tmux
	// window id, so it addresses the window across renames.
	WindowID string `json:"windowId,omitempty"`
}

// SessionTarget is the session half of a terminal action's template data,
// reachable from every template as `.Session.<field>`.
type SessionTarget struct {
	ID     string
	Name   string
	Slug   string
	Repo   string
	Path   string
	Branch string
}

// WindowTarget is the window half, reachable as `.Window.<field>`. Only the
// id is carried: it is what a tmux command addresses a window by, and unlike
// the name it does not change under the user.
type WindowTarget struct {
	ID string
}
