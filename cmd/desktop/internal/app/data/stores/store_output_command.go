package stores

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
)

const (
	maxOutputCommandStreamBytes  = 64 * 1024
	outputCommandTruncatedMarker = "\n... (truncated)"
)

type OutputCommandStore struct {
	q   *queries.DB
	now func() time.Time
}

func NewOutputCommandStore(q *queries.DB, opts Options) *OutputCommandStore {
	return &OutputCommandStore{q: q, now: opts.Now}
}

func (s *OutputCommandStore) ListRunnableAfter(ctx context.Context, afterID int64, limit int) ([]OutputCommand, error) {
	rows, err := s.q.Ctx(ctx).ListRunnableOutputCommandsAfter(ctx, queries.ListRunnableOutputCommandsAfterParams{ID: afterID, Limit: int64(limit)})
	if err != nil {
		return nil, wrap("listing runnable output commands", err)
	}
	return MapFunc[queries.OutputCommand, OutputCommand](mapOutputCommandFromDB).Slice(rows), nil
}

// The unique (actionID, key) pair prevents replayed batches from firing an
// action twice.
func (s *OutputCommandStore) Enqueue(ctx context.Context, actionID, key string, payload []byte, createdAt int64, ref models.ItemRef) error {
	return wrap("enqueuing output command", s.q.Ctx(ctx).EnqueueOutputCommand(ctx, queries.EnqueueOutputCommandParams{
		ActionID: actionID, Key: key, Payload: payload, CreatedAt: createdAt,
		ProfileID: ref.ProfileID, SourceKind: ref.SourceKind, SourceScope: ref.SourceScope, ExternalID: ref.ExternalID,
	}))
}

// Confirm claims a queued command or creates one. A terminal command for the
// same key returns the latest existing command with created=false.
func (s *OutputCommandStore) Confirm(ctx context.Context, actionID, key string, payload []byte, ref models.ItemRef) (OutputCommand, bool, error) {
	q := s.q.Ctx(ctx)
	row, err := q.ConfirmOutputCommand(ctx, queries.ConfirmOutputCommandParams{
		ActionID: actionID, Key: key, Payload: payload, CreatedAt: s.now().UnixMilli(),
		ProfileID: ref.ProfileID, SourceKind: ref.SourceKind, SourceScope: ref.SourceScope, ExternalID: ref.ExternalID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		existing, lookupErr := q.GetLatestOutputCommandForAction(ctx, queries.GetLatestOutputCommandForActionParams{ActionID: actionID, Key: key})
		return mapOutputCommandFromDB(existing), false, wrap("getting existing output command", lookupErr)
	}
	if err != nil {
		return OutputCommand{}, false, wrap("confirming output command", err)
	}
	return mapOutputCommandFromDB(row), true, nil
}

// Rerun creates a new command so prior diagnostics remain intact. If no
// completed run exists, it returns NotFoundError.
func (s *OutputCommandStore) Rerun(ctx context.Context, actionID, key string, payload []byte, ref models.ItemRef) (OutputCommand, error) {
	row, err := s.q.Ctx(ctx).RerunOutputCommand(ctx, queries.RerunOutputCommandParams{
		ActionID: actionID, Key: key, Payload: payload, CreatedAt: s.now().UnixMilli(),
		ProfileID: ref.ProfileID, SourceKind: ref.SourceKind, SourceScope: ref.SourceScope, ExternalID: ref.ExternalID,
	})
	if err != nil {
		return OutputCommand{}, errTransformQueryOne("output_command", fmt.Sprintf("%s/%s", actionID, key), err)
	}
	return mapOutputCommandFromDB(row), nil
}

func (s *OutputCommandStore) Get(ctx context.Context, id int64) (OutputCommand, error) {
	row, err := s.q.Ctx(ctx).GetOutputCommand(ctx, id)
	if err != nil {
		return OutputCommand{}, errTransformQueryOne("output_command", fmt.Sprint(id), err)
	}
	return mapOutputCommandFromDB(row), nil
}

// values are optional result JSON, stdout, and stderr, in that order; stdout
// and stderr are bounded.
func (s *OutputCommandStore) MarkDone(ctx context.Context, id int64, values ...string) error {
	var resultJSON, stdout, stderr string
	if len(values) > 0 {
		resultJSON = values[0]
	}
	if len(values) > 1 {
		stdout = values[1]
	}
	if len(values) > 2 {
		stderr = values[2]
	}
	return wrap("marking output command done", s.q.Ctx(ctx).MarkOutputCommandDone(ctx, queries.MarkOutputCommandDoneParams{
		ID: id, ResultJson: null(resultJSON), Stdout: null(boundOutputCommandStream(stdout)), Stderr: null(boundOutputCommandStream(stderr)),
	}))
}

// values are optional stdout and stderr, in that order; both are bounded.
func (s *OutputCommandStore) MarkFailed(ctx context.Context, id int64, lastErr string, values ...string) error {
	var stdout, stderr string
	if len(values) > 0 {
		stdout = values[0]
	}
	if len(values) > 1 {
		stderr = values[1]
	}
	return wrap("marking output command failed", s.q.Ctx(ctx).MarkOutputCommandFailed(ctx, queries.MarkOutputCommandFailedParams{
		ID: id, LastError: null(lastErr), Stdout: null(boundOutputCommandStream(stdout)), Stderr: null(boundOutputCommandStream(stderr)),
	}))
}

// values are optional stdout and stderr, in that order; both are bounded.
func (s *OutputCommandStore) Retry(ctx context.Context, id int64, lastErr string, values ...string) error {
	var stdout, stderr string
	if len(values) > 0 {
		stdout = values[0]
	}
	if len(values) > 1 {
		stderr = values[1]
	}
	return wrap("recording output command retry", s.q.Ctx(ctx).RetryOutputCommand(ctx, queries.RetryOutputCommandParams{
		ID: id, LastError: null(lastErr), Stdout: null(boundOutputCommandStream(stdout)), Stderr: null(boundOutputCommandStream(stderr)),
	}))
}

func (s *OutputCommandStore) CountNonterminalForAction(ctx context.Context, actionID string) (int64, error) {
	count, err := s.q.Ctx(ctx).CountNonterminalCommandsForAction(ctx, actionID)
	return count, wrap("counting nonterminal output commands", err)
}

func boundOutputCommandStream(stream string) string {
	if len(stream) <= maxOutputCommandStreamBytes {
		return stream
	}
	return stream[:maxOutputCommandStreamBytes-len(outputCommandTruncatedMarker)] + outputCommandTruncatedMarker
}
