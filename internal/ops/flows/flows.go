// Package flows declares the profile, flow and image operations of the
// desktop's app-control MCP server.
package flows

import "github.com/colonyops/hive/internal/ops"

var Toolset = ops.Toolset{Name: "flows", MCPPath: "/mcp"}

func All() []ops.Op {
	return []ops.Op{
		ProfileList, ProfileCreate, ProfileDelete,
		ProfileImageGet, ProfileImagePut, ProfileImageDelete,
		FlowGet, FlowPreview,
		NodeImageGet, NodeImagePut, NodeImageDelete,
	}
}

var ProfileList = ops.Spec[ops.NoInput]{
	Name: "profile.list", Owner: ops.OwnerDesktop, Effect: ops.EffectRead, Toolset: Toolset,
	Summary: "List profiles",
	Guidance: "List every profile with its load status and whether it has an avatar. " +
		"Profile ids from here are what every other tool's profileId argument takes; flow_get turns one into its graph and node ids. " +
		"A profile whose flow file does not parse still lists, with valid=false.",
}

type ProfileCreateIn struct {
	Name string `json:"name" jsonschema:"Display name for the new profile."`
}

var ProfileCreate = ops.Spec[ProfileCreateIn]{
	Name: "profile.create", Owner: ops.OwnerDesktop, Effect: ops.EffectWrite, Toolset: Toolset,
	Summary:  "Create a profile",
	Guidance: "Create a profile, seeded with the starter graph when exactly one GitHub account is connected and empty otherwise.",
	Args:     []string{"name"},
}

// ProfileIn names one profile.
type ProfileIn struct {
	ProfileID string `json:"profileId" jsonschema:"Profile id from profile_list."`
}

var ProfileDelete = ops.Spec[ProfileIn]{
	Name: "profile.delete", Owner: ops.OwnerDesktop, Effect: ops.EffectWrite, Toolset: Toolset,
	Summary: "Delete a profile",
	Guidance: "Delete a profile, its flow files, its avatar, and its inbox state. This is not reversible. " +
		"An id that matches no profile is not_found — nothing is ever reported as deleted that was not.",
	Args: []string{"profileId"},
}

var ProfileImageGet = ops.Spec[ProfileIn]{
	Name: "profile_image.get", Owner: ops.OwnerDesktop, Effect: ops.EffectRead, Toolset: Toolset,
	Summary:  "Read a profile's avatar",
	Guidance: "Return a profile's avatar as a PNG image. A profile with no avatar is not_found — its rail falls back to a letter chip.",
	Args:     []string{"profileId"},
}

type ProfileImagePutIn struct {
	ProfileID   string `json:"profileId"   jsonschema:"Profile id from profile_list."`
	ImageBase64 string `json:"imageBase64" jsonschema:"The image as base64 (PNG, JPEG, GIF or WebP). A data: URL is accepted and its prefix ignored."`
}

var ProfileImagePut = ops.Spec[ProfileImagePutIn]{
	Name: "profile_image.put", Owner: ops.OwnerDesktop, Effect: ops.EffectWrite, Toolset: Toolset,
	Summary: "Set a profile's avatar",
	Guidance: "Set a profile's avatar from base64-encoded image bytes (PNG, JPEG, GIF or WebP — the format is sniffed, so no media type is needed). " +
		"The image is normalized to a 128x128 PNG.",
	Args: []string{"profileId"},
}

var ProfileImageDelete = ops.Spec[ProfileIn]{
	Name: "profile_image.delete", Owner: ops.OwnerDesktop, Effect: ops.EffectWrite, Toolset: Toolset,
	Summary:  "Clear a profile's avatar",
	Guidance: "Clear a profile's avatar so its rail reverts to the letter chip.",
	Args:     []string{"profileId"},
}

