package pipelinedb

import (
	"context"
	"fmt"
)

// RetentionPolicy bounds diagnostic history retained by the desktop pipeline.
// Event-log retention is intentionally not count- or age-based: it advances
// only through the minimum durable offset of every enabled flow consumer.
type RetentionPolicy struct {
	// NodeRunLimit is the total number of newest node_run rows to retain.
	NodeRunLimit int64
	// TerminalOutputCommandLimit is the number of newest done/failed
	// output_command rows to retain. Non-terminal commands are never pruned.
	TerminalOutputCommandLimit int64
}

// DefaultRetentionPolicy keeps enough recent history for the desktop's debug
// views while bounding SQLite growth from long-running pipelines.
func DefaultRetentionPolicy() RetentionPolicy {
	return RetentionPolicy{
		NodeRunLimit:               10_000,
		TerminalOutputCommandLimit: 2_000,
	}
}

// RetentionResult describes the safe event-log boundary selected by Prune.
// EventLogThrough is zero when no event rows were eligible, either because
// there are no enabled consumers or at least one has no durable offset yet.
type RetentionResult struct {
	EventLogThrough int64
}

// Prune applies the retention policy in one transaction. An event is deleted
// only after every enabled flow consumer has durably committed at or beyond
// its offset. A newly enabled flow with no consumer_offset deliberately blocks
// event-log pruning, preserving its complete backlog. Disabled flows do not
// hold retention because re-enabling one intentionally starts from the then
// current log rather than replaying data accumulated while it was disabled.
//
// Node runs and terminal output-command rows are bounded independently. The
// latter intentionally excludes pending, awaiting_confirmation, running, and
// retryable commands; losing any of those could drop a side effect.
func (db *DB) Prune(ctx context.Context, enabledConsumers []string, policy RetentionPolicy) (RetentionResult, error) {
	if policy.NodeRunLimit < 0 {
		return RetentionResult{}, fmt.Errorf("node run retention limit must not be negative")
	}
	if policy.TerminalOutputCommandLimit < 0 {
		return RetentionResult{}, fmt.Errorf("terminal output command retention limit must not be negative")
	}

	enabled := uniqueConsumers(enabledConsumers)
	result := RetentionResult{}
	err := db.WithTx(ctx, func(q *Queries) error {
		if len(enabled) > 0 {
			offsets, err := q.ListConsumerOffsets(ctx)
			if err != nil {
				return fmt.Errorf("listing consumer offsets: %w", err)
			}

			byConsumer := make(map[string]int64, len(offsets))
			for _, offset := range offsets {
				byConsumer[offset.Consumer] = offset.Offset
			}

			minOffset := int64(0)
			allCommitted := true
			for i, consumer := range enabled {
				offset, ok := byConsumer[consumer]
				if !ok {
					allCommitted = false
					break
				}
				if i == 0 || offset < minOffset {
					minOffset = offset
				}
			}
			if allCommitted && minOffset > 0 {
				if err := q.DeleteEventsThrough(ctx, minOffset); err != nil {
					return fmt.Errorf("pruning event log through %d: %w", minOffset, err)
				}
				result.EventLogThrough = minOffset
			}
		}

		if err := q.PruneNodeRuns(ctx, policy.NodeRunLimit); err != nil {
			return fmt.Errorf("pruning node runs: %w", err)
		}
		if err := q.PruneTerminalOutputCommands(ctx, policy.TerminalOutputCommandLimit); err != nil {
			return fmt.Errorf("pruning terminal output commands: %w", err)
		}
		return nil
	})
	if err != nil {
		return RetentionResult{}, err
	}
	return result, nil
}

func uniqueConsumers(consumers []string) []string {
	seen := make(map[string]struct{}, len(consumers))
	unique := make([]string, 0, len(consumers))
	for _, consumer := range consumers {
		if consumer == "" {
			continue
		}
		if _, ok := seen[consumer]; ok {
			continue
		}
		seen[consumer] = struct{}{}
		unique = append(unique, consumer)
	}
	return unique
}
