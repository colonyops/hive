package stores

import (
	"context"
	"database/sql"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
)

type OrchestratorTokenStore struct {
	q   *queries.DB
	now func() int64
}

func NewOrchestratorTokenStore(q *queries.DB, opts Options) *OrchestratorTokenStore {
	return &OrchestratorTokenStore{q: q, now: func() int64 { return opts.Now().UnixMilli() }}
}

func (s *OrchestratorTokenStore) Create(ctx context.Context, name, hash, hint string) (OrchestratorToken, error) {
	row, err := s.q.Ctx(ctx).CreateOrchestratorToken(ctx, queries.CreateOrchestratorTokenParams{
		Name: name, TokenHash: hash, Hint: hint, CreatedAt: s.now(),
	})
	if err != nil {
		return OrchestratorToken{}, wrap("creating orchestrator token", err)
	}
	return mapOrchestratorTokenFromDB(row), nil
}

func (s *OrchestratorTokenStore) List(ctx context.Context) ([]OrchestratorToken, error) {
	rows, err := s.q.Ctx(ctx).ListOrchestratorTokens(ctx)
	if err := errTransformQueryMany(err); err != nil {
		return nil, wrap("listing orchestrator tokens", err)
	}
	out := make([]OrchestratorToken, len(rows))
	for i, row := range rows {
		out[i] = mapOrchestratorTokenFromDB(row)
	}
	return out, nil
}

func (s *OrchestratorTokenStore) GetByHash(ctx context.Context, hash string) (OrchestratorToken, error) {
	row, err := s.q.Ctx(ctx).GetOrchestratorTokenByHash(ctx, hash)
	if err != nil {
		return OrchestratorToken{}, errTransformQueryOne("orchestrator_token", "hash", err)
	}
	return mapOrchestratorTokenFromDB(row), nil
}

func (s *OrchestratorTokenStore) Touch(ctx context.Context, id int64) error {
	return wrap("touching orchestrator token", s.q.Ctx(ctx).TouchOrchestratorToken(ctx, queries.TouchOrchestratorTokenParams{
		LastUsedAt: sql.NullInt64{Int64: s.now(), Valid: true}, ID: id,
	}))
}

func (s *OrchestratorTokenStore) Delete(ctx context.Context, id int64) (bool, error) {
	n, err := s.q.Ctx(ctx).DeleteOrchestratorToken(ctx, id)
	if err != nil {
		return false, wrap("deleting orchestrator token", err)
	}
	return n > 0, nil
}
