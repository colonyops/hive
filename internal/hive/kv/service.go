// Package kv is the persistent key-value service and its expiration sweep.
package kv

import (
	"context"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/domain/kv"
	"github.com/colonyops/hive/pkg/logutils"
)

// Store is a kv.KV that can delete its expired entries in bulk.
type Store interface {
	kv.KV
	SweepExpired(ctx context.Context) error
}

// Service is the persistent KV store. Reads already treat an expired entry as
// missing; Sweep only reclaims the rows.
type Service struct {
	Store
	logger zerolog.Logger
}

var _ kv.KV = (*Service)(nil)

func NewService(logger zerolog.Logger, store Store) *Service {
	return &Service{Store: store, logger: logutils.Component(logger, "kv")}
}

// Sweep deletes expired entries every interval until ctx is done.
func (s *Service) Sweep(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.SweepExpired(ctx); err != nil {
				s.logger.Debug().Err(err).Msg("kv sweep failed")
			}
		}
	}
}
