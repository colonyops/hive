package ingest

import "github.com/colonyops/hive/cmd/desktop/internal/app/observe"

var tracer = observe.Tracer("/internal/app/ingest")

// Span attributes, not metric labels, which is why an unbounded source id is
// safe here.
const (
	attrForced    = "ingest.forced"
	attrSources   = "ingest.sources"
	attrDrained   = "ingest.drained"
	attrFailed    = "ingest.failed"
	attrAppended  = "ingest.appended"
	attrSourceID  = "ingest.source.id"
	attrSourceKnd = "ingest.source.kind"
	attrTopic     = "ingest.source.topic"
)
