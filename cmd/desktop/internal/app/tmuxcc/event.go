package tmuxcc

import "time"

// Event is one item on a client's outbound stream.
type Event interface{ isEvent() }

// WindowEventKind is the tab-set change vocabulary. The values are the wire
// strings the WebSocket adapter emits verbatim.
type WindowEventKind string

const (
	WindowAdded         WindowEventKind = "added"
	WindowClosed        WindowEventKind = "closed"
	WindowRenamed       WindowEventKind = "renamed"
	WindowActiveChanged WindowEventKind = "active-changed"
	// WindowLayoutChanged covers a resize as well as a split, a closed pane or
	// a zoom: the window's size is its layout's root box.
	WindowLayoutChanged WindowEventKind = "layout-changed"
)

// LifecycleKind is the connection-state vocabulary, wire strings likewise.
type LifecycleKind string

const (
	LifecycleAttached LifecycleKind = "attached"
	LifecyclePaused   LifecycleKind = "paused"
	LifecycleResumed  LifecycleKind = "resumed"
	LifecycleExited   LifecycleKind = "exited"
	LifecycleError    LifecycleKind = "error"
	// LifecycleDegraded says output was lost and a repaint is following it onto
	// the same stream. It is an edge, not a mode: nothing clears it, because
	// there is no state to clear — the snapshot behind it is the recovery.
	LifecycleDegraded LifecycleKind = "degraded"
)

// WindowChanged reports a change to the tab set.
type WindowChanged struct {
	Kind   WindowEventKind
	Window Window
}

// Output is one pane's decoded bytes. Every pane in a window's layout streams;
// only a pane no tracked window owns is dropped. At is the decode timestamp
// the WebSocket adapter measures send latency against.
type Output struct {
	At       time.Time
	WindowID string
	PaneID   string
	Data     []byte
}

// LifecycleChanged reports attach/pause/resume/exit/error. WindowID is set for
// per-window pause/resume; Message carries an exit reason or error text.
type LifecycleChanged struct {
	Kind     LifecycleKind
	WindowID string
	Message  string
}

func (WindowChanged) isEvent()    {}
func (Output) isEvent()           {}
func (LifecycleChanged) isEvent() {}
