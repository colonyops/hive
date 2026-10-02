package app

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
)

// sessionsWatchInterval is how often the hive session set is re-read. A poll
// rather than a filesystem watch on hive.db: every table shares that one file
// and its WAL, so a watch cannot tell a session write from a message, task or
// status write and would wake the sidebar for all of them. One read of the
// sessions table every two seconds costs less and only publishes when the
// list the frontend shows actually changed.
const sessionsWatchInterval = 2 * time.Second

type sessionLister interface {
	ListSessions(context.Context) ([]dispatch.SessionSummary, error)
}

// sessionsWatcher publishes SessionsUpdated when the hive session set changes
// under this process: a session the CLI created, renamed, recycled or
// deleted. The app's own session jobs change the set too, so one of those
// also lands here one tick after its JobsUpdated; the second reload reads a
// small list and is harmless.
type sessionsWatcher struct {
	lister   sessionLister
	bus      *events.Bus
	interval time.Duration
	logger   zerolog.Logger

	// last is the set the previous successful read returned, sorted by id.
	// Only Tick touches it, and Tick runs on one goroutine at a time: Start
	// primes it before the loop exists, and the loop is the only caller after.
	last   []dispatch.SessionSummary
	primed bool

	mu       sync.Mutex
	started  bool
	stopOnce sync.Once
	stop     chan struct{}
	done     chan struct{}
}

func newSessionsWatcher(lister sessionLister, bus *events.Bus, interval time.Duration, logger zerolog.Logger) *sessionsWatcher {
	return &sessionsWatcher{
		lister:   lister,
		bus:      bus,
		interval: interval,
		logger:   logger,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
}

// Start reads the baseline before returning, so a session written before the
// first tick is still a change from it, then polls until Stop.
func (w *sessionsWatcher) Start(ctx context.Context) {
	w.mu.Lock()
	if w.started {
		w.mu.Unlock()
		return
	}
	w.started = true
	w.mu.Unlock()

	w.Tick(ctx)
	go func() {
		defer close(w.done)
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-w.stop:
				return
			case <-ticker.C:
				w.Tick(ctx)
			}
		}
	}()
}

// Stop ends the poll and waits for its goroutine, so the hive database can
// close without a read in flight.
func (w *sessionsWatcher) Stop() {
	w.mu.Lock()
	started := w.started
	w.mu.Unlock()
	if !started {
		return
	}
	w.stopOnce.Do(func() { close(w.stop) })
	<-w.done
}

// Tick reads the session set once and publishes the difference from the
// previous read. The first successful read is the baseline and publishes
// nothing. A failed read keeps the previous baseline, so the change it hid
// is reported by the next read that succeeds.
func (w *sessionsWatcher) Tick(ctx context.Context) {
	next, err := w.lister.ListSessions(ctx)
	if err != nil {
		w.logger.Debug().Err(err).Msg("sessions watcher: list sessions failed")
		return
	}
	slices.SortFunc(next, func(a, b dispatch.SessionSummary) int { return strings.Compare(a.ID, b.ID) })
	prev, primed := w.last, w.primed
	w.last, w.primed = next, true
	if !primed {
		return
	}
	added, changed, removed := diffSessions(prev, next)
	if len(added)+len(changed)+len(removed) == 0 {
		return
	}
	w.bus.Publish(ctx, events.SessionsUpdated{Added: added, Changed: changed, Removed: removed})
}

// diffSessions reports the ids in next but not prev, in both with any field
// different, and in prev but not next. Both inputs are sorted by id, so the
// results are too.
func diffSessions(prev, next []dispatch.SessionSummary) (added, changed, removed []string) {
	before := make(map[string]dispatch.SessionSummary, len(prev))
	for _, s := range prev {
		before[s.ID] = s
	}
	for _, s := range next {
		was, ok := before[s.ID]
		switch {
		case !ok:
			added = append(added, s.ID)
		case was != s:
			changed = append(changed, s.ID)
		}
		delete(before, s.ID)
	}
	for _, s := range prev {
		if _, gone := before[s.ID]; gone {
			removed = append(removed, s.ID)
		}
	}
	return added, changed, removed
}
