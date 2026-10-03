// Package assess implements Stage 1 of the two-stage status assessment
// engine: a pure, stateless mapping from one terminal snapshot to one
// semantic assessment. Stage 2 (the debouncing status.Tracker) owns
// turning Assessment values into published terminal.Status values.
package assess

// Snapshot is everything an integration can observe about one pane at one
// instant. tmux fills it from list-panes + capture-pane; a PTY embedder
// fills it from its own buffer.
type Snapshot struct {
	Content string // visible viewport text; engine normalizes (ANSI, NBSP) once at entry
	Title   string // OSC pane title, if available
	Tool    string // classified tool ("claude", "codex", ..., "agent")
	InMode  bool   // pane is in copy-mode/view-mode (tmux #{pane_in_mode})

	// Generation is the transport's refresh generation. Engine.Assess
	// ignores it — only Tracker.Observe consumes it, idempotent per
	// (key, Generation): the hive status service resolves the same primary
	// pane twice per poll cycle, and without this a debounce counter would
	// double-advance for one real observation. Stage-1-only callers omit it.
	Generation uint64
}
