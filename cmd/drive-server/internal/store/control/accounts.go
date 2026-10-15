package control

import (
	"context"
	"database/sql"
	"time"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/cmd/drive-server/internal/store/control/db"
	"github.com/colonyops/hive/internal/platform/sqlite"
)

type accounts struct{ s *Store }

func (a accounts) Insert(ctx context.Context, acct core.Account) error {
	return wrap("insert account", a.s.q.InsertAccount(ctx, db.InsertAccountParams{
		ID:        acct.ID,
		Email:     acct.Email,
		CreatedAt: toNanos(acct.CreatedAt),
	}))
}

func (a accounts) GetByEmail(ctx context.Context, email string) (core.Account, error) {
	row, err := a.s.q.GetAccountByEmail(ctx, email)
	if err != nil {
		return core.Account{}, wrap("get account by email", err)
	}
	return accountFromRow(row), nil
}

func (a accounts) GetByID(ctx context.Context, id string) (core.Account, error) {
	row, err := a.s.q.GetAccountByID(ctx, id)
	if err != nil {
		return core.Account{}, wrap("get account by id", err)
	}
	return accountFromRow(row), nil
}

func (a accounts) SetLastSignIn(ctx context.Context, id string, at time.Time) error {
	return wrap("set last sign-in", a.s.q.SetAccountLastSignIn(ctx, db.SetAccountLastSignInParams{
		LastSignInAt: sql.NullInt64{Int64: toNanos(at), Valid: true},
		ID:           id,
	}))
}

func (a accounts) List(ctx context.Context) ([]core.Account, error) {
	rows, err := a.s.q.ListAccounts(ctx)
	if err != nil {
		return nil, wrap("list accounts", err)
	}
	out := make([]core.Account, len(rows))
	for i, row := range rows {
		out[i] = accountFromRow(row)
	}
	return out, nil
}

func (a accounts) DeleteByEmail(ctx context.Context, email string) error {
	return sqlite.WithinTx(ctx, a.s.conn, func(ctx context.Context, tx *sql.Tx) error {
		q := a.s.q.WithTx(tx)
		if err := q.DeleteSignInCodesByEmail(ctx, email); err != nil {
			return wrap("delete sign-in codes", err)
		}
		n, err := q.DeleteAccountByEmail(ctx, email)
		if err != nil {
			return wrap("delete account", err)
		}
		if n == 0 {
			return core.ErrNotFound
		}
		return nil
	})
}

func accountFromRow(r db.Account) core.Account {
	return core.Account{
		ID:           r.ID,
		Email:        r.Email,
		CreatedAt:    fromNanos(r.CreatedAt),
		LastSignInAt: fromNullNanos(r.LastSignInAt),
	}
}
