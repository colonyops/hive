package stores

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
)

// ActionRun is one output command as the run viewer lists it: the command's
// own lifecycle plus the job label and the inbox item it ran against.
type ActionRun struct {
	ID           int64
	ActionID     string
	Key          string
	Status       string
	Attempts     int64
	DispatchLane string
	IsRerun      bool
	CreatedAt    int64
	ClaimedAt    int64
	FinishedAt   int64
	LastError    string
	Label        string
	Item         models.ItemRef
	ItemID       int64
	ItemTitle    string
}

type ActionRunLogLine struct {
	ID        int64
	CommandID int64
	Attempt   int64
	Stream    string
	Text      string
	CreatedAt int64
}

// NewActionRunLogLine is a line not yet written. At is when it was produced,
// which can be earlier than the batch it is flushed in.
type NewActionRunLogLine struct {
	Stream string
	Text   string
	At     time.Time
}

type ActionRunStore struct {
	q   *queries.DB
	now func() time.Time
}

func NewActionRunStore(q *queries.DB, opts Options) *ActionRunStore {
	return &ActionRunStore{q: q, now: opts.Now}
}

func (s *ActionRunStore) List(ctx context.Context, beforeID int64, limit int) ([]ActionRun, error) {
	if beforeID <= 0 {
		beforeID = math.MaxInt64
	}
	rows, err := s.q.Ctx(ctx).ListActionRuns(ctx, queries.ListActionRunsParams{BeforeID: beforeID, RowLimit: int64(limit)})
	if err != nil {
		return nil, wrap("listing action runs", err)
	}
	runs := make([]ActionRun, 0, len(rows))
	for _, row := range rows {
		runs = append(runs, mapActionRunRow(queries.GetActionRunRow(row)))
	}
	return runs, nil
}

func (s *ActionRunStore) Get(ctx context.Context, id int64) (ActionRun, error) {
	row, err := s.q.Ctx(ctx).GetActionRun(ctx, id)
	if err != nil {
		return ActionRun{}, errTransformQueryOne("action_run", fmt.Sprint(id), err)
	}
	return mapActionRunRow(row), nil
}

func mapActionRunRow(row queries.GetActionRunRow) ActionRun {
	return ActionRun{
		ID: row.ID, ActionID: row.ActionID, Key: row.Key, Status: row.Status, Attempts: row.Attempts,
		DispatchLane: row.DispatchLane, IsRerun: row.IsRerun != 0,
		CreatedAt: row.CreatedAt, ClaimedAt: row.ClaimedAt, FinishedAt: row.FinishedAt,
		LastError: row.LastError.String, Label: row.Label,
		Item:   models.ItemRef{ProfileID: row.ProfileID, SourceKind: row.SourceKind, SourceScope: row.SourceScope, ExternalID: row.ExternalID},
		ItemID: row.ItemID, ItemTitle: row.ItemTitle,
	}
}

// AppendLog writes one batch of lines in a single transaction, so a reader
// never sees half a flush.
func (s *ActionRunStore) AppendLog(ctx context.Context, commandID, attempt int64, lines []NewActionRunLogLine) error {
	if len(lines) == 0 {
		return nil
	}
	return wrap("appending action run log", s.q.WithinTx(ctx, func(ctx context.Context, tx *queries.DB) error {
		for _, line := range lines {
			at := line.At
			if at.IsZero() {
				at = s.now()
			}
			if err := tx.InsertActionRunLogLine(ctx, queries.InsertActionRunLogLineParams{
				CommandID: commandID, Attempt: attempt, Stream: line.Stream, Text: line.Text, CreatedAt: at.UnixMilli(),
			}); err != nil {
				return err
			}
		}
		return nil
	}))
}

func (s *ActionRunStore) ListLog(ctx context.Context, commandID, afterID int64, limit int) ([]ActionRunLogLine, error) {
	rows, err := s.q.Ctx(ctx).ListActionRunLogAfter(ctx, queries.ListActionRunLogAfterParams{CommandID: commandID, ID: afterID, Limit: int64(limit)})
	if err != nil {
		return nil, wrap("listing action run log", err)
	}
	lines := make([]ActionRunLogLine, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, ActionRunLogLine(row))
	}
	return lines, nil
}

// TailCursor returns the afterID a ListLog call passes to read the last n
// lines. A log shorter than n reads from the start.
func (s *ActionRunStore) TailCursor(ctx context.Context, commandID int64, n int) (int64, error) {
	if n <= 0 {
		return 0, nil
	}
	id, err := s.q.Ctx(ctx).ActionRunLogIDFromEnd(ctx, queries.ActionRunLogIDFromEndParams{CommandID: commandID, Offset: int64(n - 1)})
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, wrap("locating action run log tail", err)
	}
	return id - 1, nil
}
