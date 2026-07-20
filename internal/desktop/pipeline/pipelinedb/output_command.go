package pipelinedb

import (
	"context"
	"database/sql"
	"fmt"
)

// ListRunnableOutputCommands returns up to limit output_command rows ready
// for automatic execution, in ID order. Commands awaiting confirmation are
// intentionally excluded so they cannot head-of-line block automatic work.
func (db *DB) ListRunnableOutputCommands(ctx context.Context, limit int) ([]OutputCommand, error) {
	rows, err := db.queries.ListRunnableOutputCommands(ctx, int64(limit))
	if err != nil {
		return nil, fmt.Errorf("listing runnable output commands: %w", err)
	}
	return rows, nil
}

// ListRunnableOutputCommandsAfter returns runnable rows with IDs greater
// than afterID. The worker uses it to continue a bounded scan after moving
// manual commands out of the runnable queue.
func (db *DB) ListRunnableOutputCommandsAfter(ctx context.Context, afterID int64, limit int) ([]OutputCommand, error) {
	rows, err := db.queries.ListRunnableOutputCommandsAfter(ctx, ListRunnableOutputCommandsAfterParams{
		ID:    afterID,
		Limit: int64(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("listing runnable output commands after %d: %w", afterID, err)
	}
	return rows, nil
}

// MarkOutputCommandAwaitingConfirmation moves a manual action command out
// of the runnable queue until it is explicitly confirmed or auto-apply is
// enabled for its action.
func (db *DB) MarkOutputCommandAwaitingConfirmation(ctx context.Context, id int64) error {
	if err := db.queries.MarkOutputCommandAwaitingConfirmation(ctx, id); err != nil {
		return fmt.Errorf("marking output command %d awaiting confirmation: %w", id, err)
	}
	return nil
}

// PromoteOutputCommandsAwaitingConfirmation makes all confirmation-gated
// commands for actionID runnable after that action enables auto-apply.
func (db *DB) PromoteOutputCommandsAwaitingConfirmation(ctx context.Context, actionID string) error {
	if err := db.queries.PromoteOutputCommandsAwaitingConfirmation(ctx, actionID); err != nil {
		return fmt.Errorf("promoting output commands for action %q: %w", actionID, err)
	}
	return nil
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
