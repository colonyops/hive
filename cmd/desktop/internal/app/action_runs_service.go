package app

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
)

const (
	actionRunDefaultListLimit = 100
	actionRunMaxListLimit     = 500
	actionRunDefaultLogLimit  = 2000
	actionRunMaxLogLimit      = 10000
)

// ActionRunsService reads what output commands did: the run history and the
// log each attempt wrote. Terminal-target actions have no durable command and
// so no run here.
type ActionRunsService struct {
	runs    *stores.ActionRunStore
	actions dispatch.ActionLister
}

func newActionRunsService(runs *stores.ActionRunStore, actions dispatch.ActionLister) *ActionRunsService {
	return &ActionRunsService{runs: runs, actions: actions}
}

type ActionRunSummary struct {
	ID         int64  `json:"id"`
	ActionID   string `json:"actionId"`
	Label      string `json:"label"`
	Type       string `json:"type,omitempty"`
	Status     string `json:"status"`
	Attempts   int64  `json:"attempts"`
	Lane       string `json:"lane"`
	Rerun      bool   `json:"rerun"`
	Key        string `json:"key"`
	CreatedAt  int64  `json:"createdAt"`
	StartedAt  int64  `json:"startedAt,omitempty"`
	FinishedAt int64  `json:"finishedAt,omitempty"`
	Error      string `json:"error,omitempty"`
	ProfileID  string `json:"profileId,omitempty"`
	ItemID     int64  `json:"itemId,omitempty"`
	ItemTitle  string `json:"itemTitle,omitempty"`
}

type ActionRunLogLine struct {
	ID      int64  `json:"id"`
	Attempt int64  `json:"attempt"`
	Stream  string `json:"stream"`
	Text    string `json:"text"`
	At      int64  `json:"at"`
}

// ActionRunLogPage is one read of a run's log. NextAfterID is what the next
// read passes to continue; More reports lines past this page.
type ActionRunLogPage struct {
	Lines       []ActionRunLogLine `json:"lines"`
	NextAfterID int64              `json:"nextAfterId"`
	More        bool               `json:"more"`
}

// List returns runs newest first, before the given run id (zero for the
// newest). Limits outside 1..500 default to 100.
func (s *ActionRunsService) List(ctx context.Context, beforeID int64, limit int) ([]ActionRunSummary, error) {
	if limit <= 0 || limit > actionRunMaxListLimit {
		limit = actionRunDefaultListLimit
	}
	rows, err := s.runs.List(ctx, beforeID, limit)
	if err != nil {
		return nil, Wrap(err, KindInternal, "listing action runs")
	}
	out := make([]ActionRunSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, s.summary(row))
	}
	return out, nil
}

func (s *ActionRunsService) Get(ctx context.Context, commandID int64) (ActionRunSummary, error) {
	row, err := s.runs.Get(ctx, commandID)
	if stores.IsNotFound(err) {
		return ActionRunSummary{}, Wrap(err, KindNotFound, "action run %d not found", commandID)
	}
	if err != nil {
		return ActionRunSummary{}, Wrap(err, KindInternal, "reading action run %d", commandID)
	}
	return s.summary(row), nil
}

// Log reads lines after afterID, oldest first.
func (s *ActionRunsService) Log(ctx context.Context, commandID, afterID int64, limit int) (ActionRunLogPage, error) {
	if limit <= 0 || limit > actionRunMaxLogLimit {
		limit = actionRunDefaultLogLimit
	}
	if _, err := s.runs.Get(ctx, commandID); err != nil {
		if stores.IsNotFound(err) {
			return ActionRunLogPage{}, Wrap(err, KindNotFound, "action run %d not found", commandID)
		}
		return ActionRunLogPage{}, Wrap(err, KindInternal, "reading action run %d", commandID)
	}
	return s.readLog(ctx, commandID, afterID, limit)
}

// Tail reads the last lines of a run's log, for a reader that wants where it
// ended rather than where it began.
func (s *ActionRunsService) Tail(ctx context.Context, commandID int64, lines int) (ActionRunLogPage, error) {
	if lines <= 0 || lines > actionRunMaxLogLimit {
		lines = actionRunDefaultLogLimit
	}
	if _, err := s.runs.Get(ctx, commandID); err != nil {
		if stores.IsNotFound(err) {
			return ActionRunLogPage{}, Wrap(err, KindNotFound, "action run %d not found", commandID)
		}
		return ActionRunLogPage{}, Wrap(err, KindInternal, "reading action run %d", commandID)
	}
	afterID, err := s.runs.TailCursor(ctx, commandID, lines)
	if err != nil {
		return ActionRunLogPage{}, Wrap(err, KindInternal, "reading action run %d log", commandID)
	}
	return s.readLog(ctx, commandID, afterID, lines)
}

func (s *ActionRunsService) readLog(ctx context.Context, commandID, afterID int64, limit int) (ActionRunLogPage, error) {
	rows, err := s.runs.ListLog(ctx, commandID, afterID, limit+1)
	if err != nil {
		return ActionRunLogPage{}, Wrap(err, KindInternal, "reading action run %d log", commandID)
	}
	page := ActionRunLogPage{Lines: make([]ActionRunLogLine, 0, min(len(rows), limit)), NextAfterID: afterID}
	if len(rows) > limit {
		page.More = true
		rows = rows[:limit]
	}
	for _, row := range rows {
		page.Lines = append(page.Lines, ActionRunLogLine{ID: row.ID, Attempt: row.Attempt, Stream: row.Stream, Text: row.Text, At: row.CreatedAt})
		page.NextAfterID = row.ID
	}
	return page, nil
}

func (s *ActionRunsService) summary(row stores.ActionRun) ActionRunSummary {
	summary := ActionRunSummary{
		ID: row.ID, ActionID: row.ActionID, Label: row.Label, Status: row.Status, Attempts: row.Attempts,
		Lane: row.DispatchLane, Rerun: row.IsRerun, Key: row.Key,
		CreatedAt: row.CreatedAt, StartedAt: row.ClaimedAt, FinishedAt: row.FinishedAt, Error: row.LastError,
		ItemID: row.ItemID, ItemTitle: row.ItemTitle,
	}
	if row.Item.Known() {
		summary.ProfileID = row.Item.ProfileID
	}
	if s.actions != nil {
		if action, ok := s.actions.Get(row.ActionID); ok {
			summary.Type = action.Type
			if summary.Label == "" {
				summary.Label = action.Label
			}
		}
	}
	if summary.Label == "" {
		summary.Label = row.ActionID
	}
	return summary
}
