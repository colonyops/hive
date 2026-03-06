package hive

import (
	"context"
	"testing"

	"github.com/colonyops/hive/internal/core/hc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeBlockerStore extends fakeHCStore with controllable blocker edge state.
type fakeBlockerStore struct {
	*fakeHCStore
	edges [][2]string
}

func newFakeBlockerStore() *fakeBlockerStore {
	return &fakeBlockerStore{
		fakeHCStore: newFakeHCStore(),
	}
}

func (f *fakeBlockerStore) AddBlocker(_ context.Context, blockerID, blockedID string) error {
	f.edges = append(f.edges, [2]string{blockerID, blockedID})
	return nil
}

func (f *fakeBlockerStore) ListBlockerEdges(_ context.Context) ([][2]string, error) {
	return f.edges, nil
}

// ---------------------------------------------------------------------------
// wouldCycle unit tests
// ---------------------------------------------------------------------------

func TestWouldCycle_SelfLoop(t *testing.T) {
	assert.True(t, wouldCycle(nil, "A", "A"))
}

func TestWouldCycle_DirectCycle(t *testing.T) {
	edges := [][2]string{{"A", "B"}}
	// Adding B→A would create A→B→A
	assert.True(t, wouldCycle(edges, "B", "A"))
}

func TestWouldCycle_NoCycle(t *testing.T) {
	edges := [][2]string{{"A", "B"}}
	// A→B already exists; adding B→C should be fine
	assert.False(t, wouldCycle(edges, "B", "C"))
}

func TestWouldCycle_IndirectCycle(t *testing.T) {
	edges := [][2]string{{"A", "B"}, {"B", "C"}}
	// Adding C→A would create A→B→C→A
	assert.True(t, wouldCycle(edges, "C", "A"))
}

func TestWouldCycle_DiamondNoCycle(t *testing.T) {
	// Diamond: A→C, B→C, A→B
	edges := [][2]string{{"A", "C"}, {"B", "C"}, {"A", "B"}}
	// No cycle in diamond — e.g. adding D→A should be fine
	assert.False(t, wouldCycle(edges, "D", "A"))
}

// ---------------------------------------------------------------------------
// HoneycombService.AddBlocker tests
// ---------------------------------------------------------------------------

func TestAddBlocker_SelfBlock(t *testing.T) {
	store := newFakeBlockerStore()
	svc := newTestHoneycombService(store)

	err := svc.AddBlocker(context.Background(), "A", "A")
	require.Error(t, err)
	assert.ErrorIs(t, err, hc.ErrCyclicDependency)
}

func TestAddBlocker_DirectCycle(t *testing.T) {
	store := newFakeBlockerStore()
	svc := newTestHoneycombService(store)

	// Add A→B
	store.edges = append(store.edges, [2]string{"A", "B"})

	// Adding B→A should fail
	err := svc.AddBlocker(context.Background(), "B", "A")
	require.Error(t, err)
	assert.ErrorIs(t, err, hc.ErrCyclicDependency)
}

func TestAddBlocker_IndirectCycle(t *testing.T) {
	store := newFakeBlockerStore()
	svc := newTestHoneycombService(store)

	store.edges = [][2]string{{"A", "B"}, {"B", "C"}}

	// Adding C→A creates A→B→C→A
	err := svc.AddBlocker(context.Background(), "C", "A")
	require.Error(t, err)
	assert.ErrorIs(t, err, hc.ErrCyclicDependency)
}

func TestAddBlocker_NoCycle(t *testing.T) {
	store := newFakeBlockerStore()
	svc := newTestHoneycombService(store)

	store.edges = [][2]string{{"A", "B"}}

	// Adding B→C is fine
	err := svc.AddBlocker(context.Background(), "B", "C")
	require.NoError(t, err)
}

func TestAddBlocker_DiamondNoCycle(t *testing.T) {
	store := newFakeBlockerStore()
	svc := newTestHoneycombService(store)

	// A→C, B→C
	store.edges = [][2]string{{"A", "C"}, {"B", "C"}}

	// Adding A→B to form diamond shape should succeed
	err := svc.AddBlocker(context.Background(), "A", "B")
	require.NoError(t, err)
}
