package guard

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/relay/internal/relay"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newClock() *clock { return &clock{t: time.Unix(1_700_000_000, 0)} }

var perSecond = relay.Rate{Per: time.Second, Count: 1, Burst: 3}

func TestLimiterBurstThenDeny(t *testing.T) {
	c := newClock()
	l := NewLimiter(perSecond, 10, time.Minute, c.Now)

	for range 3 {
		ok, _ := l.Allow("a")
		require.True(t, ok)
	}
	ok, retry := l.Allow("a")
	assert.False(t, ok)
	assert.Equal(t, time.Second, retry)

	c.Advance(1100 * time.Millisecond)
	ok, _ = l.Allow("a")
	assert.True(t, ok)
}

func TestLimiterDeniedCallDoesNotConsumeTokens(t *testing.T) {
	c := newClock()
	l := NewLimiter(perSecond, 10, time.Minute, c.Now)
	for range 3 {
		l.Allow("a")
	}
	for range 5 {
		ok, _ := l.Allow("a")
		require.False(t, ok)
	}
	c.Advance(time.Second)
	ok, _ := l.Allow("a")
	assert.True(t, ok)
}

func TestLimiterRoundsRetryUp(t *testing.T) {
	c := newClock()
	l := NewLimiter(relay.Rate{Per: time.Second, Count: 10, Burst: 1}, 10, time.Minute, c.Now)
	ok, _ := l.Allow("a")
	require.True(t, ok)
	ok, retry := l.Allow("a")
	assert.False(t, ok)
	assert.Equal(t, time.Second, retry)
}

func TestLimiterBurstDefaultsToCount(t *testing.T) {
	c := newClock()
	l := NewLimiter(relay.Rate{Per: time.Minute, Count: 2}, 10, time.Hour, c.Now)
	for range 2 {
		ok, _ := l.Allow("a")
		require.True(t, ok)
	}
	ok, _ := l.Allow("a")
	assert.False(t, ok)
}

func TestLimiterKeysAreIndependent(t *testing.T) {
	c := newClock()
	l := NewLimiter(perSecond, 10, time.Minute, c.Now)
	for range 3 {
		l.Allow("a")
	}
	ok, _ := l.Allow("a")
	require.False(t, ok)
	ok, _ = l.Allow("b")
	assert.True(t, ok)
}

func TestLimiterEvictsLeastRecentlySeenWhenFull(t *testing.T) {
	c := newClock()
	l := NewLimiter(perSecond, 2, time.Hour, c.Now)

	for range 3 {
		l.Allow("a")
	}
	c.Advance(time.Millisecond)
	l.Allow("b")
	c.Advance(time.Millisecond)
	l.Allow("a") // denied, but refreshes lastSeen
	c.Advance(time.Millisecond)
	l.Allow("c") // evicts b

	l.mu.Lock()
	defer l.mu.Unlock()
	assert.Len(t, l.entries, 2)
	assert.Contains(t, l.entries, "a")
	assert.Contains(t, l.entries, "c")
	assert.NotContains(t, l.entries, "b")
}

func TestLimiterIdleEntryGetsFreshBucket(t *testing.T) {
	c := newClock()
	l := NewLimiter(relay.Rate{Per: time.Hour, Count: 1, Burst: 3}, 10, time.Minute, c.Now)
	for range 3 {
		l.Allow("a")
	}
	ok, _ := l.Allow("a")
	require.False(t, ok)

	c.Advance(2 * time.Minute)
	l.Allow("other") // triggers the periodic sweep

	l.mu.Lock()
	assert.NotContains(t, l.entries, "a")
	l.mu.Unlock()

	for range 3 {
		ok, _ := l.Allow("a")
		assert.True(t, ok)
	}
}

func TestLimiterConcurrent(t *testing.T) {
	l := NewLimiter(relay.Rate{Per: time.Second, Count: 1000, Burst: 1000}, 8, time.Minute, nil)
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range 100 {
				l.Allow(strconv.Itoa((i + j) % 20))
			}
		}()
	}
	wg.Wait()

	l.mu.Lock()
	defer l.mu.Unlock()
	assert.LessOrEqual(t, len(l.entries), 8)
}

func TestSemaphore(t *testing.T) {
	s := NewSemaphore(2)
	r1, ok := s.TryAcquire()
	require.True(t, ok)
	_, ok = s.TryAcquire()
	require.True(t, ok)
	_, ok = s.TryAcquire()
	assert.False(t, ok)

	r1()
	_, ok = s.TryAcquire()
	require.True(t, ok)

	r1() // double release must not free a second slot
	_, ok = s.TryAcquire()
	assert.False(t, ok)
}

func TestClientIP(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("::1/128")}

	tests := []struct {
		name    string
		remote  string
		xff     []string
		trusted []netip.Prefix
		want    string
	}{
		{"plain addr:port", "192.0.2.5:4000", nil, nil, "192.0.2.5"},
		{"ipv6 brackets", "[2001:db8::1]:4000", nil, nil, "2001:db8::1"},
		{"spoofed xff ignored without trusted", "192.0.2.5:4000", []string{"1.2.3.4"}, nil, "192.0.2.5"},
		{"spoofed xff ignored from untrusted peer", "192.0.2.5:4000", []string{"1.2.3.4"}, trusted, "192.0.2.5"},
		{"trusted peer uses xff", "10.0.0.1:80", []string{"1.2.3.4"}, trusted, "1.2.3.4"},
		{"trusted proxies skipped from the right", "10.0.0.1:80", []string{"1.2.3.4, 10.0.0.2"}, trusted, "1.2.3.4"},
		{"client-forged left entry is not reached", "10.0.0.1:80", []string{"9.9.9.9, 1.2.3.4"}, trusted, "1.2.3.4"},
		{"xff across two header values", "10.0.0.1:80", []string{"1.2.3.4", "10.0.0.2"}, trusted, "1.2.3.4"},
		{"unparsable entry returns peer", "10.0.0.1:80", []string{"1.2.3.4, garbage"}, trusted, "10.0.0.1"},
		{"all entries trusted returns peer", "10.0.0.1:80", []string{"10.0.0.3, 10.0.0.2"}, trusted, "10.0.0.1"},
		{"ipv4-mapped peer unmaps", "[::ffff:192.0.2.7]:4000", nil, nil, "192.0.2.7"},
		{"ipv4-mapped xff unmaps", "10.0.0.1:80", []string{"::ffff:1.2.3.4"}, trusted, "1.2.3.4"},
		{"bare addr without port", "192.0.2.9", nil, nil, "192.0.2.9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tt.remote
			for _, v := range tt.xff {
				r.Header.Add("X-Forwarded-For", v)
			}
			assert.Equal(t, netip.MustParseAddr(tt.want), ClientIP(r, tt.trusted))
		})
	}
}

func TestClientIPUnparsablePeer(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "not-an-address"
	assert.False(t, ClientIP(r, nil).IsValid())
}
