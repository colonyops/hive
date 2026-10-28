package daemon

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/domain/terminal"
	"github.com/colonyops/hive/internal/hive/status"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type fakeFetch struct {
	mu        sync.Mutex
	calls     int
	lastRoots []status.RootRepoTarget
}

// fetch mirrors FetchBatch: active sessions and roots get a status, others
// are left out.
func (f *fakeFetch) fetch(_ context.Context, sessions []*session.Session, roots []status.RootRepoTarget) map[string]status.TerminalStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastRoots = roots
	out := map[string]status.TerminalStatus{}
	for _, s := range sessions {
		if s.State == session.StateActive {
			out[s.ID] = status.TerminalStatus{PresenceKnown: true, SessionPresent: true, Status: terminal.StatusReady}
		}
	}
	for _, r := range roots {
		out[status.RootStatusKey(r.Path)] = status.TerminalStatus{PresenceKnown: true, Status: terminal.StatusActive}
	}
	return out
}

func (f *fakeFetch) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

var (
	active   = session.Session{ID: "a", State: session.StateActive}
	recycled = session.Session{ID: "r", State: session.StateRecycled}
)

func newTestLoop(clock *fakeClock, fetch *fakeFetch) *statusLoop {
	return newStatusLoop(statusLoopDeps{
		logger:    zerolog.Nop(),
		now:       clock.Now,
		interval:  func() time.Duration { return time.Millisecond },
		available: func() bool { return true },
		list:      func(context.Context) ([]session.Session, error) { return []session.Session{active, recycled}, nil },
		fetch:     fetch.fetch,
	})
}

func TestStatusLoopServesManyCallersFromOneFetchPerTick(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1000, 0)}
	fetch := &fakeFetch{}
	loop := newTestLoop(clock, fetch)

	loop.tick(t.Context())
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			got := loop.snapshot([]*session.Session{&active}, nil)
			assert.Equal(t, terminal.StatusReady, got["a"].Status)
		})
	}
	wg.Wait()
	assert.Equal(t, 1, fetch.count())
}

func TestStatusLoopAnswersLoadingBeforeTheFirstTick(t *testing.T) {
	loop := newTestLoop(&fakeClock{now: time.Unix(1000, 0)}, &fakeFetch{})

	got := loop.snapshot([]*session.Session{&active, &recycled}, nil)

	require.Contains(t, got, "a")
	assert.True(t, got["a"].IsLoading)
	assert.False(t, got["a"].PresenceKnown, "a placeholder must not read as a terminal that ended")
	assert.Empty(t, got["a"].Status)
	assert.NotContains(t, got, "r", "a non-active session is absent, as in FetchBatch")
}

func TestStatusLoopAddsANewRootOnTheNextTick(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1000, 0)}
	fetch := &fakeFetch{}
	loop := newTestLoop(clock, fetch)
	root := status.RootRepoTarget{Name: "hive", Path: "/src/hive"}
	key := status.RootStatusKey(root.Path)

	got := loop.snapshot(nil, []status.RootRepoTarget{root})
	assert.True(t, got[key].IsLoading)
	assert.Equal(t, 0, fetch.count(), "naming a root never triggers a fetch of its own")

	loop.tick(t.Context())
	assert.Equal(t, []status.RootRepoTarget{root}, fetch.lastRoots)
	assert.Equal(t, terminal.StatusActive, loop.snapshot(nil, []status.RootRepoTarget{root})[key].Status)
}

func TestStatusLoopExpiresRootsNoClientAsksAbout(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1000, 0)}
	fetch := &fakeFetch{}
	loop := newTestLoop(clock, fetch)
	loop.snapshot(nil, []status.RootRepoTarget{{Name: "hive", Path: "/src/hive"}})

	for range 4 {
		clock.Advance(10 * time.Second)
		loop.snapshot([]*session.Session{&active}, nil)
		loop.tick(t.Context())
	}
	assert.Empty(t, fetch.lastRoots)
}

func TestStatusLoopPausesWhenIdleAndResumesOnARequest(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1000, 0)}
	fetch := &fakeFetch{}
	loop := newTestLoop(clock, fetch)

	loop.tick(t.Context())
	require.Equal(t, 1, fetch.count())

	clock.Advance(statusIdleAfter + time.Second)
	loop.tick(t.Context())
	assert.Equal(t, 1, fetch.count(), "an idle loop does not poll")
	assert.True(t, loop.health().GetPaused())

	got := loop.snapshot([]*session.Session{&active}, nil)
	assert.Equal(t, terminal.StatusReady, got["a"].Status, "the waking request gets the current snapshot")
	assert.False(t, loop.health().GetPaused())
	loop.tick(t.Context())
	assert.Equal(t, 2, fetch.count())
}

func TestStatusLoopRunWakesFromAPause(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1000, 0)}
	fetch := &fakeFetch{}
	loop := newTestLoop(clock, fetch)
	clock.Advance(statusIdleAfter + time.Second)

	ctx, cancel := context.WithCancel(t.Context())
	var stopped atomic.Bool
	go func() {
		loop.run(ctx)
		stopped.Store(true)
	}()
	t.Cleanup(func() {
		cancel()
		assert.Eventually(t, stopped.Load, time.Second, time.Millisecond)
	})

	require.Eventually(t, func() bool { return loop.health().GetPaused() }, time.Second, time.Millisecond)
	assert.Equal(t, 0, fetch.count())

	loop.snapshot([]*session.Session{&active}, nil)
	assert.Eventually(t, func() bool { return fetch.count() > 0 }, time.Second, time.Millisecond)
}

func TestStatusLoopAnswersNothingWithoutAStatusSource(t *testing.T) {
	loop := newTestLoop(&fakeClock{now: time.Unix(1000, 0)}, &fakeFetch{})
	loop.available = func() bool { return false }
	assert.Empty(t, loop.snapshot([]*session.Session{&active}, nil))
}
