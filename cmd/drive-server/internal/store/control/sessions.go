package control

import (
	"context"
	"time"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/cmd/drive-server/internal/store/control/db"
)

type sessions struct{ s *Store }

func (x sessions) Insert(ctx context.Context, sess core.Session, valueHash string) error {
	return wrap("insert session", x.s.q.InsertSession(ctx, db.InsertSessionParams{
		ID:        sess.ID,
		UserID:    sess.UserID,
		ValueHash: valueHash,
		ExpiresAt: toNanos(sess.ExpiresAt),
		CreatedAt: toNanos(sess.CreatedAt),
	}))
}

func (x sessions) GetByValueHash(ctx context.Context, valueHash string) (core.Session, error) {
	row, err := x.s.q.GetSessionByValueHash(ctx, valueHash)
	if err != nil {
		return core.Session{}, wrap("get session", err)
	}
	return core.Session{ID: row.ID, UserID: row.UserID, ExpiresAt: fromNanos(row.ExpiresAt), CreatedAt: fromNanos(row.CreatedAt)}, nil
}

func (x sessions) SetExpiry(ctx context.Context, id string, expiresAt time.Time) error {
	return wrap("extend session", x.s.q.SetSessionExpiry(ctx, db.SetSessionExpiryParams{ExpiresAt: toNanos(expiresAt), ID: id}))
}

func (x sessions) DeleteByValueHash(ctx context.Context, valueHash string) error {
	return wrap("delete session", x.s.q.DeleteSessionByValueHash(ctx, valueHash))
}

func (x sessions) DeleteExpired(ctx context.Context, now time.Time) (int, error) {
	n, err := x.s.q.DeleteExpiredSessions(ctx, toNanos(now))
	return int(n), wrap("delete expired sessions", err)
}
