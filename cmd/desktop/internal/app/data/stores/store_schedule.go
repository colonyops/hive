package stores

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
)

// ScheduleRunLimit is the number of newest runs InsertRun keeps per schedule.
const ScheduleRunLimit = 200

// ScheduleStore owns a schedule's run state: the cursor that says how far it
// was evaluated and the history of its runs. Both are keyed by the schedule's
// (workspace, id); the schedule itself lives in the workspace manifest.
type ScheduleStore struct {
	q         *queries.DB
	mapCursor MapFunc[queries.ScheduleCursor, ScheduleCursor]
	mapRun    MapFunc[queries.ScheduleRun, ScheduleRun]
}

func NewScheduleStore(q *queries.DB, _ Options) *ScheduleStore {
	return &ScheduleStore{q: q, mapCursor: mapScheduleCursorFromDB, mapRun: mapScheduleRunFromDB}
}

// Cursor reports false for a schedule that has never been evaluated.
func (s *ScheduleStore) Cursor(ctx context.Context, workspace, scheduleID string) (ScheduleCursor, bool, error) {
	row, err := s.q.Ctx(ctx).GetScheduleCursor(ctx, queries.GetScheduleCursorParams{Workspace: workspace, ScheduleID: scheduleID})
	if errors.Is(err, sql.ErrNoRows) {
		return ScheduleCursor{}, false, nil
	}
	if err != nil {
		return ScheduleCursor{}, false, wrap(fmt.Sprintf("getting schedule cursor for %s/%s", workspace, scheduleID), err)
	}
	return s.mapCursor(row), true, nil
}

func (s *ScheduleStore) SaveCursor(ctx context.Context, cursor ScheduleCursor) error {
	return wrap(fmt.Sprintf("saving schedule cursor for %s/%s", cursor.Workspace, cursor.ScheduleID),
		s.q.Ctx(ctx).UpsertScheduleCursor(ctx, queries.UpsertScheduleCursorParams(cursor)))
}

func (s *ScheduleStore) ListCursors(ctx context.Context) ([]ScheduleCursor, error) {
	rows, err := s.q.Ctx(ctx).ListScheduleCursors(ctx)
	return s.mapCursor.SliceErr(rows, wrap("listing schedule cursors", err))
}

// PruneCursors deletes every cursor inside workspaces that keep does not
// name, so a deleted schedule's id can be reused without back-firing the
// occurrences its predecessor missed. A cursor in any other workspace is left
// alone: a manifest that momentarily fails to parse must not lose the state
// that says how far its schedules got.
func (s *ScheduleStore) PruneCursors(ctx context.Context, workspaces []string, keep []ScheduleRef) error {
	scope := make(map[string]struct{}, len(workspaces))
	for _, workspace := range workspaces {
		scope[workspace] = struct{}{}
	}
	live := make(map[ScheduleRef]struct{}, len(keep))
	for _, ref := range keep {
		live[ref] = struct{}{}
	}
	return s.q.WithinTx(ctx, func(ctx context.Context, q *queries.DB) error {
		stored, err := q.ListScheduleCursors(ctx)
		if err != nil {
			return wrap("listing schedule cursors", err)
		}
		for _, row := range stored {
			if _, ok := scope[row.Workspace]; !ok {
				continue
			}
			if _, ok := live[ScheduleRef{Workspace: row.Workspace, ScheduleID: row.ScheduleID}]; ok {
				continue
			}
			err := q.DeleteScheduleCursor(ctx, queries.DeleteScheduleCursorParams{Workspace: row.Workspace, ScheduleID: row.ScheduleID})
			if err != nil {
				return wrap(fmt.Sprintf("deleting schedule cursor for %s/%s", row.Workspace, row.ScheduleID), err)
			}
		}
		return nil
	})
}

// InsertRun records one attempt and trims the schedule's history to
// ScheduleRunLimit in the same transaction. run.ID is ignored.
func (s *ScheduleStore) InsertRun(ctx context.Context, run ScheduleRun) (ScheduleRun, error) {
	var inserted queries.ScheduleRun
	err := s.q.WithinTx(ctx, func(ctx context.Context, q *queries.DB) error {
		var sessionID sql.NullInt64
		if run.SessionID != 0 {
			sessionID = sql.NullInt64{Int64: run.SessionID, Valid: true}
		}
		row, err := q.InsertScheduleRun(ctx, queries.InsertScheduleRunParams{
			Workspace: run.Workspace, ScheduleID: run.ScheduleID, ScheduleName: run.ScheduleName,
			ScheduledFor: run.ScheduledFor, StartedAt: run.StartedAt, Reason: run.Reason,
			Missed: int64(run.Missed), Status: run.Status, SessionID: sessionID,
			Prompt: run.Prompt, Error: run.Error,
		})
		if err != nil {
			return wrap(fmt.Sprintf("inserting schedule run for %s/%s", run.Workspace, run.ScheduleID), err)
		}
		inserted = row
		return wrap(fmt.Sprintf("pruning schedule runs for %s/%s", run.Workspace, run.ScheduleID),
			q.PruneScheduleRuns(ctx, queries.PruneScheduleRunsParams{
				Workspace: run.Workspace, ScheduleID: run.ScheduleID, Keep: ScheduleRunLimit,
			}))
	})
	return s.mapRun.Err(inserted, err)
}

// ListRuns is one schedule's history, newest first.
func (s *ScheduleStore) ListRuns(ctx context.Context, workspace, scheduleID string, limit int) ([]ScheduleRun, error) {
	rows, err := s.q.Ctx(ctx).ListScheduleRunsForSchedule(ctx, queries.ListScheduleRunsForScheduleParams{
		Workspace: workspace, ScheduleID: scheduleID, Limit: int64(limit),
	})
	return s.mapRun.SliceErr(rows, wrap(fmt.Sprintf("listing schedule runs for %s/%s", workspace, scheduleID), err))
}

// LastLaunchedRun skips failed and skipped attempts; false when no run ever
// started a chat.
func (s *ScheduleStore) LastLaunchedRun(ctx context.Context, workspace, scheduleID string) (ScheduleRun, bool, error) {
	row, err := s.q.Ctx(ctx).LastLaunchedScheduleRun(ctx, queries.LastLaunchedScheduleRunParams{Workspace: workspace, ScheduleID: scheduleID})
	if errors.Is(err, sql.ErrNoRows) {
		return ScheduleRun{}, false, nil
	}
	if err != nil {
		return ScheduleRun{}, false, wrap(fmt.Sprintf("getting last launched schedule run for %s/%s", workspace, scheduleID), err)
	}
	return s.mapRun(row), true, nil
}

// DeleteByWorkspace drops every cursor and run the workspace's schedules
// hold. A cursor left behind would let a workspace rebuilt under the same
// directory name back-fire every occurrence its predecessor missed.
func (s *ScheduleStore) DeleteByWorkspace(ctx context.Context, workspace string) error {
	return s.q.WithinTx(ctx, func(ctx context.Context, q *queries.DB) error {
		if err := q.DeleteScheduleRunsByWorkspace(ctx, workspace); err != nil {
			return wrap("deleting schedule runs by workspace", err)
		}
		return wrap("deleting schedule cursors by workspace", q.DeleteScheduleCursorsByWorkspace(ctx, workspace))
	})
}
