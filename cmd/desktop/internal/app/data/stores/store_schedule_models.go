package stores

import "github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"

// ScheduleCursor is how far one workspace schedule has been evaluated. Cron is
// snapshotted beside it: a re-timed schedule's cron no longer matches, which
// the scheduler reads as "new" rather than back-filling occurrences under the
// old cadence. EvaluatedThrough is unix milliseconds.
type ScheduleCursor struct {
	Workspace        string
	ScheduleID       string
	EvaluatedThrough int64
	Cron             string
}

// ScheduleRef names one schedule inside its workspace.
type ScheduleRef struct {
	Workspace  string
	ScheduleID string
}

// ScheduleRun is one execution attempt. ScheduledFor and StartedAt are unix
// milliseconds; SessionID is empty when the run launched no chat, which the
// table stores as NULL. ID is assigned on insert.
type ScheduleRun struct {
	ID           int64
	Workspace    string
	ScheduleID   string
	ScheduleName string
	ScheduledFor int64
	StartedAt    int64
	Reason       string
	Missed       int
	Status       string
	SessionID    string
	Prompt       string
	Error        string
}

func mapScheduleCursorFromDB(row queries.ScheduleCursor) ScheduleCursor {
	return ScheduleCursor(row)
}

func mapScheduleRunFromDB(row queries.ScheduleRun) ScheduleRun {
	var sessionID string
	if row.SessionID.Valid {
		sessionID = row.SessionID.String
	}
	return ScheduleRun{
		ID: row.ID, Workspace: row.Workspace, ScheduleID: row.ScheduleID, ScheduleName: row.ScheduleName,
		ScheduledFor: row.ScheduledFor, StartedAt: row.StartedAt, Reason: row.Reason, Missed: int(row.Missed),
		Status: row.Status, SessionID: sessionID, Prompt: row.Prompt, Error: row.Error,
	}
}
