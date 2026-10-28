package daemon

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/colonyops/hive/internal/domain/session"
	daemonv1 "github.com/colonyops/hive/internal/hive/daemon/gen/hive/daemon/v1"
	"github.com/colonyops/hive/internal/hive/status"
)

const (
	// statusIdleAfter pauses the loop when no client has asked for statuses,
	// so an idle daemon does not poll tmux.
	statusIdleAfter = 30 * time.Second
	// rootTTL drops a workspace root no client has asked about lately.
	rootTTL = 30 * time.Second
)

type statusLoopDeps struct {
	logger    zerolog.Logger
	now       func() time.Time
	interval  func() time.Duration
	available func() bool
	list      func(ctx context.Context) ([]session.Session, error)
	fetch     func(ctx context.Context, sessions []*session.Session, roots []status.RootRepoTarget) map[string]status.TerminalStatus
	// onTick and onError feed metrics; either may be nil.
	onTick  func(time.Duration)
	onError func()
}

// statusLoop is the only session-view caller of status.Service.FetchBatch, so
// the Stage 2 tracker sees one poll per interval however many clients ask.
// Clients read the snapshot of the last tick.
type statusLoop struct {
	statusLoopDeps

	wake chan struct{}

	mu           sync.Mutex
	snap         map[string]status.TerminalStatus
	roots        map[string]liveRoot
	lastRequest  time.Time
	paused       bool
	lastTick     time.Time
	lastDuration time.Duration
	errors       int64
}

type liveRoot struct {
	target   status.RootRepoTarget
	lastSeen time.Time
}

func newStatusLoop(deps statusLoopDeps) *statusLoop {
	if deps.now == nil {
		deps.now = time.Now
	}
	return &statusLoop{
		statusLoopDeps: deps,
		wake:           make(chan struct{}, 1),
		snap:           map[string]status.TerminalStatus{},
		roots:          map[string]liveRoot{},
		lastRequest:    deps.now(),
	}
}

func (l *statusLoop) run(ctx context.Context) {
	for {
		timer := time.NewTimer(l.interval())
		if l.isPaused() {
			timer.Stop()
			select {
			case <-ctx.Done():
				return
			case <-l.wake:
			}
		} else {
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
		l.tick(ctx)
	}
}

func (l *statusLoop) isPaused() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.paused
}

// tick runs one FetchBatch over every session and the live roots. It pauses
// the loop instead when no client asked within statusIdleAfter.
func (l *statusLoop) tick(ctx context.Context) {
	now := l.now()
	l.mu.Lock()
	if now.Sub(l.lastRequest) > statusIdleAfter {
		if !l.paused {
			l.logger.Debug().Msg("status loop paused: no client asked recently")
		}
		l.paused = true
		// Nothing polls while paused, so the snapshot ages without bound.
		// The first request after the pause reads loading, not old statuses.
		l.snap = map[string]status.TerminalStatus{}
		l.mu.Unlock()
		return
	}
	roots := make([]status.RootRepoTarget, 0, len(l.roots))
	for path, root := range l.roots {
		if now.Sub(root.lastSeen) > rootTTL {
			delete(l.roots, path)
			continue
		}
		roots = append(roots, root.target)
	}
	l.mu.Unlock()

	if !l.available() {
		return
	}
	all, err := l.list(ctx)
	if err != nil {
		l.logger.Warn().Err(err).Msg("status loop: list sessions")
		l.mu.Lock()
		l.errors++
		l.mu.Unlock()
		if l.onError != nil {
			l.onError()
		}
		return
	}
	sessions := make([]*session.Session, 0, len(all))
	for i := range all {
		sessions = append(sessions, &all[i])
	}
	results := l.fetch(ctx, sessions, roots)

	l.mu.Lock()
	l.snap = results
	l.lastTick = now
	l.lastDuration = l.now().Sub(now)
	took := l.lastDuration
	l.mu.Unlock()
	if l.onTick != nil {
		l.onTick(took)
	}
}

// snapshot answers from the last tick. A session or root the loop has not
// fetched yet reads as loading with presence unknown, so a client does not
// take it for a terminal that ended. A non-active session is absent, as in
// FetchBatch. A root named here joins the next tick, never an extra fetch.
func (l *statusLoop) snapshot(sessions []*session.Session, roots []status.RootRepoTarget) map[string]status.TerminalStatus {
	out := map[string]status.TerminalStatus{}
	if !l.available() {
		return out
	}
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()
	l.lastRequest = now
	if l.paused {
		l.paused = false
		select {
		case l.wake <- struct{}{}:
		default:
		}
	}
	for _, sess := range sessions {
		if sess.State != session.StateActive {
			continue
		}
		out[sess.ID] = l.entry(sess.ID)
	}
	for _, root := range roots {
		l.roots[root.Path] = liveRoot{target: root, lastSeen: now}
		key := status.RootStatusKey(root.Path)
		out[key] = l.entry(key)
	}
	return out
}

func (l *statusLoop) entry(key string) status.TerminalStatus {
	if s, ok := l.snap[key]; ok {
		return s
	}
	return status.TerminalStatus{IsLoading: true}
}

func (l *statusLoop) health() *daemonv1.StatusLoopHealth {
	l.mu.Lock()
	defer l.mu.Unlock()
	h := &daemonv1.StatusLoopHealth{
		LastDurationMs: l.lastDuration.Milliseconds(),
		Errors:         l.errors,
		Paused:         l.paused,
	}
	if !l.lastTick.IsZero() {
		h.LastTick = timestamppb.New(l.lastTick)
	}
	return h
}
