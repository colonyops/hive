package queries

import (
	"context"
	"fmt"
)

// resetTables is every table holding mutable pipeline state, ordered so child
// rows are deleted before the inbox items they reference rather than relying
// on cascade behavior. The migration bookkeeping table is deliberately not
// listed: schema state must survive a reset.
var resetTables = []string{
	"inbox_event",
	"feed_membership_claim",
	"inbox_item",
	"event_log",
	"consumer_offset",
	"source_head",
	"node_kv",
	"output_command",
	"node_run",
	"activity_event",
	"job",
	"webhook_capture",
	"agent_workspace_session",
	"schedule_run",
	"schedule_cursor",
}

// Seeder writes the baseline rows a reset restores. It runs inside the wipe
// transaction, so implementations must use the supplied context to join it
// through their existing store and never open another transaction.
type Seeder interface {
	Seed(ctx context.Context) error
}

// ResetAllState deletes every row from every mutable pipeline table, resets
// the tables' AUTOINCREMENT counters, and then runs reseed (when non-nil)
// against the same transaction. Because the wipe and reseed share one
// transaction, a concurrent reader observes either the old state or the
// reseeded baseline -- never an intermediate empty store. The connection stays
// open throughout, and row ids and event offsets subsequently restart from 1
// exactly as in a freshly created database. This exists for the test-only
// /_e2e/reset harness and must never run against live user state.
func (db *DB) ResetAllState(ctx context.Context, reseed Seeder) error {
	return db.WithinTx(ctx, func(ctx context.Context, tx *DB) error {
		for _, table := range resetTables {
			if _, err := tx.querier().ExecContext(ctx, `DELETE FROM `+table); err != nil {
				return fmt.Errorf("clearing table %s: %w", table, err)
			}
			// AUTOINCREMENT counters live in sqlite_sequence rows named after
			// their table; scoping the delete to resetTables leaves any other
			// sequence untouched.
			if _, err := tx.querier().ExecContext(ctx, `DELETE FROM sqlite_sequence WHERE name = ?`, table); err != nil {
				return fmt.Errorf("resetting sequence for table %s: %w", table, err)
			}
		}
		if reseed != nil {
			return reseed.Seed(ctx)
		}
		return nil
	})
}
