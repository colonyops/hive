package pipeline

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/desktop/feed"
	"github.com/colonyops/hive/internal/github"
)

// singleSearchAPI serves one search item and counts requests, so tests can
// prove githubSource routes through LiveProvider's cache/singleflight
// instead of fetching on every call.
type singleSearchAPI struct {
	calls atomic.Int32
}

func (a *singleSearchAPI) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/search/issues", func(w http.ResponseWriter, _ *http.Request) {
		a.calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		item := map[string]any{
			"number":         7,
			"title":          "fix the thing",
			"state":          "open",
			"html_url":       "https://github.com/o/r/pull/7",
			"repository_url": "https://api.github.com/repos/o/r",
			"user":           map[string]any{"login": "hayden"},
			"labels":         []any{},
			"pull_request":   map[string]any{"html_url": "https://github.com/o/r/pull/7"},
			"updated_at":     "2026-07-18T10:00:00Z",
			"created_at":     "2026-07-17T00:00:00Z",
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"total_count": 1, "items": []any{item}})
	})
	mux.HandleFunc("/notifications", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	})
	return mux
}

// newLiveProviderFixture writes a minimal profiles config with one search
// source and constructs a real feed.LiveProvider against a fake GitHub API,
// mirroring feed's own poller_test.go fixture pattern.
func newLiveProviderFixture(t *testing.T, api *singleSearchAPI) *feed.LiveProvider {
	t.Helper()
	server := httptest.NewServer(api.handler())
	t.Cleanup(server.Close)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "profiles.yaml"), []byte(`sources:
  - id: my-prs
    kind: search
    query: "is:open is:pr author:@me archived:false"
profiles:
  - id: triage
    name: Triage
    feeds:
      - id: my-open-prs
        name: My open PRs
        sources: [my-prs]
`), 0o600))

	client := github.NewClient(github.WithAPIBase(server.URL))
	store := feed.NewStore(filepath.Join(dir, "profiles.yaml"), t.TempDir())
	return feed.NewLiveProvider(client, github.NewMemoryTokenStore("tok"), store, zerolog.Nop())
}

func TestGithubSource_Produce_EmitsWireItems(t *testing.T) {
	t.Parallel()

	api := &singleSearchAPI{}
	live := newLiveProviderFixture(t, api)

	src := &githubSource{live: live, def: feed.SourceDef{ID: "my-prs", Kind: "search", Query: "is:open is:pr author:@me"}}

	var emitted []Msg
	err := src.Produce(context.Background(), func(msg Msg) error {
		emitted = append(emitted, msg)
		return nil
	})
	require.NoError(t, err)
	require.Len(t, emitted, 1)

	msg := emitted[0]
	assert.Equal(t, "source:my-prs", msg.Topic)
	assert.Equal(t, "o/r#7", msg.Key)
	assert.Equal(t, "my-prs", msg.Meta["source"])
	assert.Equal(t, "PR", msg.Meta["kind"])
	assert.Equal(t, "o/r", msg.Meta["repo"])

	var item feed.Item
	require.NoError(t, json.Unmarshal(msg.Payload, &item))
	assert.Equal(t, "o/r#7", item.ID)
	assert.Equal(t, "fix the thing", item.Title)
}

// TestGithubSource_Produce_ReusesCoalescedFetch is the point of the seam:
// githubSource must not implement its own fetching. It should route through
// LiveProvider.SourceItems (cache + singleflight + conditional requests),
// so repeated Produce calls within the cache TTL cost no extra API request
// — the same guarantee feed.Poller / feed.LiveProvider.Refresh rely on.
func TestGithubSource_Produce_ReusesCoalescedFetch(t *testing.T) {
	t.Parallel()

	api := &singleSearchAPI{}
	live := newLiveProviderFixture(t, api)
	src := &githubSource{live: live, def: feed.SourceDef{ID: "my-prs", Kind: "search", Query: "is:open is:pr author:@me"}}

	for range 3 {
		err := src.Produce(context.Background(), func(Msg) error { return nil })
		require.NoError(t, err)
	}

	assert.Equal(t, int32(1), api.calls.Load(), "three Produce calls within the cache TTL should cost one API request")
}

func TestGithubSource_Produce_PropagatesFetchError(t *testing.T) {
	t.Parallel()

	// No token: LiveProvider.SourceItems fails with ErrNotAuthenticated
	// before ever hitting the network.
	live := feed.NewLiveProvider(github.NewClient(), github.NewMemoryTokenStore(""), feed.NewStore(filepath.Join(t.TempDir(), "profiles.yaml"), t.TempDir()), zerolog.Nop())
	src := &githubSource{live: live, def: feed.SourceDef{ID: "my-prs", Kind: "search", Query: "is:open"}}

	called := false
	err := src.Produce(context.Background(), func(Msg) error {
		called = true
		return nil
	})
	require.Error(t, err)
	assert.False(t, called, "no items should be emitted when the fetch fails")
}

func TestNewGithubSourceLister_ResolvesConfiguredSources(t *testing.T) {
	t.Parallel()

	api := &singleSearchAPI{}
	live := newLiveProviderFixture(t, api)

	lister := NewGithubSourceLister(live)
	sources, err := lister(context.Background())
	require.NoError(t, err)
	require.Contains(t, sources, "my-prs")

	_, ok := sources["my-prs"].(*githubSource)
	assert.True(t, ok, "the lister should build githubSource instances")
}

// TestProducer_WithGithubSource_AppendsAcrossTicks is an end-to-end slice of
// the producer path: a real feed.LiveProvider fetching from a fake GitHub
// API, through a real githubSource, into a real pipelinedb event log.
func TestProducer_WithGithubSource_AppendsAcrossTicks(t *testing.T) {
	t.Parallel()

	api := &singleSearchAPI{}
	live := newLiveProviderFixture(t, api)
	db := openTestPipelineDB(t)

	var appendedOffsets []int64
	producer := NewProducer(db, NewGithubSourceLister(live), 0, func(offset int64) {
		appendedOffsets = append(appendedOffsets, offset)
	}, zerolog.Nop())

	producer.Tick(context.Background())
	require.Len(t, appendedOffsets, 1)

	msgs, _, err := db.ReadFrom(context.Background(), 0, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "source:my-prs", msgs[0].Topic)
	assert.Equal(t, "o/r#7", msgs[0].Key)

	// A second tick with unchanged upstream data must not re-append (dedup)
	// even though githubSource re-emits the (cached) item every tick.
	producer.Tick(context.Background())
	msgs, _, err = db.ReadFrom(context.Background(), 0, 10)
	require.NoError(t, err)
	assert.Len(t, msgs, 1, "unchanged item across ticks is not re-appended")
	assert.Equal(t, int32(1), api.calls.Load(), "still one API request: the second tick's fetch was cache-served")
}
