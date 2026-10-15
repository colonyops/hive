// Package guard holds the request guards that need no HTTP: a bounded keyed
// rate limiter and the client-IP rule.
package guard

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Limiter is a keyed token bucket that admits count events per window. The
// key map is bounded at maxKeys and idle entries are evicted, so a flood of
// distinct keys (IPs, emails) cannot grow memory without limit.
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

// NewLimiter returns a Limiter for count events per window, count >= 1. A nil
// now means time.Now.
func NewLimiter(count int, window time.Duration, maxKeys int, idle time.Duration, now func() time.Time) *Limiter {
	if now == nil {
		now = time.Now
	}
	return &Limiter{
		limit:     rate.Limit(float64(count) / window.Seconds()),
		burst:     count,
		maxKeys:   maxKeys,
		idle:      idle,
		now:       now,
		entries:   make(map[string]*entry),
		lastSweep: now(),
	}
}

// Allow reports whether key may proceed now. When it may not, retryAfter is
// the wait until it could, in whole seconds and at least one, so callers can
// put it straight into Retry-After. A denied call spends no token.
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
			l.makeRoom()
		}
		e = &entry{lim: rate.NewLimiter(l.limit, l.burst)}
		l.entries[key] = e
	}
	e.lastSeen = now

	res := e.lim.ReserveN(now, 1)
	delay := res.DelayFrom(now)
	if delay == 0 {
		return true, 0
	}
	res.CancelAt(now)
	return false, max((delay+time.Second-1)/time.Second, 1) * time.Second
}

func (l *Limiter) sweep(now time.Time) {
	l.lastSweep = now
	for k, e := range l.entries {
		if now.Sub(e.lastSeen) > l.idle {
			delete(l.entries, k)
		}
	}
}

func (l *Limiter) makeRoom() {
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
