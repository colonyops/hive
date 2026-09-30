package tmuxcc

import "github.com/colonyops/hive/cmd/desktop/internal/app/observe"

var tracer = observe.Tracer("/internal/app/tmuxcc")

// Every span here is conditional: a client repaints, reconciles and resyncs on
// its own lifetime long after the attach that opened it answered, and a wait
// nobody asked for is not a trace
// (ADR a-span-is-a-trigger-or-a-wait-and-its-count-per-trigger-is-bounded-by-configuration).
//
// Span attributes, not metric labels, which is what makes a per-pane grid size
// safe here.
const (
	attrWindows    = "tmux.windows"
	attrDeferred   = "tmux.windows.deferred"
	attrUnsized    = "tmux.unsized"
	attrPaintRows  = "tmux.paint.rows"
	attrPaintBytes = "tmux.paint.bytes"
)
