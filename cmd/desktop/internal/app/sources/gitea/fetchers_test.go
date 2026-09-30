package gitea

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/sourcehttp"
)

// Gitea has no batched query, so absence confirmation is a request per item run
// concurrently. Under -race this is what covers the parallel writes into the
// index-parallel result slice.
//
// Concurrency is proven by a barrier rather than by observing a peak: the
// handler blocks until absenceConcurrency requests are in flight together, so a
// sequential implementation never releases it and the assertion fails on the
// timeout instead of on whichever way the scheduler happened to run.
func TestItemStatesLooksUpEveryRefConcurrently(t *testing.T) {
	t.Parallel()

	var (
		inFlight atomic.Int64
		peak     atomic.Int64
		reached  = make(chan struct{})
		once     sync.Once
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := inFlight.Add(1)
		defer inFlight.Add(-1)
		for {
			seen := peak.Load()
			if current <= seen || peak.CompareAndSwap(seen, current) {
				break
			}
		}
		if current >= absenceConcurrency {
			once.Do(func() { close(reached) })
		}
		select {
		case <-reached:
		case <-time.After(2 * time.Second):
		}

		number := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		_, _ = fmt.Fprintf(w, `{"number":%s,"title":"Item %s","state":"closed",
			"html_url":"https://git.example.com/acme/app/issues/%s","updated_at":"2026-08-11T10:00:00Z",
			"pull_request":{"merged":true}}`, number, number, number)
	}))
	defer server.Close()

	fetchers, ref := connectedFetchers(t, server.URL)

	refs := make([]itemRef, 0, 12)
	for number := 1; number <= 12; number++ {
		refs = append(refs, itemRef{Repo: "acme/app", Num: number})
	}

	states, err := fetchers.For(parseRef(t, ref)).ItemStates(t.Context(), refs)
	require.NoError(t, err)
	require.Len(t, states, len(refs))

	for i, state := range states {
		assert.Truef(t, state.Found, "ref %d", i)
		assert.Equalf(t, "merged", state.State, "ref %d", i)
		assert.Equalf(t, fmt.Sprintf("Item %d", i+1), state.Title, "results are index-parallel to refs")
	}

	select {
	case <-reached:
	default:
		t.Error("the lookups never ran concurrently")
	}
	assert.LessOrEqual(t, peak.Load(), int64(absenceConcurrency), "the fan-out stays within its cap")
}

// An unaddressable ref is a verdict, not a failure: it comes back Found false
// and keeps its slot so the caller's pairing with its own list still holds.
func TestItemStatesKeepsUnaddressableRefsInPlace(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"number":2,"title":"Real","state":"open","updated_at":"2026-08-11T10:00:00Z"}`))
	}))
	defer server.Close()

	fetchers, ref := connectedFetchers(t, server.URL)
	states, err := fetchers.For(parseRef(t, ref)).ItemStates(t.Context(), []itemRef{
		{Repo: "no-slash", Num: 1},
		{Repo: "acme/app", Num: 2},
		{Repo: "acme/app", Num: 0},
	})
	require.NoError(t, err)
	require.Len(t, states, 3)

	assert.False(t, states[0].Found)
	assert.True(t, states[1].Found)
	assert.False(t, states[2].Found)
}

// A 429 arms the account's cooldown: sourcehttp classifies the response, but
// pausing polling is the provider's job.
func TestSearchArmsCooldownAndSkips(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	fetchers, ref := connectedFetchers(t, server.URL)
	fx := fetchers.For(parseRef(t, ref))
	cfg := &Config{Credential: ref, Kind: KindSearch}

	_, err := fx.Search(t.Context(), cfg)
	require.ErrorIs(t, err, sourcehttp.ErrRateLimited)
	require.EqualValues(t, 1, hits.Load())

	// The cooldown is armed, so the next poll must not reach the server.
	_, err = fx.Search(t.Context(), cfg)
	require.ErrorIs(t, err, sourcehttp.ErrRateLimited)
	assert.EqualValues(t, 1, hits.Load(), "a fetcher in cooldown must not hit the instance again")

	// Clearing the cooldowns (as connect/disconnect does) lets it poll again.
	fetchers.InvalidateAll()
	_, _ = fx.Search(t.Context(), cfg)
	assert.EqualValues(t, 2, hits.Load())
}

// A bare 429 carries no reset time; the fallback cooldown still arms.
func TestSearchArmsDefaultCooldownWithoutResetHeaders(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	fetchers, ref := connectedFetchers(t, server.URL)
	fx := fetchers.For(parseRef(t, ref))
	cfg := &Config{Credential: ref, Kind: KindSearch}

	_, err := fx.Search(t.Context(), cfg)
	require.ErrorIs(t, err, sourcehttp.ErrRateLimited)

	_, err = fx.Search(t.Context(), cfg)
	require.ErrorIs(t, err, sourcehttp.ErrRateLimited)
	assert.EqualValues(t, 1, hits.Load())
}

// One failed lookup fails the call: the caller cannot tell a lookup that never
// ran from an item that is genuinely gone, and guessing would archive it.
func TestItemStatesFailsWhenALookupFails(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	fetchers, ref := connectedFetchers(t, server.URL)
	_, err := fetchers.For(parseRef(t, ref)).ItemStates(t.Context(), []itemRef{{Repo: "acme/app", Num: 1}})
	assert.ErrorContains(t, err, "confirming absent items")
}
