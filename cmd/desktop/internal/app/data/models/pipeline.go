package models

import (
	"encoding/json"
	"strings"
)

// Msg is the pipeline's generic log record, appended by sources and consumed
// by the graph runtime (internal/app/runtime).
//
// It mirrors the design's { id, key, topic, ts, payload } contract,
// with Snapshot populated only for an authoritative full-source snapshot,
// mapped onto the event_log schema (see queries/migrations/0001_pipeline.up.sql):
//   - ID is derived from the row's "offset" (stable, unique per append; there
//     is no separate id column) and stays a decimal string rather than an
//     int64. A function node's script sees this value as a genuine
//     JavaScript value crossing goja's JSON.parse/JSON.stringify boundary
//     (runtime/js.go), including round-tripping through a script that
//     returns the message it was handed — and a JS number cannot represent
//     an int64 exactly past 2^53. The string is what keeps a large offset
//     intact for the script.
//   - Ts is the row's created_at (unix milliseconds).
//   - Snapshot is nil for ordinary item events and contains the full current
//     source item set for successful poll snapshots.
type Msg struct {
	ID      string
	Key     string
	Topic   string
	Ts      int64
	Payload json.RawMessage
	// Snapshot must NOT be omitempty: an empty snapshot (a successful poll
	// that returned zero items) marshals as [] and the frontend engine's
	// `msg.Snapshot != null` routing depends on it. With omitempty the field
	// vanishes, the boundary row (key "") is routed as an ordinary item, and
	// CommitBatch fails resolving inbox item "<kind>//" forever — wedging the
	// consumer at that offset.
	Snapshot      []SnapshotItem
	SourceKind    string
	SourceScope   string
	OccurrenceKey string `json:"OccurrenceKey,omitempty"`
}

// SnapshotItem is one current source item carried by a successful source
// snapshot. A snapshot event is distinct from ordinary changed-item events:
// it is emitted on every successful poll, including when the source is empty.
type SnapshotItem struct {
	Key     string          `json:"key"`
	Payload json.RawMessage `json:"payload"`
}

// Sink kinds: where a committed Output is written.
const (
	SinkKindFeed   = "feed"
	SinkKindAction = "action"
	SinkKindNotify = "notify"
)

// Sink identifies where an Output is committed. Feed outputs claim immutable
// inbox membership; action and notify outputs enqueue an output_command.
type Sink struct {
	Kind     string `json:"kind"`
	TargetID string `json:"targetId"`
}

// NotifyActionPrefix namespaces the synthetic action id a notify terminal
// enqueues its output_command under, so one queue serves both authored
// actions.yml actions and inline notify nodes. Authored action ids are slugs
// (`^[a-z0-9][a-z0-9-]*$`), which can contain neither ":" nor "/", so a
// synthetic id can never collide with one.
const NotifyActionPrefix = "notify:"

// NotifyActionID returns the synthetic action id for a notify node, whose
// target is the flow-qualified node id "<flowId>/<nodeId>".
func NotifyActionID(target string) string { return NotifyActionPrefix + target }

// NotifyActionTarget inverts NotifyActionID: it returns the flow-qualified
// node id an action id names, and whether the id is a notify action at all.
func NotifyActionTarget(actionID string) (string, bool) {
	target, ok := strings.CutPrefix(actionID, NotifyActionPrefix)
	return target, ok && target != ""
}

// NotifyCommand is the durable payload of a notify terminal's
// output_command. It carries the triggering item's identity alongside its
// payload: the identity resolves the inbox row a delivered notification
// links back to (so clicking the banner can select that item), while Item is
// what the node's title/body templates render over — the same shape an
// action node's executor sees, so `{{ .Payload.title }}` means the same
// thing in both.
type NotifyCommand struct {
	ProfileID   string          `json:"profileId"`
	ExternalID  string          `json:"externalId,omitempty"`
	SourceKind  string          `json:"sourceKind,omitempty"`
	SourceScope string          `json:"sourceScope,omitempty"`
	Item        json.RawMessage `json:"item,omitempty"`
}

