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

var ErrOutputCommandActive = errors.New("output command is already active")

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

func (s *OutputCommandStore) ClaimNextAutomatic(ctx context.Context, afterID int64, claimToken string) (OutputCommand, bool, error) {
	row, err := s.q.Ctx(ctx).ClaimNextAutomaticOutputCommand(ctx, queries.ClaimNextAutomaticOutputCommandParams{
		ClaimToken: claimToken,
		ClaimedAt:  s.now().UnixMilli(),
		AfterID:    afterID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return OutputCommand{}, false, nil
	}
	if err != nil {
		return OutputCommand{}, false, wrap("claiming automatic output command", err)
	}
	return mapOutputCommandFromDB(row), true, nil
}

func (s *OutputCommandStore) Enqueue(ctx context.Context, actionID, key string, payload []byte, createdAt int64, ref models.ItemRef) error {
	return wrap("enqueuing output command", s.q.Ctx(ctx).EnqueueOutputCommand(ctx, queries.EnqueueOutputCommandParams{
		ActionID: actionID, Key: key, Payload: payload, CreatedAt: createdAt,
		ProfileID: ref.ProfileID, SourceKind: ref.SourceKind, SourceScope: ref.SourceScope, ExternalID: ref.ExternalID,
	}))
}

func (s *OutputCommandStore) Confirm(ctx context.Context, actionID, key string, payload []byte, ref models.ItemRef, claimToken string) (OutputCommand, bool, error) {
	now := s.now().UnixMilli()
	q := s.q.Ctx(ctx)
	row, err := q.ConfirmOutputCommand(ctx, queries.ConfirmOutputCommandParams{
		ActionID: actionID, Key: key, Payload: payload, CreatedAt: now, ClaimToken: claimToken, ClaimedAt: now,
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

func (s *OutputCommandStore) Rerun(ctx context.Context, actionID, key string, payload []byte, ref models.ItemRef, claimToken string) (OutputCommand, error) {
	now := s.now().UnixMilli()
	row, err := s.q.Ctx(ctx).RerunOutputCommand(ctx, queries.RerunOutputCommandParams{
		ActionID: actionID, Key: key, Payload: payload, CreatedAt: now, ClaimToken: claimToken, ClaimedAt: now,
		ProfileID: ref.ProfileID, SourceKind: ref.SourceKind, SourceScope: ref.SourceScope, ExternalID: ref.ExternalID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		existing, lookupErr := s.q.Ctx(ctx).GetLatestOutputCommandForAction(ctx, queries.GetLatestOutputCommandForActionParams{ActionID: actionID, Key: key})
		if lookupErr == nil && (existing.Status == "pending" || existing.Status == "running") {
			return OutputCommand{}, fmt.Errorf("rerunning output command %s/%s: %w", actionID, key, ErrOutputCommandActive)
		}
	}
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

func (s *OutputCommandStore) Complete(ctx context.Context, id int64, claimToken, resultJSON string) error {
	rows, err := s.q.Ctx(ctx).CompleteClaimedOutputCommand(ctx, queries.CompleteClaimedOutputCommandParams{
		FinishedAt: s.now().UnixMilli(), ID: id, ClaimToken: claimToken, ResultJson: null(resultJSON),
	})
	return claimedTransition("completing", id, rows, err)
}

func (s *OutputCommandStore) Fail(ctx context.Context, id int64, claimToken, lastErr string) error {
	rows, err := s.q.Ctx(ctx).FailClaimedOutputCommand(ctx, queries.FailClaimedOutputCommandParams{
		FinishedAt: s.now().UnixMilli(), ID: id, ClaimToken: claimToken, LastError: null(lastErr),
	})
	return claimedTransition("failing", id, rows, err)
}

func (s *OutputCommandStore) Requeue(ctx context.Context, id int64, claimToken, lastErr string, delay time.Duration) error {
	rows, err := s.q.Ctx(ctx).RequeueClaimedOutputCommand(ctx, queries.RequeueClaimedOutputCommandParams{
		ID: id, ClaimToken: claimToken, NotBefore: s.now().Add(delay).UnixMilli(), LastError: null(lastErr),
	})
	return claimedTransition("requeueing", id, rows, err)
}

func (s *OutputCommandStore) Cancel(ctx context.Context, id int64, claimToken, reason string) error {
	rows, err := s.q.Ctx(ctx).CancelClaimedOutputCommand(ctx, queries.CancelClaimedOutputCommandParams{
		FinishedAt: s.now().UnixMilli(), ID: id, ClaimToken: claimToken, LastError: null(reason),
	})
	return claimedTransition("cancelling", id, rows, err)
}

func claimedTransition(verb string, id, rows int64, err error) error {
	if err != nil {
		return wrap(fmt.Sprintf("%s output command", verb), err)
	}
	if rows != 1 {
		return fmt.Errorf("%s output command %d: stale claim", verb, id)
	}
	return nil
}

func (s *OutputCommandStore) CountNonterminalForAction(ctx context.Context, actionID string) (int64, error) {
	count, err := s.q.Ctx(ctx).CountNonterminalCommandsForAction(ctx, actionID)
	return count, wrap("counting nonterminal output commands", err)
}
