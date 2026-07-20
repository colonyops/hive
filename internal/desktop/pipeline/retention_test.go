package pipeline

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/desktop/pipeline/flow"
	"github.com/colonyops/hive/internal/desktop/pipeline/pipelinedb"
	"github.com/rs/zerolog"
)

type retentionFlowLister struct {
	flows []flow.Flow
}

func (l retentionFlowLister) List() []flow.Flow { return l.flows }

type retentionStore struct {
	mu        sync.Mutex
	consumers [][]string
	calls     chan struct{}
}

func (s *retentionStore) Prune(_ context.Context, consumers []string, _ pipelinedb.RetentionPolicy) (pipelinedb.RetentionResult, error) {
	s.mu.Lock()
	s.consumers = append(s.consumers, append([]string(nil), consumers...))
	s.mu.Unlock()
	if s.calls != nil {
		select {
		case s.calls <- struct{}{}:
		default:
		}
	}
	return pipelinedb.RetentionResult{}, nil
}

func TestMaintenanceTick_UsesOnlyEnabledFlowIDs(t *testing.T) {
	store := &retentionStore{}
	maintenance := NewMaintenance(
		store,
		retentionFlowLister{flows: []flow.Flow{{ID: "enabled", Enabled: true}, {ID: "disabled", Enabled: false}}},
		pipelinedb.DefaultRetentionPolicy(),
		time.Hour,
		zerolog.Nop(),
	)

	maintenance.Tick(t.Context())

	store.mu.Lock()
	defer store.mu.Unlock()
	require.Len(t, store.consumers, 1)
	assert.Equal(t, []string{"enabled"}, store.consumers[0])
}

func TestMaintenanceStop_WaitsForScheduledLoop(t *testing.T) {
	store := &retentionStore{calls: make(chan struct{}, 1)}
	maintenance := NewMaintenance(
		store,
		retentionFlowLister{flows: []flow.Flow{{ID: "enabled", Enabled: true}}},
		pipelinedb.DefaultRetentionPolicy(),
		time.Millisecond,
		zerolog.Nop(),
	)
	maintenance.Start()

	select {
	case <-store.calls:
	case <-time.After(time.Second):
		t.Fatal("maintenance did not run on its scheduled interval")
	}
	maintenance.Stop()

	store.mu.Lock()
	calls := len(store.consumers)
	store.mu.Unlock()
	time.Sleep(5 * time.Millisecond)
	store.mu.Lock()
	defer store.mu.Unlock()
	assert.Len(t, store.consumers, calls, "Stop must prevent further database maintenance")
}
