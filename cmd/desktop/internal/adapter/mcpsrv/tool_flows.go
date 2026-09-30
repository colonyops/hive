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
)

type nodeImageInput struct {
	ProfileID string `json:"profileId" jsonschema:"Profile id from list_profiles."`
	NodeID    string `json:"nodeId"    jsonschema:"Node id within that profile's flow; ids come from get_flow."`
}

type setNodeImageInput struct {
	ProfileID   string `json:"profileId"   jsonschema:"Profile id from list_profiles."`
	NodeID      string `json:"nodeId"      jsonschema:"Node id within that profile's flow; ids come from get_flow."`
	ImageBase64 string `json:"imageBase64" jsonschema:"The image as base64 (PNG, JPEG, GIF or WebP). A data: URL is accepted and its prefix ignored."`
}

// nodeImageView reports which node a feed-mark image belongs to and whether one
// is set; the bytes come from get_node_image.
type nodeImageView struct {
	ProfileID string `json:"profileId"`
	NodeID    string `json:"nodeId"`
	HasImage  bool   `json:"hasImage"`
}

func (ctrl *Controller) GetNodeImage(ctx context.Context, _ *mcp.CallToolRequest, in nodeImageInput) (*mcp.CallToolResult, any, error) {
	data, err := ctrl.core.Flows.NodeImage(ctx, in.ProfileID, in.NodeID)
	if err != nil {
		return nil, nil, ctrl.toolError(err)
	}
	if len(data) == 0 {
		return nil, nil, ctrl.toolError(app.Errorf(app.KindNotFound, "node %q in profile %q has no image", in.NodeID, in.ProfileID))
	}
	return pngResult(data), nil, nil
}

func (ctrl *Controller) SetNodeImage(ctx context.Context, _ *mcp.CallToolRequest, in setNodeImageInput) (*mcp.CallToolResult, nodeImageView, error) {
	raw, err := decodeImage(in.ImageBase64)
	if err != nil {
		return nil, nodeImageView{}, ctrl.toolError(err)
	}
	if _, err := ctrl.core.Flows.SetNodeImage(ctx, in.ProfileID, in.NodeID, raw); err != nil {
		return nil, nodeImageView{}, ctrl.toolError(err)
	}
	return nil, nodeImageView{ProfileID: in.ProfileID, NodeID: in.NodeID, HasImage: true}, nil
}

func (ctrl *Controller) ClearNodeImage(ctx context.Context, _ *mcp.CallToolRequest, in nodeImageInput) (*mcp.CallToolResult, nodeImageView, error) {
	if err := ctrl.core.Flows.ClearNodeImage(ctx, in.ProfileID, in.NodeID); err != nil {
		return nil, nodeImageView{}, ctrl.toolError(err)
	}
	return nil, nodeImageView{ProfileID: in.ProfileID, NodeID: in.NodeID, HasImage: false}, nil
}

// flowView is a profile's graph as this adapter reports it. It exists because
// nothing else on the surface exposes a node id, and every id-taking argument
// here — execute_flow's nodeId, the node-image tools' — needs one.
//
// A profile whose file does not parse still answers, with valid=false and the
// load error, matching list_profiles: the graph you cannot run is exactly the
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
func (ctrl *Controller) GetFlow(ctx context.Context, _ *mcp.CallToolRequest, in profileInput) (*mcp.CallToolResult, any, error) {
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
		return nil, view, nil
	}
	return nil, nil, ctrl.toolError(app.Errorf(app.KindNotFound, "profile %q not found", in.ProfileID))
}

// executeFlowInput is one dry run. Exactly one of flowId, flow and flowYaml
// says which flow to execute; the rest is the input and the sandbox it runs
// against.
type executeFlowInput struct {
	FlowID string `json:"flowId,omitempty" jsonschema:"An installed flow's id, enabled or not."`
	// Flow is a flow document as a JSON object — the same schema as a
	// flows/<id>.yaml file, version included, since it is parsed through the
	// same decoder a file goes through.
	Flow map[string]any `json:"flow,omitempty" jsonschema:"A flow document as a JSON object, same schema as flows/<id>.yaml including version."`
	// FlowYAML is that document as YAML text, for a caller whose source of
	// truth is the file itself.
	FlowYAML string `json:"flowYaml,omitempty" jsonschema:"That same flow document as YAML text."`
	// FlowIDOverride only scopes the run's node KV and the feed ids in its
	// outputs; nothing is installed under it.
	FlowIDOverride string `json:"flowIdOverride,omitempty" jsonschema:"The id an inline document runs under; scopes node KV and output feed ids only, and installs nothing."`

	NodeID   string        `json:"nodeId"   jsonschema:"The node messages are delivered to — any node, not only a source. Node ids come from get_flow."`
	Messages []flowMessage `json:"messages" jsonschema:"The messages to deliver."`
	// KV seeds the sandbox: node id -> key -> value. Durable KV is neither read
	// nor written, so this is the whole world a kv.get sees.
	KV map[string]map[string]any `json:"kv,omitempty" jsonschema:"Seeds an in-memory KV sandbox as nodeId -> key -> value; durable KV is neither read nor written."`
	// Detail controls how many copies of each message body come back.
	Detail string `json:"detail,omitempty" jsonschema:"How much of each message body to report — one of summary, emitted (the default) or full."`
}

