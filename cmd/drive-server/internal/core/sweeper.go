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
	steps := []struct {
		what   string
		field  string
		delete func(context.Context, time.Time) (int, error)
	}{
		{"share links", "links", s.links.DeleteExpired},
		{"sign-in codes", "codes", s.codes.DeleteExpired},
		{"sessions", "sessions", s.sessions.DeleteExpired},
	}
	var errs []error
	for _, step := range steps {
		n, err := step.delete(ctx, s.now())
		if err != nil {
			s.log.Warn().Err(err).Msg("sweep: delete expired " + step.what)
			errs = append(errs, fmt.Errorf("sweep: delete expired %s: %w", step.what, err))
		} else if n > 0 {
			s.log.Debug().Int(step.field, n).Msg("swept expired " + step.what)
		}
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
