package pipelinedb

import (
	"context"
	"fmt"
	"time"
)

// RetentionPolicy bounds diagnostic history retained by the desktop pipeline.
// Event log retention is independent of consumer progress: consumers rebuild
// membership from the inbox on startup/deploy instead of requiring a backlog.
type RetentionPolicy struct {
	// EventLogMaxAge removes events older than this age. Zero disables the age
	// bound; Phase 5 owns product defaults.
	EventLogMaxAge time.Duration
	// EventLogPerTopicLimit retains this many newest events per topic. Zero
	// disables the count bound.
	EventLogPerTopicLimit int64
	// NodeRunLimit is the total number of newest node_run rows to retain.
	NodeRunLimit int64
	// TerminalOutputCommandLimit is the number of newest done/failed
	// output_command rows to retain. Non-terminal commands are never pruned.
	TerminalOutputCommandLimit int64
	// ActivityEventLimit is the total number of newest activity_event rows to
	// retain for the Activity view's audit history.
	ActivityEventLimit int64
	// JobLimit is the number of newest done/failed job rows to retain.
	// Non-terminal jobs are never pruned.
	JobLimit int64
}

// DefaultRetentionPolicy keeps enough recent history for the desktop's debug
// views while bounding SQLite growth from long-running pipelines.
func DefaultRetentionPolicy() RetentionPolicy {
	return RetentionPolicy{
		NodeRunLimit:               10_000,
		TerminalOutputCommandLimit: 2_000,
		ActivityEventLimit:         5_000,
		JobLimit:                   2_000,
	}
}

// RetentionResult is retained for callers that report pruning outcomes.
// EventLogThrough is no longer consumer-derived and remains zero.
type RetentionResult struct {
	EventLogThrough int64
}

// Prune applies age and per-topic-count event-log bounds independently of
// consumer liveness. enabledConsumers remains in the signature for the
// maintenance interface but intentionally has no retention effect.
//
// Node runs, terminal output-command rows, activity events, and terminal jobs
// remain bounded independently. Command and job retention excludes nonterminal
// rows so active work is never discarded.
func (db *DB) Prune(ctx context.Context, _ []string, policy RetentionPolicy) (RetentionResult, error) {
	if policy.EventLogMaxAge < 0 {
		return RetentionResult{}, fmt.Errorf("event log maximum age must not be negative")
	}
	if policy.EventLogPerTopicLimit < 0 {
		return RetentionResult{}, fmt.Errorf("event log per-topic limit must not be negative")
	}
	if policy.NodeRunLimit < 0 {
		return RetentionResult{}, fmt.Errorf("node run retention limit must not be negative")
	}
	if policy.TerminalOutputCommandLimit < 0 {
		return RetentionResult{}, fmt.Errorf("terminal output command retention limit must not be negative")
	}
	if policy.ActivityEventLimit < 0 {
		return RetentionResult{}, fmt.Errorf("activity event retention limit must not be negative")
	}
	if policy.JobLimit < 0 {
		return RetentionResult{}, fmt.Errorf("job retention limit must not be negative")
	}

	result := RetentionResult{}
	err := db.WithTx(ctx, func(q *Queries) error {
		if policy.EventLogMaxAge > 0 {
			if err := q.DeleteEventsOlderThan(ctx, time.Now().Add(-policy.EventLogMaxAge).UnixMilli()); err != nil {
				return fmt.Errorf("pruning old event log rows: %w", err)
			}
		}
		if policy.EventLogPerTopicLimit > 0 {
			if err := q.DeleteEventsOverLimitPerTopic(ctx, policy.EventLogPerTopicLimit); err != nil {
				return fmt.Errorf("pruning excess event log rows: %w", err)
			}
		}

		if err := q.PruneNodeRuns(ctx, policy.NodeRunLimit); err != nil {
			return fmt.Errorf("pruning node runs: %w", err)
		}
		if err := q.PruneTerminalOutputCommands(ctx, policy.TerminalOutputCommandLimit); err != nil {
			return fmt.Errorf("pruning terminal output commands: %w", err)
		}
		if err := q.PruneActivityEvents(ctx, policy.ActivityEventLimit); err != nil {
			return fmt.Errorf("pruning activity events: %w", err)
		}
		if err := q.PruneTerminalJobs(ctx, policy.JobLimit); err != nil {
			return fmt.Errorf("pruning terminal jobs: %w", err)
		}
		return nil
	})
	if err != nil {
		return RetentionResult{}, err
	}
	return result, nil
}