// Output is one committed side effect of a flow run.
type Output struct {
	Sink          Sink            `json:"sink"`
	Key           string          `json:"key,omitempty"`           // the item's external ID; not set by a keyless snapshot boundary
	OccurrenceKey string          `json:"occurrenceKey,omitempty"` // action dedup key only
	Payload       json.RawMessage `json:"payload,omitempty"`       // action payload only
	SourceKind    string          `json:"sourceKind,omitempty"`
	SourceScope   string          `json:"sourceScope,omitempty"`
	SourceTopic   string          `json:"sourceTopic"`
	SnapshotID    string          `json:"snapshotId,omitempty"`
}

// FeedSnapshot declares one source's complete current output scope for a feed.
// EventLogStore.Commit reconciles the (feed, source) scope's membership claims
// down to the feed outputs carrying this SnapshotID.
type FeedSnapshot struct {
	FeedID      string `json:"feedId"`
	SourceTopic string `json:"sourceTopic"`
	SnapshotID  string `json:"snapshotId"`
}

// Discard records a message a node dropped instead of forwarding, for
// metrics/observability. CommitBatch does not persist Discards as rows —
// they exist for callers that want to log or count them; the per-node
// aggregate is expected to already be reflected in the corresponding
// NodeRun.DropCount.
type Discard struct {
	MsgID  string `json:"msgId"`
	NodeID string `json:"nodeId"`
}

// NodeRun is one node's per-tick execution summary, recorded for the flows
// debug/status UI.
type NodeRun struct {
	FlowID    string `json:"flowId"`
	NodeID    string `json:"nodeId"`
	OK        bool   `json:"ok"`
	InCount   int    `json:"inCount"`
	OutCount  int    `json:"outCount"`
	DropCount int    `json:"dropCount"`
	Err       string `json:"err"`
	DurMs     int64  `json:"durMs"`
}

// KVMutation is one node's durable KV write, flushed inside CommitBatch so
// it is durable iff the tick that produced it commits. The row's flow_id is
// CommitBatch.Consumer.
type KVMutation struct {
	NodeID    string `json:"nodeId"`
	Key       string `json:"key"`
	Delete    bool   `json:"delete,omitempty"`
	Value     string `json:"value,omitempty"`
	ExpiresAt int64  `json:"expiresAt,omitempty"` // unix ms, 0 = no expiry
}

// CommitBatch is the graph runtime's (internal/app/runtime) atomic write: it
// advances a consumer's committed offset and persists the outputs/node-run
// metrics produced while processing up to that offset, all in one transaction
// (see stores.EventLogStore.Commit).
type CommitBatch struct {
	Consumer      string         `json:"consumer"` // event_log consumer key (flow id / consumer id)
	UpToOffset    int64          `json:"upToOffset"`
	Outputs       []Output       `json:"outputs"`
	FeedSnapshots []FeedSnapshot `json:"feedSnapshots"`
	Discards      []Discard      `json:"discards"`
	NodeRuns      []NodeRun      `json:"nodeRuns"`
	KVMutations   []KVMutation   `json:"kvMutations,omitempty"` // omitempty: invisible to fixtures with none
}

// FeedClaim is one item's membership in one feed, attributed to the source
// that produced it. The engine builds these for ActivateReplay; no store
// reads one back.
type FeedClaim struct {
	ProfileID string `json:"profileId"`
	FeedID    string `json:"feedId"`
	ItemID    int64  `json:"itemId"`
	SourceID  string `json:"sourceId"`
}

// ItemRef identifies an inbox item by inbox_item's own UNIQUE key rather than
// by its row id, which ActivateReplay does not preserve (ADR
// an-item-session-link-is-desktop-state-keyed-on-item-coordinates).
type ItemRef struct {
	ProfileID   string `json:"profileId"`
	SourceKind  string `json:"sourceKind"`
	SourceScope string `json:"sourceScope"`
	ExternalID  string `json:"externalId"`
}

// Known reports whether the ref names an item at all. A profile and an
// external id are the parts that cannot be empty for a real inbox row; source
// scope legitimately is (a connector that fetches as no account).
func (r ItemRef) Known() bool { return r.ProfileID != "" && r.ExternalID != "" }
