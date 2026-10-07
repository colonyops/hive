package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/activity"
	"github.com/colonyops/hive/cmd/desktop/internal/app/agentws"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
	"github.com/colonyops/hive/cmd/desktop/internal/app/schedule"
)

// scheduleWorkspaces is both the scheduler's Source and its WorkspaceNamer: a
// second adapter over the same store would only be a second place to keep the
// "valid workspaces only" rule in step.
type scheduleWorkspaces struct{ store *agentws.Store }

// A workspace whose manifest failed to parse contributes neither specs nor a
// directory: its last-good schedules may name a cadence the file no longer
// says, and listing the directory would let the pass read "every schedule
// here was deleted" from a file the app could not open.
func (w scheduleWorkspaces) Snapshot() schedule.Snapshot {
	statuses := w.store.Statuses()
	snapshot := schedule.Snapshot{
		Specs:      make([]schedule.Spec, 0, len(statuses)),
		Workspaces: make([]string, 0, len(statuses)),
	}
	for _, st := range statuses {
		if !st.Valid {
			continue
		}
		snapshot.Workspaces = append(snapshot.Workspaces, st.Dir)
		snapshot.Specs = append(snapshot.Specs, st.Workspace.Schedules...)
	}
	return snapshot
}

func (w scheduleWorkspaces) WorkspaceName(dir string) string {
	st, _ := w.store.Status(dir)
	return st.Workspace.Name
}

// scheduleStore is the only place time.Time meets the tables' unix
// milliseconds.
type scheduleStore struct{ schedules *stores.ScheduleStore }

func (s scheduleStore) Cursor(ctx context.Context, workspace, id string) (schedule.Cursor, bool, error) {
	rec, ok, err := s.schedules.Cursor(ctx, workspace, id)
	if err != nil || !ok {
		return schedule.Cursor{}, false, err
	}
	return schedule.Cursor{
		Workspace:        rec.Workspace,
		ID:               rec.ScheduleID,
		EvaluatedThrough: time.UnixMilli(rec.EvaluatedThrough),
		Cron:             rec.Cron,
	}, true, nil
}

func (s scheduleStore) SaveCursor(ctx context.Context, cursor schedule.Cursor) error {
	return s.schedules.SaveCursor(ctx, stores.ScheduleCursor{
		Workspace:        cursor.Workspace,
		ScheduleID:       cursor.ID,
		EvaluatedThrough: cursor.EvaluatedThrough.UnixMilli(),
		Cron:             cursor.Cron,
	})
}

func (s scheduleStore) PruneCursors(ctx context.Context, workspaces []string, keep []schedule.Cursor) error {
	refs := make([]stores.ScheduleRef, 0, len(keep))
	for _, cursor := range keep {
		refs = append(refs, stores.ScheduleRef{Workspace: cursor.Workspace, ScheduleID: cursor.ID})
	}
	return s.schedules.PruneCursors(ctx, workspaces, refs)
}

func (s scheduleStore) LastLaunchedRun(ctx context.Context, workspace, id string) (schedule.Run, bool, error) {
	rec, ok, err := s.schedules.LastLaunchedRun(ctx, workspace, id)
	if err != nil || !ok {
		return schedule.Run{}, false, err
	}
	return scheduleRunFromRecord(rec), true, nil
}

func (s scheduleStore) InsertRun(ctx context.Context, run schedule.Run) (schedule.Run, error) {
	rec, err := s.schedules.InsertRun(ctx, stores.ScheduleRun{
		Workspace:    run.Workspace,
		ScheduleID:   run.ScheduleID,
		ScheduleName: run.ScheduleName,
		ScheduledFor: run.ScheduledFor.UnixMilli(),
		StartedAt:    run.StartedAt.UnixMilli(),
		Reason:       string(run.Reason),
		Missed:       run.Missed,
		Status:       string(run.Status),
		SessionID:    run.SessionID,
		Prompt:       run.Prompt,
		Error:        run.Error,
	})
	if err != nil {
		return schedule.Run{}, err
	}
	return scheduleRunFromRecord(rec), nil
}

func scheduleRunFromRecord(rec stores.ScheduleRun) schedule.Run {
	return schedule.Run{
		ID:           rec.ID,
		Workspace:    rec.Workspace,
		ScheduleID:   rec.ScheduleID,
		ScheduleName: rec.ScheduleName,
		ScheduledFor: time.UnixMilli(rec.ScheduledFor),
		StartedAt:    time.UnixMilli(rec.StartedAt),
		Reason:       schedule.Reason(rec.Reason),
		Missed:       rec.Missed,
		Status:       schedule.Status(rec.Status),
		SessionID:    rec.SessionID,
		Prompt:       rec.Prompt,
		Error:        rec.Error,
	}
}

// scheduleLauncher goes through the same service a hand-started chat does, so
// a scheduled run gets the regenerated artifacts and the same session cap.
type scheduleLauncher struct{ workspaces *AgentWorkspacesService }

func (l scheduleLauncher) Launch(ctx context.Context, req schedule.LaunchRequest) (string, error) {
	view, err := l.workspaces.StartScheduledSession(ctx, StartScheduledSession{
		Workspace: req.Workspace, ScheduleID: req.ScheduleID, ScheduleName: req.ScheduleName,
		Name: req.Name, Prompt: req.Prompt,
	})
	if err != nil {
		return "", err
	}
	// An interactive launch shows an early exit as a notice on a pane the user
	// is already looking at. A scheduled one has no pane, so the same fact has
	// to reach the run history as a failure -- a CLI that is not on PATH, or a
	// flag it rejected, is not a chat that ran.
	if view.ExitedEarly {
		return "", Errorf(KindInternal, "%s", view.Notice)
	}
	return view.ID, nil
}

func (l scheduleLauncher) SessionLive(ctx context.Context, sessionID string) (bool, error) {
	return l.workspaces.SessionLive(ctx, sessionID)
}

// buildScheduler runs in every mode, mock ones included: a fixture root
// declares no schedules, so the loop passes over an empty set rather than
// becoming a second startup path to keep in step.
func (a *App) buildScheduler(logger zerolog.Logger) *schedule.Scheduler {
	workspaces := scheduleWorkspaces{store: a.agentWorkspaceStore}
	return schedule.New(schedule.Options{
		Source:   workspaces,
		Names:    workspaces,
		Store:    scheduleStore{schedules: a.Stores.Schedules},
		Launcher: scheduleLauncher{workspaces: a.AgentWorkspaces},
		Logger:   logger,
		OnRun: func(run schedule.Run) {
			a.Events.Publish(a.ctx, events.SchedulesUpdated{Workspace: run.Workspace})
			a.Activity.Record(a.ctx, activity.ScheduleRun(
				run.ScheduleName, run.Workspace, string(run.Status), scheduleRunDetail(run)))
		},
	})
}

func scheduleRunDetail(run schedule.Run) string {
	parts := []string{string(run.Reason)}
	if run.Missed > 0 {
		parts = append(parts, fmt.Sprintf("%d missed", run.Missed))
	}
	if run.SessionID != "" {
		parts = append(parts, fmt.Sprintf("chat %s", run.SessionID))
	}
	if run.Error != "" {
		parts = append(parts, run.Error)
	}
	return strings.Join(parts, " · ")
}
