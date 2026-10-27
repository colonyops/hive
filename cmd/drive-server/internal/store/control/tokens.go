package control

import (
	"context"
	"database/sql"
	"time"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/cmd/drive-server/internal/store/control/db"
)

type tokens struct{ s *Store }

func (t tokens) Insert(ctx context.Context, tok core.Token, secretHash string) error {
	return wrap("insert token", t.s.q.InsertAPIToken(ctx, db.InsertAPITokenParams{
		ID:         tok.ID,
		UserID:     tok.UserID,
		Name:       tok.Name,
		SecretHash: secretHash,
		Scope:      tok.Scope,
		ExpiresAt:  toNullNanos(tok.ExpiresAt),
		CreatedAt:  toNanos(tok.CreatedAt),
	}))
}

func (t tokens) GetBySecretHash(ctx context.Context, secretHash string) (core.Token, error) {
	row, err := t.s.q.GetAPITokenBySecretHash(ctx, secretHash)
	if err != nil {
		return core.Token{}, wrap("get token", err)
	}
	return tokenFromRow(row), nil
}

func (t tokens) ListByUser(ctx context.Context, userID string) ([]core.Token, error) {
	rows, err := t.s.q.ListAPITokensByUser(ctx, userID)
	if err != nil {
		return nil, wrap("list tokens", err)
	}
	out := make([]core.Token, len(rows))
	for i, row := range rows {
		out[i] = tokenFromRow(row)
	}
	return out, nil
}

func (t tokens) Delete(ctx context.Context, userID, id string) error {
	n, err := t.s.q.DeleteAPIToken(ctx, db.DeleteAPITokenParams{ID: id, UserID: userID})
	if err != nil {
		return wrap("delete token", err)
	}
	if n == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (t tokens) Touch(ctx context.Context, id string, at time.Time) error {
	return wrap("touch token", t.s.q.TouchAPIToken(ctx, db.TouchAPITokenParams{
		LastUsedAt: sql.NullInt64{Int64: toNanos(at), Valid: true},
		ID:         id,
	}))
}

func tokenFromRow(r db.ApiToken) core.Token {
	return core.Token{
		ID:         r.ID,
		UserID:     r.UserID,
		Name:       r.Name,
		Scope:      r.Scope,
		ExpiresAt:  fromNullNanos(r.ExpiresAt),
		LastUsedAt: fromNullNanos(r.LastUsedAt),
		CreatedAt:  fromNanos(r.CreatedAt),
	}
}
