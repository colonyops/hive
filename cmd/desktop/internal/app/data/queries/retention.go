package queries

import (
	"context"
	"database/sql"
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
	// EventLogSnapshotsPerTopicLimit retains this many newest source snapshots
	// per topic. A full-source snapshot dwarfs an item event and older
	// snapshots are fully superseded by the newest, so this — not the
	// per-topic row bound, which counts item events too — is what caps
	// event_log's size. Zero disables the bound; any positive value keeps the
	// single latest snapshot the age/count bounds also preserve.
	EventLogSnapshotsPerTopicLimit int64
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
	// ArchivedItemRetention retains archived inbox items for this duration.
	ArchivedItemRetention time.Duration
	// EventPerItemLimit retains this many newest inbox events per item.
	EventPerItemLimit int64
}

// DefaultRetentionPolicy keeps enough recent history for the desktop's debug
// views while bounding SQLite growth from long-running pipelines.
func DefaultRetentionPolicy() RetentionPolicy {
	return RetentionPolicy{
		NodeRunLimit:                   10_000,
		TerminalOutputCommandLimit:     2_000,
		ActivityEventLimit:             5_000,
		JobLimit:                       2_000,
		ArchivedItemRetention:          90 * 24 * time.Hour,
		EventPerItemLimit:              500,
		EventLogSnapshotsPerTopicLimit: 3,
	}
}

// Prune applies age and per-topic-count event-log bounds independently of
// consumer liveness.
//
// Node runs, terminal output-command rows, activity events, and terminal jobs
// remain bounded independently. Command and job retention excludes nonterminal
// rows so active work is never discarded.
func (db *DB) Prune(ctx context.Context, policy RetentionPolicy) error {
	if policy.EventLogMaxAge < 0 {
		return fmt.Errorf("event log maximum age must not be negative")
	}
	if policy.EventLogPerTopicLimit < 0 {
		return fmt.Errorf("event log per-topic limit must not be negative")
	}
	if policy.EventLogSnapshotsPerTopicLimit < 0 {
		return fmt.Errorf("event log snapshots per-topic limit must not be negative")
	}
	if policy.NodeRunLimit < 0 {
		return fmt.Errorf("node run retention limit must not be negative")
	}
	if policy.TerminalOutputCommandLimit < 0 {
		return fmt.Errorf("terminal output command retention limit must not be negative")
	}
	if policy.ActivityEventLimit < 0 {
		return fmt.Errorf("activity event retention limit must not be negative")
	}
	if policy.JobLimit < 0 {
		return fmt.Errorf("job retention limit must not be negative")
	}
	if policy.ArchivedItemRetention < 0 {
		return fmt.Errorf("archived item retention must not be negative")
	}
	if policy.EventPerItemLimit < 0 {
		return fmt.Errorf("event per-item retention limit must not be negative")
	}

	return db.WithinTx(ctx, func(ctx context.Context, tx *DB) error {
		if policy.EventLogMaxAge > 0 {
			if err := tx.DeleteEventsOlderThan(ctx, time.Now().Add(-policy.EventLogMaxAge).UnixMilli()); err != nil {
				return fmt.Errorf("pruning old event log rows: %w", err)
			}
		}
		if policy.EventLogPerTopicLimit > 0 {
			if err := tx.DeleteEventsOverLimitPerTopic(ctx, policy.EventLogPerTopicLimit); err != nil {
				return fmt.Errorf("pruning excess event log rows: %w", err)
			}
		}
		if policy.EventLogSnapshotsPerTopicLimit > 0 {
			if err := tx.DeleteSnapshotsOverLimitPerTopic(ctx, policy.EventLogSnapshotsPerTopicLimit); err != nil {
				return fmt.Errorf("pruning superseded source snapshots: %w", err)
			}
		}

		if err := tx.PruneNodeRuns(ctx, policy.NodeRunLimit); err != nil {
			return fmt.Errorf("pruning node runs: %w", err)
		}
		if err := tx.PruneTerminalOutputCommands(ctx, policy.TerminalOutputCommandLimit); err != nil {
			return fmt.Errorf("pruning terminal output commands: %w", err)
		}
		if err := tx.PruneActivityEvents(ctx, policy.ActivityEventLimit); err != nil {
			return fmt.Errorf("pruning activity events: %w", err)
		}
		if err := tx.PruneTerminalJobs(ctx, policy.JobLimit); err != nil {
			return fmt.Errorf("pruning terminal jobs: %w", err)
		}
		if err := tx.PruneArchivedInboxItems(ctx, sql.NullInt64{Int64: time.Now().Add(-policy.ArchivedItemRetention).UnixMilli(), Valid: true}); err != nil {
			return fmt.Errorf("pruning archived inbox items: %w", err)
		}
		if err := tx.DeleteOrphanedSourceHeads(ctx); err != nil {
			return fmt.Errorf("reclaiming orphaned source heads: %w", err)
		}
		if err := tx.TrimInboxItemEvents(ctx, policy.EventPerItemLimit); err != nil {
			return fmt.Errorf("trimming inbox item events: %w", err)
		}
		// No row-count cap on node_kv on purpose: a pruned "seen" key would
		// make its item re-notify, so bounding relies on TTL and teardown.
		if err := tx.DeleteExpiredNodeKV(ctx, sql.NullInt64{Int64: time.Now().UnixMilli(), Valid: true}); err != nil {
			return fmt.Errorf("sweeping expired node kv: %w", err)
		}
		return nil
	})
}
