package app

import "github.com/colonyops/hive/internal/platform/observe"

var tracer = observe.Tracer("/internal/app")

// Span attributes, not metric labels, which is why an unbounded session slug is
// safe here.
const (
	attrTerminalSlug    = "terminal.slug"
	attrTerminalCols    = "terminal.cols"
	attrTerminalRows    = "terminal.rows"
	attrTerminalRunning = "terminal.running"
	attrTerminalWindows = "terminal.windows"
)
