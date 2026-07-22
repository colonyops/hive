package pipelinedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// Sink kinds: where a committed Output is written.
const (
	SinkKindFeed   = "feed"
	SinkKindAction = "action"
)

// Sink identifies where an Output is committed. Feed outputs retain the
// Phase-1 wire shape but have no durable effect until membership claims land;
// action outputs enqueue an output_command.
type Sink struct {
	Kind     string `json:"kind"`
	TargetID string `json:"targetId"`
}

// Output is one committed side effect of a flow run.
type Output struct {
	Sink           Sink            `json:"sink"`
	Key            string          `json:"key"` // output dedup key
	Payload        json.RawMessage `json:"payload"`
	Unread         bool            `json:"unread"` // retained feed-output field
	SourceTopic    string          `json:"sourceTopic,omitempty"`
	SnapshotID     string          `json:"snapshotId,omitempty"`
	PreserveUnread bool            `json:"preserveUnread,omitempty"`
}

// FeedSnapshot declares one source's complete current output scope for a feed.
// CommitBatch accepts these declarations but does not persist reconciliation
// state until membership claims are introduced.
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

// NodeRunView is one node's per-tick execution summary, recorded for the
// flows debug/status UI. It is named "View" (rather than NodeRun) only to
// avoid colliding with the sqlc-generated raw row model of the same name in
// models.go — package pipeline's NodeRun alias re-exports this type under
// the name callers actually use (see pipeline/commit.go).
type NodeRunView struct {
	FlowID    string `json:"flowId"`
	NodeID    string `json:"nodeId"`
	OK        bool   `json:"ok"`
	InCount   int    `json:"inCount"`
	OutCount  int    `json:"outCount"`
	DropCount int    `json:"dropCount"`
	Err       string `json:"err"`
	DurMs     int64  `json:"durMs"`
}

// CommitBatch is the frontend graph runtime's atomic write: it advances a
// consumer's committed offset and persists the outputs/node-run metrics
// produced while processing up to that offset, all in one transaction (see
// DB.CommitBatch).
type CommitBatch struct {
	Consumer      string         `json:"consumer"`   // event_log consumer key (flow id / consumer id)
	UpToOffset    string         `json:"upToOffset"` // decimal event-log offset; strings preserve int64 precision across Wails
	Outputs       []Output       `json:"outputs"`
	FeedSnapshots []FeedSnapshot `json:"feedSnapshots"`
	Discards      []Discard      `json:"discards"`
	NodeRuns      []NodeRunView  `json:"nodeRuns"`
}

// CommitBatch applies b atomically: feed outputs and snapshots are accepted
// but currently have no durable effect, action outputs are enqueued into
// output_command (deduped by (action_id, key)), node runs are recorded, and
// the consumer's offset advances to b.UpToOffset.
//
// Idempotency by offset: if b.UpToOffset is at or below the consumer's
// currently committed offset, this batch was already applied in a previous
// commit and the call is a no-op without touching output_command or node_run.
// Only output_command needs its own dedup key, since two different batches
// could legitimately enqueue the same action.
func (db *DB) CommitBatch(ctx context.Context, b CommitBatch) error {
	offset, err := strconv.ParseInt(b.UpToOffset, 10, 64)
	if err != nil || offset < 0 {
		return fmt.Errorf("parsing commit offset %q: expected a non-negative decimal int64", b.UpToOffset)
	}

	return db.WithTx(ctx, func(q *Queries) error {
		current, err := q.GetConsumerOffset(ctx, b.Consumer)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("reading committed offset for consumer %q: %w", b.Consumer, err)
		}

		if offset <= current.Offset {
			// Already applied by a previous commit of this batch (or a
			// stale/out-of-order commit) — no-op.
			return nil
		}

		now := time.Now().UnixMilli()

		for _, out := range b.Outputs {
			switch out.Sink.Kind {
			case SinkKindFeed:
				// Feed persistence is introduced with membership claims in a later phase.
			case SinkKindAction:
				if err := q.EnqueueOutputCommand(ctx, EnqueueOutputCommandParams{
					ActionID:  out.Sink.TargetID,
					Key:       out.Key,
					Payload:   []byte(out.Payload),
					CreatedAt: now,
				}); err != nil {
					return fmt.Errorf("enqueuing output_command %s/%s: %w", out.Sink.TargetID, out.Key, err)
				}
			default:
				return fmt.Errorf("commit batch: unknown sink kind %q", out.Sink.Kind)
			}
		}

		for _, nr := range b.NodeRuns {
			var errCol sql.NullString
			if nr.Err != "" {
				errCol = sql.NullString{String: nr.Err, Valid: true}
			}
			if err := q.InsertNodeRun(ctx, InsertNodeRunParams{
				FlowID:    nr.FlowID,
				NodeID:    nr.NodeID,
				Ok:        boolToInt64(nr.OK),
				InCount:   int64(nr.InCount),
				OutCount:  int64(nr.OutCount),
				DropCount: int64(nr.DropCount),
				Err:       errCol,
				EndedAt:   now,
				DurMs:     nr.DurMs,
			}); err != nil {
				return fmt.Errorf("inserting node_run for %s/%s: %w", nr.FlowID, nr.NodeID, err)
			}
		}

		if err := q.CommitConsumerOffset(ctx, CommitConsumerOffsetParams{
			Consumer: b.Consumer,
			Offset:   offset,
		}); err != nil {
			return fmt.Errorf("advancing consumer offset for %q: %w", b.Consumer, err)
		}

		return nil
	})
}

func boolToInt64(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
