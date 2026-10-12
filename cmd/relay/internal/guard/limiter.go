// Package guard holds the relay's HTTP-free request guards: a bounded keyed
// rate limiter, a semaphore, and the client-IP rule.
package guard

import (
	"math"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/colonyops/hive/cmd/relay/internal/relay"
)

// Limiter is a keyed token bucket: one rate.Limiter per key, with the map
// bounded at maxKeys and idle entries evicted so a flood of distinct keys
// (IPs, tokens) cannot grow memory without limit.
type Limiter struct {
	limit   rate.Limit
	burst   int
	maxKeys int
	idle    time.Duration
	now     func() time.Time

	mu        sync.Mutex
	entries   map[string]*entry
	lastSweep time.Time
}

type entry struct {
	lim      *rate.Limiter
	lastSeen time.Time
}

func NewLimiter(r relay.Rate, maxKeys int, idle time.Duration, now func() time.Time) *Limiter {
	if now == nil {
		now = time.Now
	}
	burst := r.Burst
	if burst <= 0 {
		burst = r.Count
	}
	return &Limiter{
		limit:     rate.Limit(float64(r.Count) / r.Per.Seconds()),
		burst:     burst,
		maxKeys:   maxKeys,
		idle:      idle,
		now:       now,
		entries:   make(map[string]*entry),
		lastSweep: now(),
	}
}

// Allow reports whether key may proceed now. When it may not, retryAfter is
// the wait until it could, rounded up to whole seconds with a minimum of one
// second so callers can put it straight into Retry-After.
func (l *Limiter) Allow(key string) (ok bool, retryAfter time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	if now.Sub(l.lastSweep) >= l.idle {
		l.sweep(now)
	}

	e, found := l.entries[key]
	if !found {
		if len(l.entries) >= l.maxKeys {
			l.makeRoom(now)
		}
		e = &entry{lim: rate.NewLimiter(l.limit, l.burst)}
		l.entries[key] = e
	}
	e.lastSeen = now

	res := e.lim.ReserveN(now, 1)
	if res.OK() {
		delay := res.DelayFrom(now)
		if delay == 0 {
			return true, 0
		}
		res.CancelAt(now)
		return false, ceilSeconds(delay)
	}
	// A burst below one token can never admit a request.
	return false, ceilSeconds(time.Duration(math.MaxInt64 - int64(time.Second)))
}

func ceilSeconds(d time.Duration) time.Duration {
	secs := (d + time.Second - 1) / time.Second
	if secs < 1 {
		secs = 1
	}
	return secs * time.Second
}

func (l *Limiter) sweep(now time.Time) {
	l.lastSweep = now
	for k, e := range l.entries {
		if now.Sub(e.lastSeen) > l.idle {
			delete(l.entries, k)
		}
	}
}

func (l *Limiter) makeRoom(now time.Time) {
	l.sweep(now)
	if len(l.entries) < l.maxKeys {
		return
	}
	var oldestKey string
	var oldest time.Time
	first := true
	for k, e := range l.entries {
		if first || e.lastSeen.Before(oldest) {
			oldestKey, oldest, first = k, e.lastSeen, false
		}
	}
	delete(l.entries, oldestKey)
}
