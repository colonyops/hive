package mcpsrv

import (
	"cmp"
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
	"github.com/colonyops/hive/cmd/desktop/internal/app/runtime"
	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/internal/ops/flows"
)

// nodeImageView reports which node a feed-mark image belongs to and whether one
// is set; the bytes come from node_image_get.
type nodeImageView struct {
	ProfileID string `json:"profileId"`
	NodeID    string `json:"nodeId"`
	HasImage  bool   `json:"hasImage"`
}

func (ctrl *Controller) GetNodeImage(ctx context.Context, in flows.NodeImageIn) (*mcp.CallToolResult, any, error) {
	data, err := ctrl.core.Flows.NodeImage(ctx, in.ProfileID, in.NodeID)
	if err != nil {
		return nil, nil, err
	}
	if len(data) == 0 {
		return nil, nil, app.Errorf(app.KindNotFound, "node %q in profile %q has no image", in.NodeID, in.ProfileID)
	}
	return pngResult(data), nil, nil
}

func (ctrl *Controller) SetNodeImage(ctx context.Context, in flows.NodeImagePutIn) (nodeImageView, error) {
	raw, err := decodeImage(in.ImageBase64)
	if err != nil {
		return nodeImageView{}, err
	}
	if _, err := ctrl.core.Flows.SetNodeImage(ctx, in.ProfileID, in.NodeID, raw); err != nil {
		return nodeImageView{}, err
	}
	return nodeImageView{ProfileID: in.ProfileID, NodeID: in.NodeID, HasImage: true}, nil
}

func (ctrl *Controller) ClearNodeImage(ctx context.Context, in flows.NodeImageIn) (nodeImageView, error) {
	if err := ctrl.core.Flows.ClearNodeImage(ctx, in.ProfileID, in.NodeID); err != nil {
		return nodeImageView{}, err
	}
	return nodeImageView{ProfileID: in.ProfileID, NodeID: in.NodeID, HasImage: false}, nil
}

// flowView is a profile's graph as this adapter reports it. It exists because
// nothing else on the surface exposes a node id, and every id-taking argument
// here — flow_preview's nodeId, the node-image tools' — needs one.
//
// A profile whose file does not parse still answers, with valid=false and the
// load error, matching profile_list: the graph you cannot run is exactly the
// one you need to read.
type flowView struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	Enabled  bool        `json:"enabled"`
	Valid    bool        `json:"valid"`
	Error    string      `json:"error,omitempty"`
	Warnings []string    `json:"warnings,omitempty"`
	Nodes    []flow.Node `json:"nodes"`
	Wires    []flow.Wire `json:"wires"`
}

// GetFlow's Out type is `any` because flow.Node's wire shape comes from its
// MarshalJSON, which flattens each node's per-type config to the top level. An
// inferred schema would describe the Go struct instead, and so would be wrong
// about every node it claimed to describe.
func (ctrl *Controller) GetFlow(ctx context.Context, in flows.ProfileIn) (any, error) {
	for _, st := range ctrl.core.Flows.Statuses(ctx) {
		if st.ID != in.ProfileID {
			continue
		}
		view := flowView{
			ID: st.ID, Name: st.Flow.Name, Enabled: st.Flow.Enabled, Valid: st.Valid,
			Warnings: st.Warnings, Nodes: st.Flow.Nodes, Wires: st.Flow.Wires,
		}
		if st.Err != nil {
			view.Error = st.Err.Error()
		}
		if view.Nodes == nil {
			view.Nodes = []flow.Node{}
		}
		if view.Wires == nil {
			view.Wires = []flow.Wire{}
		}
		return view, nil
	}
	return nil, app.Errorf(app.KindNotFound, "profile %q not found", in.ProfileID)
}

// executeFlowResult is what the run observed. Nothing in it was applied:
// outputs and kvMutations are what a live run would have committed.
type executeFlowResult struct {
	// ProfileID is the flow that ran: the installed profile's id, or the id
	// an inline document ran under.
	ProfileID string `json:"profileId"`
	// Detail echoes the level this result was rendered at, so a caller reading
	// a node with no received list can tell "omitted" from "empty".
	Detail string `json:"detail"`
	// Warnings are the soft diagnostics parsing an inline document produced,
	// the same ones a file load reports.
	Warnings []string `json:"warnings,omitempty"`
	runtime.DryRunResult
	// Nodes shadows DryRunResult.Nodes with the traces trimmed to Detail. The
	// embedded field is still encoded by its own name, so the shadow has to
	// carry the same JSON name to replace rather than duplicate it — Go
	// resolves the conflict in favour of the shallower field.
	Nodes []nodeTraceView `json:"nodes"`
}

