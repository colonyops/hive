//go:build unix

package timer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"syscall"
)

// Row is the minimal shape needed from a timer row for liveness checks.
// It mirrors the columns the helpers actually read on db.Timer, avoiding a
// direct dependency on internal/data/db (which already imports this package
// for the Status enum — a hard import cycle).
type Row struct {
	ID  string
	Pid sql.NullInt64
}

// MarkInactiveParams is the argument shape for MarkInactiveTimersForSession.
// It mirrors db.MarkInactiveTimersForSessionParams so callers can pass the
// generated struct via a small adapter without us depending on the db pkg.
type MarkInactiveParams struct {
	SessionID string
	IDs       []string
}

// InactiveQuerier is the slice of *db.Queries this helper actually needs.
// Defined here so the timer package can stay free of an internal/data/db
// import (db -> timer for Status would otherwise be cyclic).
type InactiveQuerier interface {
	ActiveTimersForSession(ctx context.Context, sessionID string) ([]Row, error)
	ActiveTimersAll(ctx context.Context) ([]Row, error)
	MarkInactiveTimersForSession(ctx context.Context, arg MarkInactiveParams) error
	MarkInactiveTimersAll(ctx context.Context, ids []string) error
}

// MarkInactiveForSession marks any active timers for sessionID whose PID is
// no longer alive as 'orphaned'. Returns the number of rows transitioned.
//
// PID-recycling caveat (Linux / macOS): the kernel may reuse a PID shortly
// after the original process exits. syscall.Kill(pid, 0) against a recycled
// PID returns nil and we therefore report the row as "alive" — meaning we
// won't transition it on this pass. This matches hive timer's
// fire-and-pray semantics: the worst case is that the row stays 'active'
// until the next sweep tick re-checks. Window is short and the consequence
// is cosmetic.
func MarkInactiveForSession(ctx context.Context, q InactiveQuerier, sessionID string) (int, error) {
	rows, err := q.ActiveTimersForSession(ctx, sessionID)
	if err != nil {
		return 0, fmt.Errorf("list active timers for session %q: %w", sessionID, err)
	}
	deadIDs := collectDeadIDs(rows)
	if len(deadIDs) == 0 {
		return 0, nil
	}
	if err := q.MarkInactiveTimersForSession(ctx, MarkInactiveParams{
		SessionID: sessionID,
		IDs:       deadIDs,
	}); err != nil {
		return 0, fmt.Errorf("mark inactive timers for session %q: %w", sessionID, err)
	}
	return len(deadIDs), nil
}

// MarkInactiveAll is the unscoped variant used by the background sweep.
// Same PID-recycling caveat applies — see MarkInactiveForSession.
func MarkInactiveAll(ctx context.Context, q InactiveQuerier) (int, error) {
	rows, err := q.ActiveTimersAll(ctx)
	if err != nil {
		return 0, fmt.Errorf("list active timers: %w", err)
	}
	deadIDs := collectDeadIDs(rows)
	if len(deadIDs) == 0 {
		return 0, nil
	}
	if err := q.MarkInactiveTimersAll(ctx, deadIDs); err != nil {
		return 0, fmt.Errorf("mark inactive timers: %w", err)
	}
	return len(deadIDs), nil
}

// collectDeadIDs filters rows down to those whose PID is set and the PID is
// no longer alive. Rows with NULL pid are skipped — the SQL clauses already
// exclude them, but we double-check defensively.
func collectDeadIDs(rows []Row) []string {
	dead := []string{}
	for _, r := range rows {
		if !r.Pid.Valid {
			continue
		}
		if !pidAlive(int(r.Pid.Int64)) {
			dead = append(dead, r.ID)
		}
	}
	return dead
}

// pidAlive returns true if signaling pid with 0 succeeds (process exists and
// we have permission to signal it) or returns EPERM (process exists but we
// don't own it). ESRCH means the process is gone. Any other error is
// treated as alive — defensive, since we don't want to spuriously orphan a
// row on an unexpected errno.
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	if errors.Is(err, syscall.ESRCH) {
		return false
	}
	// EPERM or anything else: treat as alive.
	return true
}
