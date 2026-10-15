package guard_test

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/drive-server/internal/guard"
)

func TestLimiterBurstThenDenyWithRoundedRetry(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	l := guard.NewLimiter(3, time.Hour, 10, time.Hour, func() time.Time { return now })

	for range 3 {
		ok, _ := l.Allow("a")
		require.True(t, ok)
	}
	ok, wait := l.Allow("a")
	assert.False(t, ok)
	assert.Equal(t, 20*time.Minute, wait)

	ok, _ = l.Allow("b")
	assert.True(t, ok, "keys are independent")

	now = now.Add(20 * time.Minute)
	ok, _ = l.Allow("a")
	assert.True(t, ok, "one token refills per window/count")
}

func TestLimiterDeniedCallSpendsNothing(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	l := guard.NewLimiter(1, time.Minute, 10, time.Hour, func() time.Time { return now })
	l.Allow("a")
	for range 5 {
		_, wait := l.Allow("a")
		assert.Equal(t, time.Minute, wait)
	}
}

func TestLimiterEvictsOldestWhenFull(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	l := guard.NewLimiter(1, time.Hour, 2, 24*time.Hour, func() time.Time { return now })
	l.Allow("old")
	now = now.Add(time.Second)
	l.Allow("mid")
	now = now.Add(time.Second)
	l.Allow("new") // evicts "old"

	ok, _ := l.Allow("old")
	assert.True(t, ok, "an evicted key starts a fresh bucket")
	ok, _ = l.Allow("new")
	assert.False(t, ok)
}

func request(remote string, xff ...string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = remote
	for _, v := range xff {
		r.Header.Add("X-Forwarded-For", v)
	}
	return r
}

func TestClientIPIgnoresForwardedForWithoutTrustedProxies(t *testing.T) {
	got := guard.ClientIP(request("203.0.113.9:4000", "1.2.3.4"), nil)
	assert.Equal(t, netip.MustParseAddr("203.0.113.9"), got)
}

func TestClientIPWalksForwardedForFromTheRight(t *testing.T) {
	trusted, err := guard.ParseProxies([]string{"10.0.0.0/8", "192.168.1.1"})
	require.NoError(t, err)

	got := guard.ClientIP(request("10.0.0.2:4000", "6.6.6.6, 198.51.100.7, 10.0.0.3"), trusted)
	assert.Equal(t, netip.MustParseAddr("198.51.100.7"), got)

	got = guard.ClientIP(request("203.0.113.9:4000", "1.2.3.4"), trusted)
	assert.Equal(t, netip.MustParseAddr("203.0.113.9"), got, "an untrusted peer cannot set the header")

	got = guard.ClientIP(request("10.0.0.2:4000", "garbage"), trusted)
	assert.Equal(t, netip.MustParseAddr("10.0.0.2"), got)
}

func TestParseProxiesRejectsGarbage(t *testing.T) {
	_, err := guard.ParseProxies([]string{"not-an-ip"})
	assert.Error(t, err)
}
