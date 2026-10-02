package app

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
	"github.com/colonyops/hive/cmd/desktop/internal/app/hivewatch"
)

type fakeSessionLister struct {
	mu       sync.Mutex
	sessions []dispatch.SessionSummary
	calls    int
}

// ListSessions reverses its order on every second read, so a repeated read of
// the same set proves store order is not a change.
func (f *fakeSessionLister) ListSessions(context.Context) ([]dispatch.SessionSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	out := slices.Clone(f.sessions)
	if f.calls%2 == 0 {
		slices.Reverse(out)
	}
	return out, nil
}

func (f *fakeSessionLister) set(sessions ...dispatch.SessionSummary) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessions = sessions
}

type fakeTasksFingerprinter struct {
	mu sync.Mutex
	fp dispatch.TasksFingerprint
}

func (f *fakeTasksFingerprinter) TasksFingerprint(context.Context) (dispatch.TasksFingerprint, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.fp, nil
}

func summary(id, name string) dispatch.SessionSummary {
	return dispatch.SessionSummary{ID: id, Name: name, Slug: name, Repo: "github.com/colonyops/hive", State: "active"}
}

func watcherFor(probes ...hivewatch.Probe) *hivewatch.Watcher {
	return hivewatch.New(time.Hour, zerolog.Nop(), probes...)
}

func TestSessionsProbe_OrderIsNotAChange(t *testing.T) {
	bus := newTestBus(t)
	updates := subscribeEvents[events.SessionsUpdated](t, bus)
	lister := &fakeSessionLister{sessions: []dispatch.SessionSummary{summary("a", "one"), summary("b", "two")}}
	w := watcherFor(sessionsProbe(lister, bus))

	w.Tick(t.Context())
	w.Tick(t.Context())

	requireNoMoreEvents(t, updates)
}

func TestSessionsProbe_PublishesTheDelta(t *testing.T) {
	bus := newTestBus(t)
	updates := subscribeEvents[events.SessionsUpdated](t, bus)
	lister := &fakeSessionLister{sessions: []dispatch.SessionSummary{summary("a", "one"), summary("b", "two"), summary("c", "three")}}
	w := watcherFor(sessionsProbe(lister, bus))
	w.Tick(t.Context())

	recycled := summary("b", "two")
	recycled.State = "recycled"
	lister.set(summary("a", "one"), recycled, summary("d", "four"))
	w.Tick(t.Context())

	got := requireEvents(t, updates, 1)
	require.Equal(t, events.SessionsUpdated{Added: []string{"d"}, Changed: []string{"b"}, Removed: []string{"c"}}, got[0])
}

func TestTasksProbe_PublishesOnlyWhenTheFingerprintChanges(t *testing.T) {
	bus := newTestBus(t)
	updates := subscribeEvents[events.TasksUpdated](t, bus)
	tasks := &fakeTasksFingerprinter{fp: dispatch.TasksFingerprint{Items: 3}}
	w := watcherFor(tasksProbe(tasks, bus))

	w.Tick(t.Context())
	w.Tick(t.Context())
	requireNoMoreEvents(t, updates)

	tasks.mu.Lock()
	tasks.fp.Comments++
	tasks.mu.Unlock()
	w.Tick(t.Context())

	requireEvents(t, updates, 1)
}
