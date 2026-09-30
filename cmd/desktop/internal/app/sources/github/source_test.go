package github_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/credentials"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
	"github.com/colonyops/hive/cmd/desktop/internal/app/ingest"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
	ghsource "github.com/colonyops/hive/cmd/desktop/internal/app/sources/github"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/github/feed"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/github/ghclient"
)

// testCredential is the account every fixture source fetches as. Sources
// carry a credential ref now, so a test source has to name one.
const testCredential = ghsource.Provider + "/octocat"

// newSource constructs the connector the way the resolver does — through its
// own factory — so these tests exercise the seam the producer uses rather
// than an internal struct.
func newSource(t *testing.T, fetchers *ghsource.Fetchers, kind, query string) connector.PullSource {
	t.Helper()
	instance, err := ghsource.NewFactory(fetchers).New(
		connector.Node{FlowID: "triage", NodeID: "in-prs"},
		&ghsource.Config{Credential: testCredential, Kind: kind, Query: query},
	)
	require.NoError(t, err)
	return instance.Pull
}

// sourceNode is one github source node, built the way the flow loader builds
// it so the resolver can read its connector config back out.
func sourceNode(id, kind, query string) flow.Node {
	return flow.Node{
		ID:     id,
		Type:   ghsource.Descriptor.Type,
		Config: flow.NewSourceConfig(ghsource.Descriptor.Type, &ghsource.Config{Credential: testCredential, Kind: kind, Query: query}),
	}
}

// newResolver wires the registry's instance half over the fake GitHub API.
func newResolver(fetchers *ghsource.Fetchers, flows fakeFlows) *ingest.Resolver {
	return ingest.NewResolver(flows, map[string]connector.Factory{
		ghsource.Descriptor.Type: ghsource.NewFactory(fetchers),
	}, zerolog.Nop())
}

