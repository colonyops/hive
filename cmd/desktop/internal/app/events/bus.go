// Package events is the core's typed publish/subscribe bus. The core
// publishes a payload that says what happened; each adapter subscribes and
// degrades it to whatever its transport needs — the Wails adapter to a
// wake-up signal, a streaming consumer to the delta itself.
//
// Every payload type lives in events.go and implements the unexported
// eventName method, so an adapter cannot invent a core event and the whole
// event vocabulary is readable in one file.
package events

import (
	"context"
	"sync"

	"github.com/rs/zerolog"
)

// Event is any payload published on the bus.
type Event interface{ eventName() string }

// Policy is a subscriber's delivery contract. Construct one with Coalesce or
// Buffer; the zero value is not valid.
type Policy struct {
	coalesce bool
	size     int
	valid    bool
}

// Coalesce keeps only the newest undelivered event and never blocks the
// publisher. Correct for a GUI that re-reads state on wake-up, which is every
// wailsui subscriber.
func Coalesce() Policy { return Policy{coalesce: true, valid: true} }

// Buffer delivers every event in order through a queue of size n, blocking
// the publisher when it is full. Correct for a consumer that must see each
// message. n < 1 is treated as 1: an unbuffered queue would make every
// publish synchronous with its slowest subscriber.
//
// Policy is a constructor rather than an enum plus a size argument so that
// "coalesce with a queue size" is unrepresentable rather than merely wrong.
func Buffer(n int) Policy {
	if n < 1 {
		n = 1
	}
	return Policy{size: n, valid: true}
}

// Bus fans events out to subscribers. The zero value is not usable; call New.
type Bus struct {
	logger zerolog.Logger

	mu     sync.RWMutex
	subs   map[string][]*subscription
	closed bool

	wg sync.WaitGroup
}

type subscription struct {
	name string

	// coalesce state: one undelivered slot plus a wake signal.
	mu      sync.Mutex
	pending Event
	has     bool
	wake    chan struct{}

	// buffer state.
	queue chan Event

	stop     chan struct{}
	stopOnce sync.Once
}

func New(logger zerolog.Logger) *Bus {
	return &Bus{logger: logger, subs: map[string][]*subscription{}}
}

// Publish delivers e to every subscriber per its policy. It never panics on a
// slow or failed subscriber. A Buffer subscriber with a full queue blocks the
// publisher until there is room or ctx is done; a Coalesce subscriber never
// blocks it at all.
func (b *Bus) Publish(ctx context.Context, e Event) {
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return
	}
	subs := b.subs[e.eventName()]
	delivered := make([]*subscription, len(subs))
	copy(delivered, subs)
	b.mu.RUnlock()

	for _, s := range delivered {
		s.deliver(ctx, b.logger, e)
	}
}

func (s *subscription) deliver(ctx context.Context, logger zerolog.Logger, e Event) {
	if s.queue != nil {
		select {
		case s.queue <- e:
		case <-s.stop:
		case <-ctx.Done():
		}
		return
	}

	s.mu.Lock()
	dropped := s.has
	s.pending, s.has = e, true
	s.mu.Unlock()
	if dropped {
		// The event says what was dropped; the subscriber name says who
		// dropped it. Debug rather than warn: for a coalescing subscriber a
		// drop is the contract working, not a fault.
		logger.Debug().Ctx(ctx).Str("subscriber", s.name).Str("event", e.eventName()).Msg("events: coalesced an undelivered event")
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Close stops delivery and waits for in-flight handlers. It is safe to call
// more than once, and safe to call concurrently with Publish.
func (b *Bus) Close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	var all []*subscription
	for _, subs := range b.subs {
		all = append(all, subs...)
	}
	b.subs = map[string][]*subscription{}
	b.mu.Unlock()

	for _, s := range all {
		s.stopOnce.Do(func() { close(s.stop) })
	}
	b.wg.Wait()
}

// Subscribe registers fn for events of type E and runs it on its own
// goroutine until ctx is done, cancel is called, or the bus is closed.
//
// It is a package-level function rather than a method because Go methods
// cannot carry type parameters. ctx is the subscription's lifetime and the
// context every delivery is handled with — deliberately not the publisher's,
// because a subscriber's work must not be torn down because the request that
// triggered it returned. name identifies the subscriber in log lines when a
// handler panics or a coalescing queue drops.
func Subscribe[E Event](ctx context.Context, b *Bus, name string, p Policy, fn func(context.Context, E)) (cancel func()) {
	if !p.valid {
		panic("events: subscribe with a zero Policy; use Coalesce() or Buffer(n)")
	}

	var zero E
	s := &subscription{name: name, stop: make(chan struct{})}
	if p.coalesce {
		s.wake = make(chan struct{}, 1)
	} else {
		s.queue = make(chan Event, p.size)
	}

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return func() {}
	}
	b.subs[zero.eventName()] = append(b.subs[zero.eventName()], s)
	b.wg.Add(1)
	b.mu.Unlock()

	call := func(e Event) {
		typed, ok := e.(E)
		if !ok {
			return
		}
		defer func() {
			if r := recover(); r != nil {
				b.logger.Error().Interface("panic", r).Str("subscriber", name).Str("event", e.eventName()).Msg("events: subscriber panicked")
			}
		}()
		fn(ctx, typed)
	}

	go func() {
		defer b.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case <-s.stop:
				return
			case e := <-s.queue:
				call(e)
			case <-s.wake:
				s.mu.Lock()
				e, ok := s.pending, s.has
				s.pending, s.has = nil, false
				s.mu.Unlock()
				if ok {
					call(e)
				}
			}
		}
	}()

	return func() {
		s.stopOnce.Do(func() { close(s.stop) })
		b.mu.Lock()
		defer b.mu.Unlock()
		key := zero.eventName()
		remaining := b.subs[key][:0]
		for _, existing := range b.subs[key] {
			if existing != s {
				remaining = append(remaining, existing)
			}
		}
		b.subs[key] = remaining
	}
}