// nodeTraceView is one node's trace with its two message lists under the
// caller's control. Both are pointers so the three states stay distinct: absent
// means this detail level did not report it, [] means the node genuinely had
// none — a distinction a plain slice with omitempty collapses, and the run's
// collections are non-nil precisely so tooling can count them.
type nodeTraceView struct {
	runtime.NodeTrace
	Received *[]models.Msg           `json:"received,omitempty"`
	Emitted  *[]runtime.PortEmission `json:"emitted,omitempty"`
}

// traceViews trims each node's message lists to what detail reports.
func traceViews(traces []runtime.NodeTrace, detail string) []nodeTraceView {
	out := make([]nodeTraceView, len(traces))
	for i, trace := range traces {
		view := nodeTraceView{NodeTrace: trace}
		if detail == ops.DetailFull {
			view.Received = &traces[i].Received
		}
		if detail != ops.DetailSummary {
			view.Emitted = &traces[i].Emitted
		}
		out[i] = view
	}
	return out
}

func (ctrl *Controller) ExecuteFlow(ctx context.Context, in flows.FlowPreviewIn) (any, error) {
	document, err := flowDocument(in)
	if err != nil {
		return nil, err
	}

	messages, err := toStoreMessages(in.Messages)
	if err != nil {
		return nil, err
	}
	kv, err := seedKV(in.KV)
	if err != nil {
		return nil, err
	}

	result, err := ctrl.core.Flows.DryRun(ctx, app.FlowDryRun{
		FlowID:     in.ProfileID,
		Document:   document,
		DocumentID: in.ProfileIDOverride,
		NodeID:     in.NodeID,
		Messages:   messages,
		KV:         kv,
	})
	if err != nil {
		return nil, err
	}
	detail := cmp.Or(in.Detail, flows.DetailEmitted)
	return executeFlowResult{
		ProfileID:    result.FlowID,
		Detail:       detail,
		Warnings:     result.Warnings,
		DryRunResult: result.Run,
		Nodes:        traceViews(result.Run.Nodes, detail),
	}, nil
}

// flowDocument renders whichever inline form was given back into bytes for the
// core's parser. A JSON object needs no conversion: the parser is a YAML
// decoder, and JSON is YAML.
func flowDocument(in flows.FlowPreviewIn) ([]byte, error) {
	switch {
	case in.FlowYAML != "":
		return []byte(in.FlowYAML), nil
	case len(in.Flow) > 0:
		encoded, err := json.Marshal(in.Flow)
		if err != nil {
			return nil, app.Wrap(err, app.KindInvalid, "encoding the flow document")
		}
		return encoded, nil
	default:
		return nil, nil
	}
}

// seedKV flattens the input's JSON values into the opaque JSON text node KV
// stores, so what a seeded kv.get parses back is what the caller wrote.
func seedKV(seed map[string]map[string]any) (map[string]map[string]string, error) {
	if len(seed) == 0 {
		return nil, nil
	}
	out := make(map[string]map[string]string, len(seed))
	for nodeID, entries := range seed {
		values := make(map[string]string, len(entries))
		for key, value := range entries {
			raw, err := json.Marshal(value)
			if err != nil {
				return nil, app.Wrap(err, app.KindInvalid, "encoding kv value %q for node %q", key, nodeID)
			}
			values[key] = string(raw)
		}
		out[nodeID] = values
	}
	return out, nil
}

// toStoreMessages converts the wire messages into the engine's own envelope.
//
// A nil Snapshot stays nil and an empty one stays empty: models.Msg treats a
// present-but-empty snapshot as a successful poll that returned zero items,
// which declares feed reconciliation, while an absent one is an ordinary
// item message. Collapsing the two silently changes what the run means.
func toStoreMessages(in []flows.Message) ([]models.Msg, error) {
	out := make([]models.Msg, len(in))
	for i, m := range in {
		payload, err := rawJSON(m.Payload)
		if err != nil {
			return nil, err
		}
		msg := models.Msg{
			ID: m.ID, Key: m.Key, Topic: m.Topic, Ts: m.Ts, Payload: payload,
			SourceKind: m.SourceKind, SourceScope: m.SourceScope, OccurrenceKey: m.OccurrenceKey,
		}
		if m.Snapshot != nil {
			snapshot := make([]models.SnapshotItem, len(m.Snapshot))
			for j, item := range m.Snapshot {
				itemPayload, err := rawJSON(item.Payload)
				if err != nil {
					return nil, err
				}
				snapshot[j] = models.SnapshotItem{Key: item.Key, Payload: itemPayload}
			}
			msg.Snapshot = snapshot
		}
		out[i] = msg
	}
	return out, nil
}

// rawJSON re-encodes a decoded value as the raw JSON text the engine carries.
// A nil value stays nil rather than becoming the literal "null", so an omitted
// payload is absent instead of present-and-null.
func rawJSON(v any) (json.RawMessage, error) {
	if v == nil {
		return nil, nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, app.Wrap(err, app.KindInvalid, "encoding a message payload")
	}
	return raw, nil
}
