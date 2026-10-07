package ingest

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
)

type retentionStore struct {
	mu     sync.Mutex
	calls  int
	pruned chan struct{}
}

func (s *retentionStore) Prune(_ context.Context, _ queries.RetentionPolicy) error {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	if s.pruned != nil {
		select {
		case s.pruned <- struct{}{}:
		default:
		}
	}
	return nil
}

func TestMaintenanceTick_Prunes(t *testing.T) {
	pruner := &retentionStore{}
	maintenance := NewMaintenance(
		pruner,
		queries.DefaultRetentionPolicy(),
		time.Hour,
		zerolog.Nop(),
	)

	maintenance.Tick(t.Context())

	pruner.mu.Lock()
	defer pruner.mu.Unlock()
	require.Equal(t, 1, pruner.calls)
}

func TestMaintenanceStop_WaitsForScheduledLoop(t *testing.T) {
	pruner := &retentionStore{pruned: make(chan struct{}, 1)}
	maintenance := NewMaintenance(
		pruner,
		queries.DefaultRetentionPolicy(),
		time.Millisecond,
		zerolog.Nop(),
	)
	maintenance.Start(t.Context())

	select {
	case <-pruner.pruned:
	case <-time.After(time.Second):
		t.Fatal("maintenance did not run on its scheduled interval")
	}
	maintenance.Stop()

	pruner.mu.Lock()
	calls := pruner.calls
	pruner.mu.Unlock()
	time.Sleep(5 * time.Millisecond)
	pruner.mu.Lock()
	defer pruner.mu.Unlock()
	assert.Equal(t, calls, pruner.calls, "Stop must prevent further database maintenance")
}
