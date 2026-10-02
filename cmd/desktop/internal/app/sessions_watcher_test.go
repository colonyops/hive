package app

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
)

type fakeSessionLister struct {
	mu       sync.Mutex
	sessions []dispatch.SessionSummary
	err      error
	calls    int
}

// ListSessions alternates the order it returns the set in, so a repeated read
// of the same set proves store order is not a change.
func (f *fakeSessionLister) ListSessions(context.Context) ([]dispatch.SessionSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	out := slices.Clone(f.sessions)
	if f.calls%2 == 0 {
		slices.Reverse(out)
	}
	return out, nil
}

func (f *fakeSessionLister) set(sessions []dispatch.SessionSummary, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessions = sessions
	f.err = err
}

func (f *fakeSessionLister) reads() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func summary(id, name string) dispatch.SessionSummary {
	return dispatch.SessionSummary{ID: id, Name: name, Slug: name, Repo: "github.com/colonyops/hive", State: "active"}
}

func newWatcherUnderTest(t *testing.T, lister *fakeSessionLister, interval time.Duration) (*sessionsWatcher, <-chan events.SessionsUpdated) {
	t.Helper()
	bus := newTestBus(t)
	updates := subscribeEvents[events.SessionsUpdated](t, bus)
	return newSessionsWatcher(lister, bus, interval, zerolog.Nop()), updates
}

func TestSessionsWatcherTick_FirstReadIsTheBaseline(t *testing.T) {
	lister := &fakeSessionLister{sessions: []dispatch.SessionSummary{summary("a", "one")}}
	watcher, updates := newWatcherUnderTest(t, lister, time.Hour)

	watcher.Tick(t.Context())

	requireNoMoreEvents(t, updates)
}

func TestSessionsWatcherTick_OrderIsNotAChange(t *testing.T) {
	lister := &fakeSessionLister{sessions: []dispatch.SessionSummary{summary("a", "one"), summary("b", "two")}}
	watcher, updates := newWatcherUnderTest(t, lister, time.Hour)

	watcher.Tick(t.Context())
	watcher.Tick(t.Context())

	requireNoMoreEvents(t, updates)
}

func TestSessionsWatcherTick_PublishesTheDelta(t *testing.T) {
	lister := &fakeSessionLister{sessions: []dispatch.SessionSummary{summary("a", "one"), summary("b", "two"), summary("c", "three")}}
	watcher, updates := newWatcherUnderTest(t, lister, time.Hour)
	watcher.Tick(t.Context())

	renamed := summary("b", "two-renamed")
	lister.set([]dispatch.SessionSummary{summary("a", "one"), renamed, summary("d", "four")}, nil)
	watcher.Tick(t.Context())

	got := requireEvents(t, updates, 1)
	require.Equal(t, events.SessionsUpdated{Added: []string{"d"}, Changed: []string{"b"}, Removed: []string{"c"}}, got[0])
}

func TestSessionsWatcherTick_StateChangeIsAChange(t *testing.T) {
	lister := &fakeSessionLister{sessions: []dispatch.SessionSummary{summary("a", "one")}}
	watcher, updates := newWatcherUnderTest(t, lister, time.Hour)
	watcher.Tick(t.Context())

	recycled := summary("a", "one")
	recycled.State = "recycled"
	lister.set([]dispatch.SessionSummary{recycled}, nil)
	watcher.Tick(t.Context())

	got := requireEvents(t, updates, 1)
	require.Equal(t, events.SessionsUpdated{Changed: []string{"a"}}, got[0])
}

func TestSessionsWatcherTick_FailedReadKeepsTheBaseline(t *testing.T) {
	lister := &fakeSessionLister{sessions: []dispatch.SessionSummary{summary("a", "one")}}
	watcher, updates := newWatcherUnderTest(t, lister, time.Hour)
	watcher.Tick(t.Context())

	lister.set(nil, errors.New("database is locked"))
	watcher.Tick(t.Context())
	requireNoMoreEvents(t, updates)

	// The change the failed read hid is reported once a read succeeds.
	lister.set([]dispatch.SessionSummary{summary("a", "one"), summary("b", "two")}, nil)
	watcher.Tick(t.Context())

	got := requireEvents(t, updates, 1)
	require.Equal(t, events.SessionsUpdated{Added: []string{"b"}}, got[0])
}

func TestSessionsWatcherStart_PrimesBeforeReturning(t *testing.T) {
	lister := &fakeSessionLister{sessions: []dispatch.SessionSummary{summary("a", "one")}}
	watcher, updates := newWatcherUnderTest(t, lister, time.Hour)

	watcher.Start(t.Context())
	t.Cleanup(watcher.Stop)

	require.Equal(t, 1, lister.reads(), "Start reads the baseline before returning")
	requireNoMoreEvents(t, updates)
}

func TestSessionsWatcherStart_PublishesAChangeTheLoopReads(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		lister := &fakeSessionLister{sessions: []dispatch.SessionSummary{summary("a", "one")}}
		watcher, updates := newWatcherUnderTest(t, lister, sessionsWatchInterval)
		watcher.Start(t.Context())
		t.Cleanup(watcher.Stop)

		lister.set([]dispatch.SessionSummary{summary("a", "one"), summary("b", "two")}, nil)
		time.Sleep(sessionsWatchInterval)
		synctest.Wait()

		got := requireEvents(t, updates, 1)
		require.Equal(t, events.SessionsUpdated{Added: []string{"b"}}, got[0])
	})
}

func TestSessionsWatcherStop_WaitsForTheLoop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		lister := &fakeSessionLister{}
		watcher, _ := newWatcherUnderTest(t, lister, sessionsWatchInterval)
		watcher.Start(t.Context())
		time.Sleep(sessionsWatchInterval)
		synctest.Wait()
		require.Equal(t, 2, lister.reads(), "the baseline and one tick")

		watcher.Stop()
		watcher.Stop()

		time.Sleep(time.Hour)
		synctest.Wait()
		require.Equal(t, 2, lister.reads(), "a read ran after Stop returned")
	})
}

func TestSessionsWatcherStop_BeforeStartIsANoop(t *testing.T) {
	watcher, _ := newWatcherUnderTest(t, &fakeSessionLister{}, time.Hour)
	watcher.Stop()
}
