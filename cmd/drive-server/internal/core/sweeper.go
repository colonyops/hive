package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/pkg/logutils"
)

const sweepBatch = 100

// Sweeper deletes expired share links, links whose document is gone, and
// expired sign-in codes and sessions.
type Sweeper struct {
	links    ShareStore
	codes    CodeStore
	sessions SessionStore
	users    UserStores
	log      zerolog.Logger
	now      func() time.Time
}

func NewSweeper(log zerolog.Logger, links ShareStore, codes CodeStore, sessions SessionStore, users UserStores, opts ...Option) *Sweeper {
	return &Sweeper{links: links, codes: codes, sessions: sessions, users: users, log: logutils.Component(log, "sweeper"), now: newOptions(opts).now}
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
	if err := s.dropDangling(ctx); err != nil {
		errs = append(errs, err)
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

// dropDangling pages by link id, so deleting a row never shifts the page.
func (s *Sweeper) dropDangling(ctx context.Context) error {
	var errs []error
	after := ""
	for {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}
		batch, err := s.links.ListAfter(ctx, after, sweepBatch)
		if err != nil {
			return errors.Join(append(errs, s.warn(err, "list share links"))...)
		}
		for _, l := range batch {
			if err := s.dropIfDangling(ctx, l); err != nil {
				errs = append(errs, s.warn(err, "check share link"))
			}
		}
		if len(batch) < sweepBatch {
			return errors.Join(errs...)
		}
		after = batch[len(batch)-1].ID
	}
}

func (s *Sweeper) dropIfDangling(ctx context.Context, l ShareLink) error {
	store, release, err := s.users.Open(ctx, l.UserID)
	if err != nil {
		return fmt.Errorf("open user store: %w", err)
	}
	defer release()

	exists, err := store.HasID(ctx, l.DocumentID)
	switch {
	case err != nil:
		return fmt.Errorf("check document: %w", err)
	case exists:
		return nil
	default:
		return s.links.Delete(ctx, l.ID)
	}
}

func (s *Sweeper) warn(err error, msg string) error {
	s.log.Warn().Err(err).Msg("sweep: " + msg)
	return fmt.Errorf("sweep: %s: %w", msg, err)
}
