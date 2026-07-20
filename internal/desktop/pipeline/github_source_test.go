package pipeline

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/desktop/feed"
	"github.com/colonyops/hive/internal/desktop/pipeline/flow"
	"github.com/colonyops/hive/internal/github"
)

// fakeFlows is an in-memory FlowLister for the source-lister tests.
type fakeFlows []flow.Flow

func (f fakeFlows) List() []flow.Flow { return f }

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

// newLiveProviderFixture constructs a real feed.LiveProvider against a fake
// GitHub API. Source config now lives in the flow's github-source nodes, not
// a profiles config, so the provider needs no store.
func newLiveProviderFixture(t *testing.T, api *singleSearchAPI) *feed.LiveProvider {
	t.Helper()
	server := httptest.NewServer(api.handler())
	t.Cleanup(server.Close)

	client := github.NewClient(github.WithAPIBase(server.URL))
	return feed.NewLiveProvider(client, github.NewMemoryTokenStore("tok"), zerolog.Nop())
}

func TestGithubSource_Produce_EmitsWireItems(t *testing.T) {
	t.Parallel()

	api := &singleSearchAPI{}
	live := newLiveProviderFixture(t, api)

	src := &githubSource{live: live, def: feed.SourceDef{ID: "triage/in-prs", Kind: "search", Query: "is:open is:pr author:@me"}, topic: "source:triage/in-prs"}

	var emitted []Msg
	err := src.Produce(context.Background(), func(msg Msg) error {
		emitted = append(emitted, msg)
		return nil
	})
	require.NoError(t, err)
	require.Len(t, emitted, 1)

	msg := emitted[0]
	assert.Equal(t, "source:triage/in-prs", msg.Topic)
	assert.Equal(t, "o/r#7", msg.Key)
	assert.Equal(t, "triage/in-prs", msg.Meta["source"])
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
	src := &githubSource{live: live, def: feed.SourceDef{ID: "triage/in-prs", Kind: "search", Query: "is:open is:pr author:@me"}, topic: "source:triage/in-prs"}

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
	live := feed.NewLiveProvider(github.NewClient(), github.NewMemoryTokenStore(""), zerolog.Nop())
	src := &githubSource{live: live, def: feed.SourceDef{ID: "triage/in-prs", Kind: "search", Query: "is:open"}, topic: "source:triage/in-prs"}

	called := false
	err := src.Produce(context.Background(), func(Msg) error {
		called = true
		return nil
	})
	require.Error(t, err)
	assert.False(t, called, "no items should be emitted when the fetch fails")
}

func TestNewFlowSourceLister_ResolvesEnabledSourceNodesAcrossFlows(t *testing.T) {
	t.Parallel()

	api := &singleSearchAPI{}
	live := newLiveProviderFixture(t, api)

	flows := fakeFlows{
		{
			ID:      "triage",
			Enabled: true,
			Nodes: []flow.Node{
				{ID: "in-prs", Type: "github-source", Config: &flow.GithubSourceConfig{Kind: "search", Query: "is:open"}},
				{ID: "off", Type: "github-source", Disabled: true, Config: &flow.GithubSourceConfig{Kind: "notifications"}},
				{ID: "sink", Type: "feed", Config: &flow.FeedConfig{}},
			},
		},
		// A disabled flow contributes no sources.
		{
			ID:      "paused",
			Enabled: false,
			Nodes:   []flow.Node{{ID: "in", Type: "github-source", Config: &flow.GithubSourceConfig{Kind: "notifications"}}},
		},
	}

	lister := NewFlowSourceLister(live, flows)
	sources, err := lister(context.Background())
	require.NoError(t, err)

	// Only the one enabled node in the one enabled flow, keyed flow-qualified.
	require.Len(t, sources, 1)
	require.Contains(t, sources, "triage/in-prs")
	gs, ok := sources["triage/in-prs"].(*githubSource)
	require.True(t, ok, "the lister should build githubSource instances")
	assert.Equal(t, "source:triage/in-prs", gs.topic)
}

// TestProducer_WithGithubSource_AppendsAcrossTicks is an end-to-end slice of
// the producer path: a real feed.LiveProvider fetching from a fake GitHub
// API, through a real githubSource, into a real pipelinedb event log.
func TestProducer_WithGithubSource_AppendsAcrossTicks(t *testing.T) {
	t.Parallel()

	api := &singleSearchAPI{}
	live := newLiveProviderFixture(t, api)
	db := openTestPipelineDB(t)

	flows := fakeFlows{{
		ID:      "triage",
		Enabled: true,
		Nodes: []flow.Node{
			{ID: "in-prs", Type: "github-source", Config: &flow.GithubSourceConfig{Kind: "search", Query: "is:open is:pr author:@me"}},
		},
	}}

	var appendedOffsets []int64
	producer := NewProducer(db, NewFlowSourceLister(live, flows), 0, func(offset int64) {
		appendedOffsets = append(appendedOffsets, offset)
	}, zerolog.Nop())

	producer.Tick(context.Background())
	require.Len(t, appendedOffsets, 1)

	msgs, _, err := db.ReadFrom(context.Background(), 0, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	assert.Equal(t, "source:triage/in-prs", msgs[0].Topic)
	assert.Equal(t, "o/r#7", msgs[0].Key)
	assert.Len(t, msgs[1].Snapshot, 1)

	// A second tick with unchanged upstream data must not re-append (dedup)
	// even though githubSource re-emits the (cached) item every tick.
	producer.Tick(context.Background())
	msgs, _, err = db.ReadFrom(context.Background(), 0, 10)
	require.NoError(t, err)
	assert.Len(t, msgs, 3, "unchanged items are deduplicated while every successful tick appends a snapshot")
	assert.Equal(t, int32(1), api.calls.Load(), "still one API request: the second tick's fetch was cache-served")
}
