package control

import (
	"context"
	"time"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/cmd/drive-server/internal/store/control/db"
)

type outbox struct{ s *Store }

func (o outbox) Append(ctx context.Context, m core.Mail) error {
	return wrap("queue mail", o.s.q.InsertMail(ctx, db.InsertMailParams{
		ID:            m.ID,
		ToAddr:        m.To,
		Subject:       m.Subject,
		Body:          m.Body,
		NextAttemptAt: toNanos(m.NextAt),
		CreatedAt:     toNanos(m.CreatedAt),
	}))
}

func (o outbox) Due(ctx context.Context, now time.Time, limit int) ([]core.Mail, error) {
	rows, err := o.s.q.ListDueMail(ctx, db.ListDueMailParams{NextAttemptAt: toNanos(now), Limit: int64(limit)})
	if err != nil {
		return nil, wrap("list due mail", err)
	}
	out := make([]core.Mail, len(rows))
	for i, r := range rows {
		out[i] = core.Mail{
			ID:        r.ID,
			To:        r.ToAddr,
			Subject:   r.Subject,
			Body:      r.Body,
			Attempts:  int(r.Attempts),
			NextAt:    fromNanos(r.NextAttemptAt),
			CreatedAt: fromNanos(r.CreatedAt),
		}
	}
	return out, nil
}

func (o outbox) MarkSent(ctx context.Context, id string) error {
	return wrap("delete sent mail", o.s.q.DeleteMail(ctx, id))
}

func (o outbox) MarkFailed(ctx context.Context, id string, nextAt time.Time) error {
	return wrap("defer mail", o.s.q.DeferMail(ctx, db.DeferMailParams{NextAttemptAt: toNanos(nextAt), ID: id}))
}
