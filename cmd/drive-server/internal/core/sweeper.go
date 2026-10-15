package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/pkg/logutils"
)

// Sweeper deletes expired share links, sign-in codes, and sessions.
type Sweeper struct {
	links    ShareStore
	codes    CodeStore
	sessions SessionStore
	log      zerolog.Logger
	now      func() time.Time
}

func NewSweeper(log zerolog.Logger, links ShareStore, codes CodeStore, sessions SessionStore, opts ...Option) *Sweeper {
	return &Sweeper{links: links, codes: codes, sessions: sessions, log: logutils.Component(log, "sweeper"), now: newOptions(opts).now}
}

// Tick runs one sweep. A failed step is logged and does not stop the rest.
func (s *Sweeper) Tick(ctx context.Context) error {
	var errs []error
	if n, err := s.links.DeleteExpired(ctx, s.now()); err != nil {
		errs = append(errs, s.warn(err, "delete expired share links"))
	} else if n > 0 {
		s.log.Debug().Int("links", n).Msg("swept expired share links")
	}
	if n, err := s.codes.DeleteExpired(ctx, s.now()); err != nil {
		errs = append(errs, s.warn(err, "delete expired sign-in codes"))
	} else if n > 0 {
		s.log.Debug().Int("codes", n).Msg("swept expired sign-in codes")
	}
	if n, err := s.sessions.DeleteExpired(ctx, s.now()); err != nil {
		errs = append(errs, s.warn(err, "delete expired sessions"))
	} else if n > 0 {
		s.log.Debug().Int("sessions", n).Msg("swept expired sessions")
	}
	return errors.Join(errs...)
}

func (s *Sweeper) Run(ctx context.Context, every time.Duration) error {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		_ = s.Tick(ctx)
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
	}
}

func (s *Sweeper) warn(err error, msg string) error {
	s.log.Warn().Err(err).Msg("sweep: " + msg)
	return fmt.Errorf("sweep: %s: %w", msg, err)
}
