package app

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
	"github.com/colonyops/hive/cmd/desktop/internal/app/runtime"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources"
)

// DryRunDocumentID is the flow id an inline document runs under when it names
// none. It scopes the run's node KV and the feed ids in its would-be outputs,
// both of which are flow-qualified; it never has to match anything installed,
// because a dry run reads and writes nothing that survives it.
const DryRunDocumentID = "dryrun"

// FlowDryRun asks for one flow execution against caller-supplied input.
//
// Exactly one of FlowID and Document identifies the flow: an installed one, or
// a document that is not on disk — which is what makes this a testing tool
// rather than "run the installed flow harder". An unsaved edit can be executed
// before it is deployed.
type FlowDryRun struct {
	// FlowID names an installed flow, enabled or not.
	FlowID string
	// Document is a flow document in the flows/*.yaml shape, as YAML or as
	// JSON. It is parsed and validated exactly as a file would be, and nothing
	// is written.
	Document []byte
	// DocumentID is the id Document runs under; empty means DryRunDocumentID.
	// Ignored when FlowID is set.
	DocumentID string

	// NodeID is the node the input is delivered to. Any node will do, not only
	// a source: injecting straight at a function node is how one node is
	// exercised in isolation against a captured payload.
	NodeID string
	// Messages is the input. A message with a Snapshot expands into its items
	// and declares feed reconciliation, the same as a source poll would.
	Messages []models.Msg
	// KV seeds the sandbox node KV: node id -> key -> the value's JSON text.
	// Durable KV is never read and never written, so dedup and notify-once
	// logic is exercised against exactly what is seeded here and nothing else.
	KV map[string]map[string]string
}

// FlowDryRunResult is a dry run's outcome: the flow it resolved to, whatever
// soft warnings parsing it produced, and what every node did.
type FlowDryRunResult struct {
	FlowID   string
	Warnings []string
	Run      runtime.DryRunResult
}

// DryRun executes a flow against supplied input and reports what each node
// emitted, without touching live state.
//
// Nothing is committed: no feed membership, no inbox rows, no notifications, no
// output_commands, no durable KV. The outputs those would have come from are
// returned instead, so "what would this flow have done with this input" is
// answered without doing it. Sources do not fetch — a source node is a relay at
// run time, so its output during a dry run is exactly what the caller injected.
//
// The runner is built for this call and closed after it. That is load-bearing
// rather than tidy: a Runner carries each function node's `state` object across
// messages, so reusing the engine's installed runner would let a dry run mutate
// the state a live run depends on. It also makes repeated calls with the same
// input give the same answer, which is the property an agent debugging in a
// loop is relying on.
func (s *FlowsService) DryRun(ctx context.Context, req FlowDryRun) (FlowDryRunResult, error) {
	f, warnings, err := s.resolveDryRunFlow(req)
	if err != nil {
		return FlowDryRunResult{}, err
	}

	// The diagnostics below reach the caller verbatim rather than as a wrapped
	// cause: a dry run is asked precisely because something is wrong, so "the
	// script does not compile, at line 4" is the answer, not a detail for the
	// log. Everything here is the caller's own input.
	runner, err := runtime.NewRunner(f, runtime.Options{
		Scripts: s.scripts,
		KV:      runtime.MemoryKV(req.KV),
	})
	if err != nil {
		return FlowDryRunResult{}, Errorf(KindInvalid, "flow %q cannot be executed: %v", f.ID, err)
	}
	defer runner.Close()

	result, err := runner.DryRun(ctx, req.NodeID, dryRunInput(f, req))
	if err != nil {
		return FlowDryRunResult{}, Errorf(KindInvalid, "%v", err)
	}
	return FlowDryRunResult{FlowID: f.ID, Warnings: warnings, Run: result}, nil
}

// resolveDryRunFlow turns the request's flow reference into a validated flow.
func (s *FlowsService) resolveDryRunFlow(req FlowDryRun) (flow.Flow, []string, error) {
	if req.FlowID != "" {
		f, ok := s.flows.Get(req.FlowID)
		if !ok {
			return flow.Flow{}, nil, Errorf(KindNotFound, "flow %q not found", req.FlowID)
		}
		return f, nil, nil
	}

	id := req.DocumentID
	if id == "" {
		id = DryRunDocumentID
	}
	f, warnings, err := s.flows.ParseDocument(id, req.Document)
	if err != nil {
		return flow.Flow{}, nil, Errorf(KindInvalid, "the flow document did not parse: %v", err)
	}
	return f, warnings, nil
}

// dryRunInput fills in the envelope fields a caller should not have to know
// about. An injected message needs an id for the discard records that name it,
// and a source topic for the feed membership a terminal downstream would claim
// — both of which a caller writing a payload by hand has no reason to hold.
//
// The topic is only defaulted when the input goes to a source node, because
// that is the only case where the right value is derivable: a source's live
// topic is its own flow-qualified id. Injecting mid-graph leaves the topic
// alone, since what a real upstream would have set is not knowable here.
func dryRunInput(f flow.Flow, req FlowDryRun) []models.Msg {
	topic := ""
	for i := range f.Nodes {
		if f.Nodes[i].ID != req.NodeID {
			continue
		}
		if _, isSource := sources.Lookup(f.Nodes[i].Type); isSource {
			topic = "source:" + f.ID + "/" + req.NodeID
		}
		break
	}

	out := make([]models.Msg, len(req.Messages))
	for i, msg := range req.Messages {
		if msg.ID == "" {
			msg.ID = runtime.SyntheticMsgID(i)
		}
		if msg.Topic == "" {
			msg.Topic = topic
		}
		out[i] = msg
	}
	return out
}