var FlowGet = ops.Spec[ProfileIn]{
	Name: "flow.get", Owner: ops.OwnerDesktop, Effect: ops.EffectRead, Toolset: Toolset,
	Summary: "Read a profile's flow",
	Guidance: "Return one profile's graph: every node with its id, type and config, and the wires between them. " +
		"This is where node ids come from — flow_preview's nodeId and the node-image tools take them. " +
		"A profile whose flow file does not parse still answers, with valid=false and the load error.",
	Args: []string{"profileId"},
}

// DetailEmitted is flow_preview's own middle rung between DetailSummary and
// DetailFull: each node's emitted messages but not its received ones. A
// node's input is its upstream's output, and the entry node's is what the
// caller just sent, so received is derivable everywhere.
const DetailEmitted = "emitted"

// FlowPreviewIn is one dry run. Exactly one of profileId, flow and flowYaml
// says which flow to run; the rest is the input and the sandbox it runs
// against.
type FlowPreviewIn struct {
	ProfileID string `json:"profileId,omitempty" jsonschema:"An installed profile's id, enabled or not."`
	// Flow is parsed through the same decoder a flows/<id>.yaml file goes
	// through, so it carries the file's version too.
	Flow     map[string]any `json:"flow,omitempty"     jsonschema:"A flow document as a JSON object, same schema as flows/<id>.yaml including version."`
	FlowYAML string         `json:"flowYaml,omitempty" jsonschema:"That same flow document as YAML text."`
	// ProfileIDOverride only scopes the run's node KV and the feed ids in its
	// outputs; nothing is installed under it.
	ProfileIDOverride string `json:"profileIdOverride,omitempty" jsonschema:"The profile id an inline document runs under; scopes node KV and output feed ids only, and installs nothing."`

	NodeID   string    `json:"nodeId"   jsonschema:"The node messages are delivered to — any node, not only a source. Node ids come from flow_get."`
	Messages []Message `json:"messages" jsonschema:"The messages to deliver."`
	// KV seeds the sandbox. Durable KV is neither read nor written, so this
	// is the whole world a kv.get sees.
	KV     map[string]map[string]any `json:"kv,omitempty"     jsonschema:"Seeds an in-memory KV sandbox as nodeId -> key -> value; durable KV is neither read nor written."`
	Detail string                    `json:"detail,omitempty" jsonschema:"How much of each message body to report — one of summary, emitted (the default) or full."`
}

// Message mirrors the desktop's message envelope for the wire. That envelope
// carries its raw JSON as json.RawMessage, whose Go type is []byte, so the
// schema inferrer would describe Payload as an array and reject the JSON
// object every real payload is (ADR mcp-replaces-the-agent-facing-http-api).
// any is what the schema has to say for arbitrary JSON.
//
// Field names match the envelope's own JSON (Go names, not lowercased) so a
// payload captured from the app round-trips unchanged.
type Message struct {
	ID            string         `json:"ID,omitempty"            jsonschema:"Message id; a synthetic one is generated when empty."`
	Key           string         `json:"Key,omitempty"           jsonschema:"The source's own key for this item."`
	Topic         string         `json:"Topic,omitempty"         jsonschema:"Event topic; defaults to the source node's own topic when injecting at a source."`
	Ts            int64          `json:"Ts,omitempty"            jsonschema:"Unix millisecond timestamp."`
	Payload       any            `json:"Payload,omitempty"       jsonschema:"The item's payload as arbitrary JSON."`
	Snapshot      []SnapshotItem `json:"Snapshot,omitempty"      jsonschema:"A full source snapshot; present makes this a snapshot message, which declares feed reconciliation exactly as a poll would. An empty array is still a snapshot."`
	SourceKind    string         `json:"SourceKind,omitempty"    jsonschema:"The source connector kind."`
	SourceScope   string         `json:"SourceScope,omitempty"   jsonschema:"The source's scope within that kind."`
	OccurrenceKey string         `json:"OccurrenceKey,omitempty" jsonschema:"Distinguishes repeat occurrences of one item."`
}

type SnapshotItem struct {
	Key     string `json:"key"`
	Payload any    `json:"payload" jsonschema:"The item's payload as arbitrary JSON."`
}

