package control

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"time"

	"github.com/colonyops/hive/cmd/drive-server/internal/store/control/db"
	"github.com/colonyops/hive/internal/platform/sqlite"
	"github.com/colonyops/hive/pkg/randid"
)

type codes struct{ s *Store }

func (c codes) Put(ctx context.Context, email, codeHash string, expiresAt time.Time) error {
	return sqlite.WithinTx(ctx, c.s.conn, func(ctx context.Context, tx *sql.Tx) error {
		q := c.s.q.WithTx(tx)
		if err := q.DeleteSignInCodesForEmail(ctx, email); err != nil {
			return wrap("replace sign-in code", err)
		}
		return wrap("insert sign-in code", q.InsertSignInCode(ctx, db.InsertSignInCodeParams{
			ID:        randid.UUIDv7(),
			Email:     email,
			CodeHash:  codeHash,
			ExpiresAt: toNanos(expiresAt),
		}))
	})
}

func (c codes) Consume(ctx context.Context, email, codeHash string, now time.Time, maxAttempts int) (ok bool, left int, err error) {
	err = sqlite.WithinTx(ctx, c.s.conn, func(ctx context.Context, tx *sql.Tx) error {
		q := c.s.q.WithTx(tx)
		row, err := q.GetSignInCodeByEmail(ctx, email)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return wrap("get sign-in code", err)
		}
		if !now.Before(fromNanos(row.ExpiresAt)) || int(row.Attempts) >= maxAttempts {
			return wrap("delete sign-in code", q.DeleteSignInCode(ctx, row.ID))
		}
		if subtle.ConstantTimeCompare([]byte(row.CodeHash), []byte(codeHash)) == 1 {
			ok = true
			return wrap("delete sign-in code", q.DeleteSignInCode(ctx, row.ID))
		}
		left = maxAttempts - int(row.Attempts) - 1
		if left == 0 {
			return wrap("delete sign-in code", q.DeleteSignInCode(ctx, row.ID))
		}
		return wrap("count attempt", q.BumpSignInCodeAttempts(ctx, row.ID))
	})
	if err != nil {
		return false, 0, err
	}
	return ok, left, nil
}

func (c codes) DeleteExpired(ctx context.Context, now time.Time) (int, error) {
	n, err := c.s.q.DeleteExpiredSignInCodes(ctx, toNanos(now))
	return int(n), wrap("delete expired sign-in codes", err)
}
