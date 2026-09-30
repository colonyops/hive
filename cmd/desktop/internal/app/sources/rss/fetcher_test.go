package rss

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests do not run in parallel: they assert on deltas of a cumulative
// counter that every fetch in this package writes to.

const oneEntry = `<rss version="2.0"><channel><title>Example</title>
  <item><guid>a</guid><title>A</title><link>https://example.com/a</link></item>
</channel></rss>`

func newFetchers(t *testing.T) *Fetchers {
	t.Helper()
	return NewFetchers(zerolog.Nop())
}

func serveFeed(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server.URL + "/feed.xml"
}

// The whole point of holding validators: a feed that has not published costs
// one round trip and no parse.
func TestFetchSendsValidatorsAndAnswersA304FromTheCachedWindow(t *testing.T) {
	var requests []string
	url := serveFeed(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Header.Get("If-None-Match"))
		w.Header().Set("ETag", `"v1"`)
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		_, _ = w.Write([]byte(oneEntry))
	})

	fetchers := newFetchers(t)
	before := fetchCount(t, resultNotModified)

	first, err := fetchers.Fetch(t.Context(), url)
	require.NoError(t, err)
	require.Len(t, first, 1)

	second, err := fetchers.Fetch(t.Context(), url)
	require.NoError(t, err)

	assert.Equal(t, []string{"", `"v1"`}, requests, "the second request is conditional on the first response's ETag")
	assert.Equal(t, first, second, "a 304 re-emits the window rather than an empty one")
	assert.Equal(t, before+1, fetchCount(t, resultNotModified))
}

// An empty window is the failure this connector must never produce: every
// downstream node reads it as "the feed published nothing".
func TestFetchAfterInvalidateIsUnconditional(t *testing.T) {
	var conditional []bool
	url := serveFeed(t, func(w http.ResponseWriter, r *http.Request) {
		conditional = append(conditional, r.Header.Get("If-None-Match") != "")
		w.Header().Set("ETag", `"v1"`)
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		_, _ = w.Write([]byte(oneEntry))
	})

	fetchers := newFetchers(t)
	_, err := fetchers.Fetch(t.Context(), url)
	require.NoError(t, err)

	fetchers.InvalidateAll()
	entries, err := fetchers.Fetch(t.Context(), url)
	require.NoError(t, err)

	assert.Equal(t, []bool{false, false}, conditional, "a dropped cache has no window to answer a 304 with")
	assert.Len(t, entries, 1)
}

// Two nodes on one feed are one subscription, not two.
func TestFetchSharesOneCacheAcrossCallers(t *testing.T) {
	var hits int
	url := serveFeed(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("ETag", `"v1"`)
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		_, _ = w.Write([]byte(oneEntry))
	})

	fetchers := newFetchers(t)
	for range 3 {
		_, err := fetchers.Fetch(t.Context(), url)
		require.NoError(t, err)
	}
	assert.Equal(t, 3, hits, "each drain still asks; only the body and the parse are saved")
}

// An HTML error page served with a 200 is the common shape of a feed that
// moved, and the one a parser has to reject rather than ingest as empty.
func TestFetchFailsOnADocumentThatIsNotAFeed(t *testing.T) {
	url := serveFeed(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html><body>Not found</body></html>"))
	})

	before := fetchCount(t, resultParseFailure)
	_, err := newFetchers(t).Fetch(t.Context(), url)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a feed")
	assert.Equal(t, before+1, fetchCount(t, resultParseFailure))
}

func TestFetchFailsOnANonSuccessStatus(t *testing.T) {
	url := serveFeed(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	before := fetchCount(t, resultHTTPError)
	_, err := newFetchers(t).Fetch(t.Context(), url)

	require.Error(t, err)
	assert.Equal(t, before+1, fetchCount(t, resultHTTPError))
}

func TestFetchFailsOnAnUnreachableHost(t *testing.T) {
	before := fetchCount(t, resultUnreachable)
	_, err := newFetchers(t).Fetch(t.Context(), "http://127.0.0.1:1/feed.xml")

	require.Error(t, err)
	assert.Equal(t, before+1, fetchCount(t, resultUnreachable))
}

// Truncating would hand the parser a short document, and a short parse is a
// smaller window than the feed published.
func TestFetchFailsRatherThanTruncateAnOversizedDocument(t *testing.T) {
	url := serveFeed(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<rss version="2.0"><channel><title>` + strings.Repeat("x", maxBody) + `</title></channel></rss>`))
	})

	before := fetchCount(t, resultTooLarge)
	_, err := newFetchers(t).Fetch(t.Context(), url)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "larger than")
	assert.Equal(t, before+1, fetchCount(t, resultTooLarge))
}

// A default Go user agent is what a fair number of CDNs answer with a 403.
func TestFetchSendsAUserAgentAndAccept(t *testing.T) {
	var got http.Header
	url := serveFeed(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		_, _ = w.Write([]byte(oneEntry))
	})

	_, err := newFetchers(t).Fetch(t.Context(), url)
	require.NoError(t, err)

	assert.Equal(t, userAgent, got.Get("User-Agent"))
	assert.Contains(t, got.Get("Accept"), "application/atom+xml")
}