var FlowPreview = ops.Spec[FlowPreviewIn]{
	Name: "flow.preview", Owner: ops.OwnerDesktop, Effect: ops.EffectRead, Toolset: Toolset,
	Summary: "Dry-run a flow",
	Guidance: "Dry-run a flow against input you supply and report what every node did, committing nothing — no feed membership, " +
		"inbox rows, notifications, queued actions or durable kv. Name the flow with exactly one of profileId (an installed profile's flow, enabled or not), " +
		"flow (a flow document as a JSON object, same schema as flows/<id>.yaml, version included) or flowYaml (that document as YAML text), " +
		"so an unsaved edit can be executed before it is deployed. Messages are delivered to nodeId — any node, not only a source, which is how " +
		"one function node is exercised in isolation against a captured payload; flow_get is where node ids come from. " +
		"Sources never fetch: a source node relays what you inject. " +
		"Envelope fields left empty are filled in. kv seeds an in-memory sandbox (nodeId -> key -> value) that is the whole world a kv.get sees, " +
		"so notify-once logic is testable against a known starting state; what the run would have written comes back in kvMutations. " +
		"Each node reports its drops, timing, console output, and a structured error with line and column for a script failure, whose line " +
		"numbers are relative to the on_message body you wrote. detail controls how many copies of each message body come back: \"emitted\" " +
		"(the default) reports what each node put on its output ports, \"summary\" drops message bodies entirely — use it for a snapshot run, " +
		"whose payloads would otherwise be repeated once per node per item — and \"full\" adds each node's received messages. " +
		"Prefer this over deploying an edit and waiting for a poll.",
	Fields: ops.Constraints{"detail": {Enum: []string{ops.DetailSummary, DetailEmitted, ops.DetailFull}, Default: DetailEmitted}},
}

// NodeImageIn names one node of a profile's flow.
type NodeImageIn struct {
	ProfileID string `json:"profileId" jsonschema:"Profile id from profile_list."`
	NodeID    string `json:"nodeId"    jsonschema:"Node id within that profile's flow; ids come from flow_get."`
}

var NodeImageGet = ops.Spec[NodeImageIn]{
	Name: "node_image.get", Owner: ops.OwnerDesktop, Effect: ops.EffectRead, Toolset: Toolset,
	Summary:  "Read a source node's feed mark",
	Guidance: "Return a source node's feed-mark image as a PNG. A node with no mark is not_found — its items fall back to the node's glyph.",
	Args:     []string{"profileId", "nodeId"},
}

type NodeImagePutIn struct {
	ProfileID   string `json:"profileId"   jsonschema:"Profile id from profile_list."`
	NodeID      string `json:"nodeId"      jsonschema:"Node id within that profile's flow; ids come from flow_get."`
	ImageBase64 string `json:"imageBase64" jsonschema:"The image as base64 (PNG, JPEG, GIF or WebP). A data: URL is accepted and its prefix ignored."`
}

var NodeImagePut = ops.Spec[NodeImagePutIn]{
	Name: "node_image.put", Owner: ops.OwnerDesktop, Effect: ops.EffectWrite, Toolset: Toolset,
	Summary: "Set a source node's feed mark",
	Guidance: "Set a source node's feed-mark image from base64-encoded image bytes (PNG, JPEG, GIF or WebP). " +
		"It is normalized to a 128x128 PNG and shown on that source's items instead of its icon. " +
		"Webhook and command sources carry a mark; other node types are rejected.",
	Args: []string{"profileId", "nodeId"},
}

var NodeImageDelete = ops.Spec[NodeImageIn]{
	Name: "node_image.delete", Owner: ops.OwnerDesktop, Effect: ops.EffectWrite, Toolset: Toolset,
	Summary:  "Clear a source node's feed mark",
	Guidance: "Clear a source node's feed-mark image so its items revert to the node's icon.",
	Args:     []string{"profileId", "nodeId"},
}
