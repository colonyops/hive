package hc

import (
	"context"
	"testing"

	"github.com/colonyops/hive/internal/domain/hc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failingBlockerStore struct {
	*fakeHCStore
	err error
}

func (f *failingBlockerStore) AddBlocker(context.Context, string, string) error {
	return f.err
}

// ---------------------------------------------------------------------------
// hc.WouldCycle unit tests
// ---------------------------------------------------------------------------

func TestWouldCycle_SelfLoop(t *testing.T) {
	assert.True(t, hc.WouldCycle(nil, "A", "A"))
}

func TestWouldCycle_DirectCycle(t *testing.T) {
	edges := [][2]string{{"A", "B"}}
	// Adding B→A would create A→B→A
	assert.True(t, hc.WouldCycle(edges, "B", "A"))
}

func TestWouldCycle_NoCycle(t *testing.T) {
	edges := [][2]string{{"A", "B"}}
	// A→B already exists; adding B→C should be fine
	assert.False(t, hc.WouldCycle(edges, "B", "C"))
}

func TestWouldCycle_IndirectCycle(t *testing.T) {
	edges := [][2]string{{"A", "B"}, {"B", "C"}}
	// Adding C→A would create A→B→C→A
	assert.True(t, hc.WouldCycle(edges, "C", "A"))
}

func TestWouldCycle_DiamondNoCycle(t *testing.T) {
	// Diamond: A→C, B→C, A→B
	edges := [][2]string{{"A", "C"}, {"B", "C"}, {"A", "B"}}
	// No cycle in diamond — e.g. adding D→A should be fine
	assert.False(t, hc.WouldCycle(edges, "D", "A"))
}

// ---------------------------------------------------------------------------
// Service.AddBlocker tests
// ---------------------------------------------------------------------------

func TestAddBlocker_ReturnsTheStoreError(t *testing.T) {
	svc := newTestHoneycombService(&failingBlockerStore{fakeHCStore: newFakeHCStore(), err: hc.ErrCyclicDependency})

	err := svc.AddBlocker(context.Background(), "A", "B")

	require.ErrorIs(t, err, hc.ErrCyclicDependency)
}
