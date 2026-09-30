package queries

import (
	"context"
	"fmt"
	"time"
)

func wrap(msg string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", msg, err)
}

const interruptedOutputCommandError = "interrupted: application stopped while action was running"

// RecoverInterruptedOutputCommands makes stale explicit invocations and their
// linked jobs terminal. It also fails unlinked queued jobs, which in v1 can only
// be left by a crash between Begin and Running. A running command may already
// have performed its side effect before a crash, so retrying it in the
// background would be unauthorized and unsafe.
//
// Open runs this at startup before any store exists, and it writes both job
// and output_command rows, so it lives on DB (placement clause 4).
func (db *DB) RecoverInterruptedOutputCommands(ctx context.Context) error {
	return db.WithinTx(ctx, func(ctx context.Context, tx *DB) error {
		if _, err := tx.querier().ExecContext(ctx, `
		UPDATE job
		SET status = 'failed', step = 'Failed', error = ?, updated_at = ?
		WHERE (status = 'queued' AND command_id IS NULL)
			OR (status = 'running'
				AND command_id IN (SELECT id FROM output_command WHERE status = 'running'))`,
			interruptedOutputCommandError, time.Now().UnixMilli()); err != nil {
			return wrap("recovering interrupted jobs", err)
		}
		if _, err := tx.querier().ExecContext(ctx, `
		UPDATE output_command
		SET status = 'failed', attempts = attempts + 1, last_error = ?
		WHERE status = 'running'`, interruptedOutputCommandError); err != nil {
			return wrap("recovering interrupted output commands", err)
		}
		return nil
	})
}