// openTestPipelineDB opens a throwaway store on a temp dir, migrated and
// closed with the test.
func openTestPipelineDB(t *testing.T) *queries.DB {
	t.Helper()
	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

type notifierFunc func(offset int64)

func (f notifierFunc) PublishLogAppended(offset int64) { f(offset) }

func newTestProducer(db *queries.DB, sources ingest.Sources, interval time.Duration, onAppended func(int64), logger zerolog.Logger) *ingest.Producer {
	st := stores.New(db, stores.Options{})
	var notifier ingest.LogAppendNotifier
	if onAppended != nil {
		notifier = notifierFunc(onAppended)
	}
	return ingest.NewProducer(ingest.ProducerDeps{
		Ingester:  st.InboxItems,
		Snapshots: st.EventLog,
		Heads:     st.SourceHeads,
		Sources:   sources,
		Interval:  interval,
		Notifier:  notifier,
		Logger:    logger,
	})
}

func readFrom(ctx context.Context, db *queries.DB, offset int64, limit int) ([]models.Msg, int64, error) {
	return stores.New(db, stores.Options{}).EventLog.ReadFrom(ctx, offset, limit)
}

// fakeFlows is an in-memory FlowLister for the source-lister tests.
type fakeFlows []flow.Flow

func (f fakeFlows) List() []flow.Flow { return f }

// singleSearchAPI serves one search item and counts requests, so tests can
// prove githubSource routes through LiveProvider's cache/singleflight
// instead of fetching on every call.
type singleSearchAPI struct {
	calls   atomic.Int32
	aliases atomic.Int32
}

func (a *singleSearchAPI) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		a.calls.Add(1)
		var request struct {
			Query string `json:"query"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		aliases := strings.Count(request.Query, ": search(")
		a.aliases.Store(int32(aliases))
		w.Header().Set("Content-Type", "application/json")
		item := map[string]any{
			"__typename": "PullRequest",
			"number":     7,
			"title":      "fix the thing",
			"state":      "OPEN",
			"url":        "https://github.com/o/r/pull/7",
			"isDraft":    false,
			"author":     map[string]any{"login": "octocat"},
			"repository": map[string]any{"nameWithOwner": "o/r"},
			"labels":     map[string]any{"nodes": []any{}},
			"updatedAt":  "2026-07-18T10:00:00Z",
			"createdAt":  "2026-07-17T00:00:00Z",
		}
		data := make(map[string]any, aliases)
		for i := range aliases {
			data[fmt.Sprintf("s%d", i)] = map[string]any{"nodes": []any{item}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	})
	mux.HandleFunc("/notifications", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	})
	return mux
}

// newFetchersFixture builds the per-account fetcher registry against a fake
// GitHub API, with testCredential already connected.
func newFetchersFixture(t *testing.T, api *singleSearchAPI) *ghsource.Fetchers {
	t.Helper()
	server := httptest.NewServer(api.handler())
	t.Cleanup(server.Close)

	creds := credentials.NewMemoryStore()
	ref, err := credentials.ParseRef(testCredential)
	require.NoError(t, err)
	require.NoError(t, creds.Set(ref, "tok"))

	client := ghclient.NewClient(ghclient.WithAPIBase(server.URL))
	return ghsource.NewFetchers(client, creds, zerolog.Nop())
}

// newUnconnectedFetchers has no credential stored, so every fetch fails with
// ErrNotAuthenticated before reaching the network.
func newUnconnectedFetchers() *ghsource.Fetchers {
	return ghsource.NewFetchers(ghclient.NewClient(), credentials.NewMemoryStore(), zerolog.Nop())
}

func TestGithubSource_Produce_EmitsWireItems(t *testing.T) {
	t.Parallel()

	api := &singleSearchAPI{}
	fetchers := newFetchersFixture(t, api)

	src := newSource(t, fetchers, "search", "is:open is:pr author:@me")

	var emitted []ingest.Msg
	err := src.Produce(t.Context(), func(msg ingest.Msg) error {
		emitted = append(emitted, msg)
		return nil
	})
	require.NoError(t, err)
	require.Len(t, emitted, 1)

	msg := emitted[0]
	assert.Equal(t, "source:triage/in-prs", msg.Topic)
	assert.Equal(t, "o/r#7", msg.Key)

	var item feed.Item
	require.NoError(t, json.Unmarshal(msg.Payload, &item))
	assert.Equal(t, "o/r#7", item.ID)
	assert.Equal(t, "fix the thing", item.Title)
}

// TestGithubSource_Produce_ReusesCoalescedFetch is the point of the seam:
// githubSource must not implement its own fetching. It should route through
// LiveProvider.SourceItems (cache + singleflight + conditional requests),
// so repeated Produce calls within the cache TTL cost no extra API request.
func TestGithubSource_Produce_ReusesCoalescedFetch(t *testing.T) {
	t.Parallel()

	api := &singleSearchAPI{}
	fetchers := newFetchersFixture(t, api)
	src := newSource(t, fetchers, "search", "is:open is:pr author:@me")

	for range 3 {
		err := src.Produce(t.Context(), func(ingest.Msg) error { return nil })
		require.NoError(t, err)
	}

	assert.Equal(t, int32(1), api.calls.Load(), "three Produce calls within the cache TTL should cost one API request")
}

func TestGithubSource_Produce_PropagatesFetchError(t *testing.T) {
	t.Parallel()

	// No connected account: LiveProvider.SourceItems fails with
	// ErrNotAuthenticated before ever hitting the network.
	src := newSource(t, newUnconnectedFetchers(), "search", "is:open")

	called := false
	err := src.Produce(t.Context(), func(ingest.Msg) error {
		called = true
		return nil
	})
	require.Error(t, err)
	assert.False(t, called, "no items should be emitted when the fetch fails")
}

func TestResolver_ResolvesEnabledSourceNodesAcrossFlows(t *testing.T) {
	t.Parallel()

	api := &singleSearchAPI{}
	fetchers := newFetchersFixture(t, api)

	off := sourceNode("off", "notifications", "")
	off.Disabled = true

	flows := fakeFlows{
		{
			ID:      "triage",
			Enabled: true,
			Nodes: []flow.Node{
				sourceNode("in-prs", "search", "is:open"),
				off,
				{ID: "sink", Type: "feed", Config: &flow.FeedConfig{}},
			},
		},
		// A disabled flow contributes no sources.
		{
			ID:      "paused",
			Enabled: false,
			Nodes:   []flow.Node{sourceNode("in", "notifications", "")},
		},
	}

	instances := newResolver(fetchers, flows).PullInstances(t.Context())

	// Only the one enabled node in the one enabled flow, addressed
	// flow-qualified.
	require.Len(t, instances, 1)
	assert.Equal(t, "triage/in-prs", instances[0].Node.ID())
	assert.Equal(t, "source:triage/in-prs", instances[0].Node.Topic())
	assert.Equal(t, ghsource.Descriptor.Type, instances[0].Type)
	assert.NotNil(t, instances[0].Pull, "a pull connector must carry something to drain")
}

// TestProducer_PrefetchesSearchSourcesInOneBatch preserves per-source
// snapshots while issuing one aliased GraphQL request for all search nodes.
func TestProducer_PrefetchesSearchSourcesInOneBatch(t *testing.T) {
	t.Parallel()

	api := &singleSearchAPI{}
	fetchers := newFetchersFixture(t, api)
	db := openTestPipelineDB(t)
	flows := fakeFlows{
		{
			ID:      "triage",
			Enabled: true,
			Nodes: []flow.Node{
				sourceNode("issues", "search", "is:open is:issue"),
			},
		},
		{
			ID:      "reviews",
			Enabled: true,
			Nodes: []flow.Node{
				sourceNode("prs", "search", "is:open is:pr"),
			},
		},
	}
	producer := newTestProducer(db, newResolver(fetchers, flows), time.Hour, nil, zerolog.Nop())

	producer.Tick(t.Context())

	assert.Equal(t, int32(1), api.calls.Load())
	assert.Equal(t, int32(2), api.aliases.Load())
	msgs, _, err := readFrom(t.Context(), db, 0, 10)
	require.NoError(t, err)
	topics := make(map[string]bool)
	for _, msg := range msgs {
		topics[msg.Topic] = true
	}
	assert.True(t, topics["source:triage/issues"])
	assert.True(t, topics["source:reviews/prs"])
}

// Exercise declared capabilities through a real fetch and database. A broken
// handoff silently ingests GitHub items as generic and disables classification
// and absence confirmation, so a no-error assertion cannot catch it.
func TestProducer_WithGithubSource_IngestsAsGithubNotGeneric(t *testing.T) {
	t.Parallel()

	api := &singleSearchAPI{}
	fetchers := newFetchersFixture(t, api)
	db := openTestPipelineDB(t)

	flows := fakeFlows{{
		ID:      "triage",
		Enabled: true,
		Nodes: []flow.Node{
			sourceNode("in-prs", "search", "is:open is:pr"),
		},
	}}

	producer := newTestProducer(db, newResolver(fetchers, flows), time.Hour, nil, zerolog.Nop())
	producer.Tick(t.Context())

	// source_kind alone does not prove it: Produce stamps "github" on every
	// Msg, so it survives the fallback. profile_id is the one field only
	// IngestMetadata supplies -- generic ingestion uses the flow-qualified
	// source id instead of the flow id.
	var sourceKind, profileID string
	require.NoError(t, db.Conn().QueryRowContext(t.Context(),
		`SELECT source_kind, profile_id FROM inbox_item`).Scan(&sourceKind, &profileID))
	assert.Equal(t, "github", sourceKind)
	assert.Equal(t, "triage", profileID, "the instance's declared metadata did not reach the producer")

	// The GitHub classifier ran rather than the generic fallback. Both call a
	// first observation "observed", so assert on what only the GitHub one
	// produces: its fixed summary, and a lifecycle read out of the payload's
	// state rather than left unknown.
	var summary, lifecycle string
	require.NoError(t, db.Conn().QueryRowContext(t.Context(),
		`SELECT e.summary, i.lifecycle FROM inbox_event e JOIN inbox_item i ON i.id = e.item_id ORDER BY e.id LIMIT 1`).
		Scan(&summary, &lifecycle))
	assert.Equal(t, "Added to workspace", summary, "the generic classifier ran instead of the GitHub one")
	assert.Equal(t, "active", lifecycle, "the GitHub classifier did not read the payload state")
}

// TestProducer_WithGithubSource_AppendsAcrossTicks is an end-to-end slice of
// the producer path: a real feed.LiveProvider fetching from a fake GitHub
// API, through a real GitHub connector instance, into a real store event log.
func TestProducer_WithGithubSource_AppendsAcrossTicks(t *testing.T) {
	t.Parallel()

	api := &singleSearchAPI{}
	fetchers := newFetchersFixture(t, api)
	db := openTestPipelineDB(t)

	flows := fakeFlows{{
		ID:      "triage",
		Enabled: true,
		Nodes: []flow.Node{
			sourceNode("in-prs", "search", "is:open is:pr author:@me"),
		},
	}}

	var appendedOffsets []int64
	producer := newTestProducer(db, newResolver(fetchers, flows), 0, func(offset int64) {
		appendedOffsets = append(appendedOffsets, offset)
	}, zerolog.Nop())

	producer.Tick(t.Context())
	require.Len(t, appendedOffsets, 1)

	msgs, _, err := readFrom(t.Context(), db, 0, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	assert.Equal(t, "source:triage/in-prs", msgs[0].Topic)
	assert.Equal(t, "o/r#7", msgs[0].Key)
	assert.Len(t, msgs[1].Snapshot, 1)

	// A second tick with unchanged upstream data must not re-append (dedup)
	// even though githubSource re-emits the (cached) item every tick.
	producer.Tick(t.Context())
	msgs, _, err = readFrom(t.Context(), db, 0, 10)
	require.NoError(t, err)
	assert.Len(t, msgs, 3, "unchanged items are deduplicated while every successful tick appends a snapshot")
	assert.Equal(t, int32(1), api.calls.Load(), "still one API request: the second tick's fetch was cache-served")
}
