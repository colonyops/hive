package relay

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog"
)

// Sweeper enforces retention and the per-source row cap on a timer.
type Sweeper struct {
	store  Store
	limits Limits
	now    func() time.Time
	log    zerolog.Logger
}

func NewSweeper(store Store, limits Limits, log zerolog.Logger) *Sweeper {
	return &Sweeper{store: store, limits: limits, now: time.Now, log: log}
}

// Tick drops expired sign-in codes, then walks every account's sources once:
// DeleteBefore(now - Retention), then TrimTo(DeliveriesPerSource). The drivers credit the freed bytes themselves.
// Work runs per source and a failure skips only that source, so one account's
// volume or fault never holds back another's retention.
func (s *Sweeper) Tick(ctx context.Context) error {
	now := s.now()
	var errs []error
	if n, err := s.store.AuthCodes().DeleteExpired(ctx, now); err != nil {
		s.log.Warn().Err(err).Msg("sweep: delete expired sign-in codes")
		errs = append(errs, fmt.Errorf("sweep: delete expired sign-in codes: %w", err))
	} else if n > 0 {
		s.log.Debug().Int("codes", n).Msg("swept expired sign-in codes")
	}

	accts, err := s.store.Accounts().List(ctx)
	if err != nil {
		return errors.Join(append(errs, fmt.Errorf("sweep: list accounts: %w", err))...)
	}
	cutoff := now.Add(-s.limits.Retention)
	for _, a := range accts {
		srcs, err := s.store.Sources().ListByEmail(ctx, a.Email)
		if err != nil {
			errs = append(errs, s.failed(err, "list sources", a.Email, ""))
			continue
		}
		for _, src := range srcs {
			if err := ctx.Err(); err != nil {
				return errors.Join(append(errs, err)...)
			}
			if err := s.sweepSource(ctx, src, cutoff); err != nil {
				errs = append(errs, s.failed(err, "sweep source", a.Email, src.ID))
			}
		}
	}
	return errors.Join(errs...)
}

func (s *Sweeper) sweepSource(ctx context.Context, src Source, cutoff time.Time) error {
	expired, expiredBytes, err := s.store.Deliveries().DeleteBefore(ctx, src.Email, src.ID, cutoff)
	if err != nil {
		return fmt.Errorf("delete expired: %w", err)
	}
	trimmed, trimmedBytes, err := s.store.Deliveries().TrimTo(ctx, src.Email, src.ID, s.limits.DeliveriesPerSource)
	if err != nil {
		return fmt.Errorf("trim: %w", err)
	}
	if expired+trimmed > 0 {
		s.log.Debug().
			Str("source", src.ID).
			Int("expired", expired).
			Int("trimmed", trimmed).
			Int64("freed_bytes", expiredBytes+trimmedBytes).
			Msg("swept source")
	}
	return nil
}

func (s *Sweeper) failed(err error, what, email, sourceID string) error {
	s.log.Warn().Err(err).Str("email", email).Str("source", sourceID).Msg("sweep: " + what)
	if sourceID != "" {
		return fmt.Errorf("sweep %s: %w", sourceID, err)
	}
	return fmt.Errorf("sweep %s: %w", email, err)
}

// Run ticks every `every` until ctx is done; a failed tick is logged and the
// loop continues.
func (s *Sweeper) Run(ctx context.Context, every time.Duration) error {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		if err := s.Tick(ctx); err != nil && ctx.Err() == nil {
			s.log.Warn().Err(err).Msg("sweep tick failed")
		}
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
	}
}
