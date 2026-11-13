// Package nativeterm draws terminals natively: each one is a libghostty
// surface in an NSView layered over the webview at a rectangle the frontend
// reports. The webview never renders these terminals.
//
// It is built only on macOS with the `ghostty` build tag, which links the
// libghostty that scripts/build/build-ghostty.sh builds. Elsewhere Available
// reports false and New fails.
package nativeterm

import "errors"

// ErrUnavailable is returned by New in a build without native terminals.
var ErrUnavailable = errors.New("nativeterm: this build has no native terminals")

// EventKind is what happened to a surface.
type EventKind int

const (
	// EventClosed: the surface's command ended. Data is empty, or why the
	// surface failed to start.
	EventClosed EventKind = 1
	// EventTitle: the program set the title. Data is the title.
	EventTitle EventKind = 2
	// EventFocus: the surface gained ("1") or lost ("0") keyboard focus.
	EventFocus EventKind = 3
	// EventKey: a ⌘ chord the surface does not bind, for the app's keymap.
	// Data is JSON: key, keyCode, meta, ctrl, alt, shift.
	EventKey EventKind = 4
	// EventOpenURL: the user opened a link. Data is the URL.
	EventOpenURL EventKind = 5
	// EventCell: the cell size changed. Data is "width,height" in points.
	EventCell EventKind = 6
)

// Event is one surface's report.
type Event struct {
	ID   string
	Kind EventKind
	Data string
}

// Spec is what a surface runs.
type Spec struct {
	// Command is a shell command line the surface runs in its own PTY.
	Command  string
	Cwd      string
	FontSize float64
	// Env is added to the command's environment.
	Env map[string]string
}

// Rect is where a surface sits, in CSS pixels from the webview's top-left.
type Rect struct {
	X, Y, W, H float64
	Visible    bool
}
