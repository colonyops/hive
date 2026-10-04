// Package hivewatch observes hive.db state that another process writes. The
// hive CLI shares that database and has no way to signal the desktop, so the
// Watcher re-reads each registered probe on one ticker and reports a value
// that differs from the last read.
package hivewatch

import (
	"context"
	"sync"
	"time"

	"github.com/colonyops/hive/pkg/logutils"

	"github.com/rs/zerolog"
)

// Spec describes one slice of shared state. Read must be cheap: it runs on
// every tick for the life of the app. OnChange receives the previous and the
// new value and runs on the Watcher's goroutine.
type Spec[S any] struct {
	Name     string
	Read     func(context.Context) (S, error)
	Equal    func(a, b S) bool
	OnChange func(ctx context.Context, prev, next S)
}

// Probe is a Spec the Watcher runs. Build one with NewProbe.
type Probe interface {
	name() string
	tick(ctx context.Context, logger zerolog.Logger)
}

// NewProbe wraps spec with the last value it read. The first successful read
// is the baseline and reports nothing. A failed read keeps the baseline, so a
// change it hid is reported by the next read that succeeds.
func NewProbe[S any](spec Spec[S]) Probe {
	return &probe[S]{spec: spec}
}

type probe[S any] struct {
	spec    Spec[S]
	last    S
	primed  bool
	failing bool
}

func (p *probe[S]) name() string { return p.spec.Name }

func (p *probe[S]) tick(ctx context.Context, logger zerolog.Logger) {
	next, err := p.spec.Read(ctx)
	if err != nil {
		// Warn once per outage: a read that keeps failing every tick would
		// otherwise flood the log, and at debug it would be invisible.
		if p.failing {
			logger.Debug().Err(err).Str("probe", p.spec.Name).Msg("probe read still failing")
		} else {
			logger.Warn().Err(err).Str("probe", p.spec.Name).Msg("probe read failing; updates paused")
		}
		p.failing = true
		return
	}
	if p.failing {
		logger.Info().Str("probe", p.spec.Name).Msg("probe read recovered")
		p.failing = false
	}
	prev, primed := p.last, p.primed
	p.last, p.primed = next, true
	if primed && !p.spec.Equal(prev, next) {
		logger.Debug().Str("probe", p.spec.Name).Msg("probe changed")
		p.spec.OnChange(ctx, prev, next)
	}
}

// Watcher runs its probes in order on one goroutine, so a probe's state is
// never touched concurrently.
type Watcher struct {
	interval time.Duration
	logger   zerolog.Logger
	probes   []Probe

	mu       sync.Mutex
	started  bool
	stopOnce sync.Once
	stop     chan struct{}
	done     chan struct{}
}

func New(interval time.Duration, logger zerolog.Logger, probes ...Probe) *Watcher {
	return &Watcher{
		interval: interval,
		logger:   logutils.Component(logger, "hivewatch"),
		probes:   probes,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
}

// Start reads every baseline before returning, so a write that lands before
// the first tick is still reported, then polls until Stop or ctx ends.
func (w *Watcher) Start(ctx context.Context) {
	w.mu.Lock()
	if w.started {
		w.mu.Unlock()
		return
	}
	w.started = true
	w.mu.Unlock()

	w.Tick(ctx)
	names := make([]string, len(w.probes))
	for i, p := range w.probes {
		names[i] = p.name()
	}
	w.logger.Info().Dur("interval", w.interval).Strs("probes", names).Msg("watcher started")
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

// Stop ends the loop and waits for it, so hive.db can close without a read in
// flight. It is safe to call twice or without Start.
func (w *Watcher) Stop() {
	w.mu.Lock()
	started := w.started
	w.mu.Unlock()
	if !started {
		return
	}
	w.stopOnce.Do(func() {
		close(w.stop)
		<-w.done
		w.logger.Info().Msg("watcher stopped")
	})
}

// Tick runs every probe once. Only Start and the loop call it outside tests.
func (w *Watcher) Tick(ctx context.Context) {
	for _, p := range w.probes {
		p.tick(ctx, w.logger)
	}
}
