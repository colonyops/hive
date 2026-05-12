package stores

import (
	"context"

	"github.com/colonyops/hive/internal/core/timer"
	"github.com/colonyops/hive/internal/data/db"
)

// DBTimerAdapter wraps *db.Queries to satisfy timer.InactiveQuerier. The
// adapter exists because the timer package's Row type is intentionally a
// minimal subset (ID + Pid) decoupled from the sqlc-generated db.Timer
// (db -> timer for the Status enum would otherwise be cyclic).
type DBTimerAdapter struct{ Q *db.Queries }

func (a DBTimerAdapter) ActiveTimersForSession(ctx context.Context, sessionID string) ([]timer.Row, error) {
	rows, err := a.Q.ActiveTimersForSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return timerRows(rows), nil
}

func (a DBTimerAdapter) ActiveTimersAll(ctx context.Context) ([]timer.Row, error) {
	rows, err := a.Q.ActiveTimersAll(ctx)
	if err != nil {
		return nil, err
	}
	return timerRows(rows), nil
}

func (a DBTimerAdapter) MarkInactiveTimersForSession(ctx context.Context, arg timer.MarkInactiveParams) error {
	return a.Q.MarkInactiveTimersForSession(ctx, db.MarkInactiveTimersForSessionParams{
		SessionID: arg.SessionID,
		Ids:       arg.IDs,
	})
}

func (a DBTimerAdapter) MarkInactiveTimersAll(ctx context.Context, ids []string) error {
	return a.Q.MarkInactiveTimersAll(ctx, ids)
}

func timerRows(in []db.Timer) []timer.Row {
	out := make([]timer.Row, len(in))
	for i, r := range in {
		var pid *int64
		if r.Pid.Valid {
			v := r.Pid.Int64
			pid = &v
		}
		out[i] = timer.Row{ID: r.ID, Pid: pid}
	}
	return out
}
