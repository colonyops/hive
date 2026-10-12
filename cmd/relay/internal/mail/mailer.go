package mail

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/relay/internal/relay"
)

const drainBatch = 50

// backoff is the wait after each failed attempt; past the last step a mail is
// retried hourly.
var backoff = []time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute}

const backoffCeiling = time.Hour

// Mailer drains the outbox through a Sender.
type Mailer struct {
	outbox relay.OutboxStore
	sender Sender
	log    zerolog.Logger
	now    func() time.Time
}

func NewMailer(outbox relay.OutboxStore, sender Sender, log zerolog.Logger) *Mailer {
	return &Mailer{outbox: outbox, sender: sender, log: log, now: time.Now}
}

// Run drains every `every` until ctx is done; a failed pass is logged and the
// loop continues.
func (m *Mailer) Run(ctx context.Context, every time.Duration) error {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		if err := m.Drain(ctx); err != nil && ctx.Err() == nil {
			m.log.Warn().Err(err).Msg("mail drain failed")
		}
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
	}
}

// Drain sends every due mail once. A send failure defers that mail and does
// not stop the pass.
func (m *Mailer) Drain(ctx context.Context) error {
	due, err := m.outbox.Due(ctx, m.now(), drainBatch)
	if err != nil {
		return fmt.Errorf("list due mail: %w", err)
	}
	var errs []error
	for _, mail := range due {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := m.sender.Send(ctx, mail); err != nil {
			next := m.now().Add(Backoff(mail.Attempts))
			m.log.Warn().Err(err).Str("id", mail.ID).Int("attempt", mail.Attempts+1).Time("next", next).Msg("mail send failed")
			if err := m.outbox.MarkFailed(ctx, mail.ID, next); err != nil {
				errs = append(errs, fmt.Errorf("mark %s failed: %w", mail.ID, err))
			}
			continue
		}
		if err := m.outbox.MarkSent(ctx, mail.ID, m.now()); err != nil {
			errs = append(errs, fmt.Errorf("mark %s sent: %w", mail.ID, err))
		}
	}
	return errors.Join(errs...)
}

// Backoff is the wait before retrying a mail that has already failed
// attempts times.
func Backoff(attempts int) time.Duration {
	if attempts < len(backoff) {
		return backoff[max(attempts, 0)]
	}
	return backoffCeiling
}
