package runtime

import "github.com/colonyops/hive/cmd/desktop/internal/app/observe"

var tracer = observe.Tracer("/internal/app/runtime")

// Span attributes, not metric labels, which is why an unbounded flow id is
// safe here.
const (
	attrReload   = "runtime.reload"
	attrFlows    = "runtime.flows"
	attrPages    = "runtime.pages"
	attrMessages = "runtime.messages"
	attrFailed   = "runtime.failed"
	attrFlowID   = "runtime.flow.id"
)
