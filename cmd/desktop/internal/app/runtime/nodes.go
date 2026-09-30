package runtime

import (
	"context"
	"maps"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources"
)

// behavior is what a node type does when a message reaches it. Exactly one of
// the three fields is set.
//
// This is the runtime half of the node-type declaration whose config half is
// flow.registry: adding a node type is one line here and one line there, and
// Run never branches on a type string. A behavior with none of the three set
// is a node the engine does not know how to execute, which Run reports rather
// than silently dropping.
type behavior struct {
	// relay marks a node the core ingests for. The producer and the webhook
	// listener append its observations to the event log, so at run time it
	// only passes what was routed to it on to its wires.
	relay bool
	// sinks derives the committed side effects one message reaching this
	// terminal produces. Terminals have no outputs, so this is where a
	// message's journey ends.
	sinks func(flowID, nodeID string, cfg flow.NodeConfig, msg models.Msg) []models.Output
	// processor builds this node's per-instance transformer. It is called once
	// per node when a Runner is built.
	processor func(r *Runner, nodeID string, cfg flow.NodeConfig) (processor, error)
	// snapshotReconciled marks a node whose committed sink output is
	// membership in a set that a source snapshot restates wholesale — a
	// feed. Run declares one reconciliation scope per (node, source) so the
	// commit can drop rows the latest snapshot no longer includes, even when
	// the snapshot is empty, and the engine derives a flow's feed ids from
	// the same flag to know what replay must recompute membership for and,
	// on disable, must remove. It is meaningless on a behavior with no sinks.
	snapshotReconciled bool
	// kvCapable marks a node that owns durable node KV. The engine derives
	// the ids ActivateReplay retains from this flag; a node type missing it
	// has its rows reconciled away on the next activation.
	kvCapable bool
}

// processor transforms one message into port-indexed outputs. A nil result
// (or one whose ports are all empty) discards the message.
type processor interface {
	process(ctx context.Context, msg models.Msg, kv NodeKV, console ConsoleSink) ([][]models.Msg, error)
	// reset drops whatever state the processor accumulated for its node,
	// so the next message starts clean. The engine calls it after a timeout —
	// the "terminate, respawn" the browser gave a wedged worker.
	reset()
	// close releases the processor's resources.
	close()
}

// behaviors is the runtime registry, keyed by the same type strings
// flow.registry uses.
//
// Source types are not listed: a source is a relay by definition — the core
// ingests for it, so at run time it only forwards what was routed to it — and
// deriving them from the connector registry is what keeps adding a connector
// a change to internal/app/sources alone.
var behaviors = buildBehaviors(map[string]behavior{
	"github-filter": {processor: newFilterNode},
	"function":      {processor: newFunctionNode, kvCapable: true},
	"feed":          {sinks: feedSinks, snapshotReconciled: true},
	"action":        {sinks: actionSinks},
	"notify":        {sinks: notifySinks},
})

// buildBehaviors merges the behaviours declared here with the relay behaviour
// every registered source connector has. A collision panics for the same
// reason flow.buildRegistry's does: both maps are compile-time constants, so
// there is nothing to resolve to at run time.
func buildBehaviors(declared map[string]behavior) map[string]behavior {
	out := make(map[string]behavior, len(declared))
	maps.Copy(out, declared)
	for _, connectorType := range sources.Types() {
		if _, clash := out[connectorType]; clash {
			panic("runtime: source connector " + connectorType + " collides with a declared node type")
		}
		out[connectorType] = behavior{relay: true}
	}
	return out
}

// feedSinks claims immutable inbox membership for the arriving item. A feed
// is a pure inbox surface: it never interrupts — raising a notification is a
// notify node's job.
//
// The payload rides along so the commit can mint an inbox row for a key that
// never went through ingest — a function node that split one source message
// into per-entity items with keys it minted (see models.CommitBatch). For a key
// the producer already ingested, the payload is redundant and the row's own
// classifier-owned presentation wins.
func feedSinks(flowID, nodeID string, _ flow.NodeConfig, msg models.Msg) []models.Output {
	return []models.Output{{
		Sink:        models.Sink{Kind: models.SinkKindFeed, TargetID: flowID + "/" + nodeID},
		Key:         msg.Key,
		Payload:     msg.Payload,
		SourceTopic: msg.Topic,
		SourceKind:  msg.SourceKind,
		SourceScope: msg.SourceScope,
	}}
}

// actionSinks enqueues an output_command against the referenced actions.yml
// action. An action runs over the payload and dedups on the message's own
// occurrence key, so the source identity is carried for attribution only.
func actionSinks(_, _ string, cfg flow.NodeConfig, msg models.Msg) []models.Output {
	config, ok := cfg.(*flow.ActionConfig)
	if !ok {
		return nil
	}
	return []models.Output{{
		Sink:          models.Sink{Kind: models.SinkKindAction, TargetID: config.Action},
		Key:           msg.Key,
		OccurrenceKey: msg.OccurrenceKey,
		Payload:       msg.Payload,
		SourceKind:    msg.SourceKind,
		SourceScope:   msg.SourceScope,
		SourceTopic:   "",
	}}
}

// notifySinks raises a notification for whatever is routed to it. The output
// carries the message's source identity as well as its payload so the backend
// can resolve the inbox row behind it and a clicked notification can reveal
// that item.
func notifySinks(flowID, nodeID string, _ flow.NodeConfig, msg models.Msg) []models.Output {
	return []models.Output{{
		Sink:          models.Sink{Kind: models.SinkKindNotify, TargetID: flowID + "/" + nodeID},
		Key:           msg.Key,
		OccurrenceKey: msg.OccurrenceKey,
		Payload:       msg.Payload,
		SourceTopic:   msg.Topic,
		SourceKind:    msg.SourceKind,
		SourceScope:   msg.SourceScope,
	}}
}