// detailEmitted is execute_flow's own middle rung between the surface-wide
// detailSummary and detailFull: each node's emitted messages but not its
// received ones. A node's input is its upstream's output, and the entry node's
// is what the caller just sent, so received is derivable everywhere.
//
// The levels exist because a message costs one copy per hop it survives: every
// node reports it in received and again in emitted, so one 3KB payload through
// a five-node chain leaves as ~30KB, and a snapshot dry run multiplies that by
// its item count. They never touch the run's outputs, feed snapshots, kv
// mutations, counters, drops, console output or errors — those are one entry
// per thing that happened, not one per hop.
const detailEmitted = "emitted"

// flowMessage mirrors models.Msg for the wire. It exists because models.Msg
// carries its raw JSON as json.RawMessage, whose Go type is []byte, and the
// SDK's schema inferrer therefore describes Payload as an array — a schema
// that rejects the JSON object every real payload is, before the handler is
// ever reached. `any` is what the schema has to say for arbitrary JSON, so the
// wire type says it here and converts at the seam, which is where a
// transport-shaped DTO belongs anyway.
//
// Field names match models.Msg's own JSON (Go names, not lowercased) so a
// payload captured from the app round-trips unchanged.
type flowMessage struct {
	ID            string         `json:"ID,omitempty"            jsonschema:"Message id; a synthetic one is generated when empty."`
	Key           string         `json:"Key,omitempty"           jsonschema:"The source's own key for this item."`
	Topic         string         `json:"Topic,omitempty"         jsonschema:"Event topic; defaults to the source node's own topic when injecting at a source."`
	Ts            int64          `json:"Ts,omitempty"            jsonschema:"Unix millisecond timestamp."`
	Payload       any            `json:"Payload,omitempty"       jsonschema:"The item's payload as arbitrary JSON."`
	Snapshot      []flowSnapshot `json:"Snapshot,omitempty"      jsonschema:"A full source snapshot; present makes this a snapshot message, which declares feed reconciliation exactly as a poll would. An empty array is still a snapshot."`
	SourceKind    string         `json:"SourceKind,omitempty"    jsonschema:"The source connector kind."`
	SourceScope   string         `json:"SourceScope,omitempty"   jsonschema:"The source's scope within that kind."`
	OccurrenceKey string         `json:"OccurrenceKey,omitempty" jsonschema:"Distinguishes repeat occurrences of one item."`
}

type flowSnapshot struct {
	Key     string `json:"key"`
	Payload any    `json:"payload" jsonschema:"The item's payload as arbitrary JSON."`
}

// executeFlowResult is what the run observed. Nothing in it was applied:
// outputs and kvMutations are what a live run would have committed.
type executeFlowResult struct {
	// FlowID is the flow that ran — the installed id, or the id an inline
	// document ran under.
	FlowID string `json:"flowId"`
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
		if detail == detailFull {
			view.Received = &traces[i].Received
		}
		if detail != detailSummary {
			view.Emitted = &traces[i].Emitted
		}
		out[i] = view
	}
	return out
}

func (ctrl *Controller) ExecuteFlow(ctx context.Context, _ *mcp.CallToolRequest, in executeFlowInput) (*mcp.CallToolResult, any, error) {
	if err := validateFlowSource(in); err != nil {
		return nil, nil, ctrl.toolError(err)
	}

	document, err := flowDocument(in)
	if err != nil {
		return nil, nil, ctrl.toolError(err)
	}

	messages, err := toStoreMessages(in.Messages)
	if err != nil {
		return nil, nil, ctrl.toolError(err)
	}
	kv, err := seedKV(in.KV)
	if err != nil {
		return nil, nil, ctrl.toolError(err)
	}

	result, err := ctrl.core.Flows.DryRun(ctx, app.FlowDryRun{
		FlowID:     in.FlowID,
		Document:   document,
		DocumentID: in.FlowIDOverride,
		NodeID:     in.NodeID,
		Messages:   messages,
		KV:         kv,
	})
	if err != nil {
		return nil, nil, ctrl.toolError(err)
	}
	detail := cmp.Or(in.Detail, detailEmitted)
	return nil, executeFlowResult{
		FlowID:       result.FlowID,
		Detail:       detail,
		Warnings:     result.Warnings,
		DryRunResult: result.Run,
		Nodes:        traceViews(result.Run.Nodes, detail),
	}, nil
}

func validateFlowSource(in executeFlowInput) error {
	sources := 0
	for _, given := range []bool{in.FlowID != "", len(in.Flow) > 0, in.FlowYAML != ""} {
		if given {
			sources++
		}
	}
	if sources != 1 {
		return app.Errorf(app.KindInvalid, "set exactly one of flowId, flow, or flowYaml")
	}
	if in.NodeID == "" {
		return app.Errorf(app.KindInvalid, "nodeId is required")
	}
	if len(in.Messages) == 0 {
		return app.Errorf(app.KindInvalid, "messages must not be empty")
	}
	switch in.Detail {
	case "", detailSummary, detailEmitted, detailFull:
	default:
		return app.Errorf(app.KindInvalid, "detail must be one of %q, %q or %q", detailSummary, detailEmitted, detailFull)
	}
	return nil
}

// flowDocument renders whichever inline form was given back into bytes for the
// core's parser. A JSON object needs no conversion: the parser is a YAML
// decoder, and JSON is YAML.
func flowDocument(in executeFlowInput) ([]byte, error) {
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
func toStoreMessages(in []flowMessage) ([]models.Msg, error) {
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
