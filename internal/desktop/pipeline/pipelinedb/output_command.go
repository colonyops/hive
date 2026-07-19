package pipelinedb

import (
	"context"
	"database/sql"
	"fmt"
)

// ListPendingOutputCommands returns up to limit output_command rows still
// awaiting execution, oldest first — the output worker drains this queue on
// every tick.
func (db *DB) ListPendingOutputCommands(ctx context.Context, limit int) ([]OutputCommand, error) {
	rows, err := db.queries.ListPendingOutputCommands(ctx, int64(limit))
	if err != nil {
		return nil, fmt.Errorf("listing pending output commands: %w", err)
	}
	return rows, nil
}

// MarkOutputCommandDone records a successful execution.
func (db *DB) MarkOutputCommandDone(ctx context.Context, id int64) error {
	if err := db.queries.MarkOutputCommandDone(ctx, id); err != nil {
		return fmt.Errorf("marking output command %d done: %w", id, err)
	}
	return nil
}

// RetryOutputCommand records a failed execution attempt (incrementing
// attempts and recording lastErr) but leaves the command "pending" so the
// worker picks it up again on a later tick. Callers should stop retrying and
// call MarkOutputCommandFailed instead once attempts reaches the worker's
// retry cap.
func (db *DB) RetryOutputCommand(ctx context.Context, id int64, lastErr string) error {
	if err := db.queries.RetryOutputCommand(ctx, RetryOutputCommandParams{
		ID:        id,
		LastError: sql.NullString{String: lastErr, Valid: true},
	}); err != nil {
		return fmt.Errorf("recording retry attempt for output command %d: %w", id, err)
	}
	return nil
}

// MarkOutputCommandFailed records a terminal failure: the command's status
// becomes "failed" (it will not be retried again), with attempts
// incremented and lastErr recorded for diagnosis.
func (db *DB) MarkOutputCommandFailed(ctx context.Context, id int64, lastErr string) error {
	if err := db.queries.MarkOutputCommandFailed(ctx, MarkOutputCommandFailedParams{
		ID:        id,
		LastError: sql.NullString{String: lastErr, Valid: true},
	}); err != nil {
		return fmt.Errorf("marking output command %d failed: %w", id, err)
	}
	return nil
}
