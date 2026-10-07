package webhook

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
)

// webhookInstance builds one live instance the way the resolver does — through
// the connector's own factory — so these tests exercise the metadata and
// classifier a delivery actually gets rather than a hand-rolled stand-in.
func webhookInstance(t *testing.T, flowID, nodeID, path, secret string) connector.Instance {
	t.Helper()
	instance, err := NewFactory().New(
		connector.Node{FlowID: flowID, NodeID: nodeID},
		&Config{Path: path, Secret: secret},
	)
	require.NoError(t, err)
	return instance
}

// fakeInstances is the listener's Instances seam: the push-mode instances the
// registry would resolve from the current flow set.
func fakeInstances(instances ...connector.Instance) Instances {
	return func(context.Context) []connector.Instance { return instances }
}

type notifierFunc func(offset int64)

func (f notifierFunc) PublishLogAppended(offset int64) { f(offset) }

func newWebhookTestListener(t *testing.T, instances Instances) (*Listener, *queries.DB, *int64) {
	t.Helper()
	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	st := stores.New(db, stores.Options{})

	var lastOffset int64
	listener := NewListener(st.InboxItems, st.EventLog, st.WebhookCaptures, st.InboxItems, instances, "127.0.0.1", 0, notifierFunc(func(offset int64) { lastOffset = offset }), zerolog.Nop())
	return listener, db, &lastOffset
}

func readForConsumer(db *queries.DB, ctx context.Context, consumer string, limit int) ([]models.Msg, error) {
	return stores.New(db, stores.Options{}).EventLog.ReadForConsumer(ctx, consumer, limit, models.RoutedTopics{All: true})
}

func postHook(t *testing.T, handler http.Handler, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestWebhookListenerIngestsDelivery(t *testing.T) {
	listener, db, lastOffset := newWebhookTestListener(t, fakeInstances(webhookInstance(t, "triage", "hook", "ci-alerts", "")))
	handler := listener.Handler()

	body := `{"id":"build-42","title":"Build failed","url":"https://ci.example/42","status":"red"}`
	rec := postHook(t, handler, "/hooks/ci-alerts", body, nil)
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"delivered":1}`, rec.Body.String())

	ctx := t.Context()
	item, err := db.GetInboxItemByExternalID(ctx, queries.GetInboxItemByExternalIDParams{
		ProfileID: "triage", SourceKind: SourceKind, SourceScope: "hook", ExternalID: "build-42",
	})
	require.NoError(t, err)
	assert.Equal(t, "Build failed", item.Title)
	assert.Equal(t, "https://ci.example/42", item.Url)
	assert.JSONEq(t, body, string(item.Payload))

	// One routed observation row plus one authoritative snapshot row.
	msgs, err := readForConsumer(db, ctx, "triage", 10)
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	assert.Equal(t, "source:triage/hook", msgs[0].Topic)
	assert.Equal(t, "build-42", msgs[0].Key)
	require.Len(t, msgs[1].Snapshot, 1)
	assert.Equal(t, "build-42", msgs[1].Snapshot[0].Key)

	// The wake-up carries the snapshot row's offset (the last append).
	assert.Equal(t, msgs[1].ID, fmt.Sprint(*lastOffset))

	capture, err := stores.New(db, stores.Options{}).WebhookCaptures.Get(ctx, "source:triage/hook")
	require.NoError(t, err)
	assert.JSONEq(t, body, string(capture.Body))
	assert.Positive(t, capture.ReceivedAt)
}

func TestMountAPIServesAlongsideHooks(t *testing.T) {
	listener, _, _ := newWebhookTestListener(t, fakeInstances(webhookInstance(t, "triage", "hook", "ci", "")))
	listener.MountAPI("/api/", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	handler := listener.Handler()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/anything", nil))
	assert.Equal(t, http.StatusTeapot, rec.Code, "the mounted handler serves /api/")

	assert.Equal(t, http.StatusAccepted, postHook(t, handler, "/hooks/ci", `{"id":"x"}`, nil).Code,
		"mounting the API does not disturb /hooks/")
}

func TestMountAPIServesMultipleHandlers(t *testing.T) {
	listener, _, _ := newWebhookTestListener(t, fakeInstances(webhookInstance(t, "triage", "hook", "ci", "")))
	listener.MountAPI("/api/", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	listener.MountAPI("/debug/pprof/", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	handler := listener.Handler()

	for path, want := range map[string]int{
		"/api/anything":     http.StatusTeapot,
		"/debug/pprof/heap": http.StatusOK,
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		assert.Equal(t, want, rec.Code, "%s routes to its mounted handler", path)
	}

	assert.Equal(t, http.StatusAccepted, postHook(t, handler, "/hooks/ci", `{"id":"x"}`, nil).Code,
		"the extra mounts do not disturb /hooks/")
}

func TestWebhookListenerDeduplicatesUnchangedBody(t *testing.T) {
	listener, db, _ := newWebhookTestListener(t, fakeInstances(webhookInstance(t, "triage", "hook", "ci", "")))
	handler := listener.Handler()
	body := `{"id":"x","title":"same"}`

	captures := stores.New(db, stores.Options{}).WebhookCaptures
	require.Equal(t, http.StatusAccepted, postHook(t, handler, "/hooks/ci", body, nil).Code)
	first, err := captures.Get(t.Context(), "source:triage/hook")
	require.NoError(t, err)

	require.Equal(t, http.StatusAccepted, postHook(t, handler, "/hooks/ci", body, nil).Code)

	ctx := t.Context()
	msgs, err := readForConsumer(db, ctx, "triage", 10)
	require.NoError(t, err)
	assert.Len(t, msgs, 2, "an unchanged re-delivery must not append new event rows")

	// The capture still tracks the latest request.
	second, err := captures.Get(ctx, "source:triage/hook")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, second.ReceivedAt, first.ReceivedAt)
}

func TestWebhookListenerUpdatesChangedBodySameID(t *testing.T) {
	listener, db, _ := newWebhookTestListener(t, fakeInstances(webhookInstance(t, "triage", "hook", "ci", "")))
	handler := listener.Handler()

	require.Equal(t, http.StatusAccepted, postHook(t, handler, "/hooks/ci", `{"id":"x","n":1}`, nil).Code)
	require.Equal(t, http.StatusAccepted, postHook(t, handler, "/hooks/ci", `{"id":"x","n":2}`, nil).Code)

	ctx := t.Context()
	item, err := db.GetInboxItemByExternalID(ctx, queries.GetInboxItemByExternalIDParams{
		ProfileID: "triage", SourceKind: SourceKind, SourceScope: "hook", ExternalID: "x",
	})
	require.NoError(t, err)
	assert.Equal(t, int64(2), item.Revision)
	assert.JSONEq(t, `{"id":"x","n":2}`, string(item.Payload))

	// Snapshot still contains exactly one item for the key.
	msgs, err := readForConsumer(db, ctx, "triage", 10)
	require.NoError(t, err)
	require.Len(t, msgs, 4)
	assert.Len(t, msgs[3].Snapshot, 1)
}

func TestWebhookListenerContentHashKeyWithoutID(t *testing.T) {
	listener, db, _ := newWebhookTestListener(t, fakeInstances(webhookInstance(t, "triage", "hook", "ci", "")))
	handler := listener.Handler()

	require.Equal(t, http.StatusAccepted, postHook(t, handler, "/hooks/ci", `{"event":"a"}`, nil).Code)
	require.Equal(t, http.StatusAccepted, postHook(t, handler, "/hooks/ci", `{"event":"b"}`, nil).Code)

	ctx := t.Context()
	rows, err := stores.New(db, stores.Options{}).InboxItems.ListUnarchivedBySource(ctx, "triage", SourceKind, "hook")
	require.NoError(t, err)
	require.Len(t, rows, 2, "distinct bodies without ids are distinct items")
	assert.Len(t, rows[0].ExternalID, 64, "content-hash key is the sha256 hex")
	assert.True(t, strings.HasPrefix(rows[0].Title, "ci · "), "fallback title includes the path: %q", rows[0].Title)
}

func TestWebhookListenerNumericID(t *testing.T) {
	listener, db, _ := newWebhookTestListener(t, fakeInstances(webhookInstance(t, "triage", "hook", "ci", "")))
	require.Equal(t, http.StatusAccepted, postHook(t, listener.Handler(), "/hooks/ci", `{"id":1234}`, nil).Code)

	_, err := db.GetInboxItemByExternalID(t.Context(), queries.GetInboxItemByExternalIDParams{
		ProfileID: "triage", SourceKind: SourceKind, SourceScope: "hook", ExternalID: "1234",
	})
	require.NoError(t, err)
}

func TestWebhookListenerSecret(t *testing.T) {
	listener, _, _ := newWebhookTestListener(t, fakeInstances(webhookInstance(t, "triage", "hook", "ci", "s3cret")))
	handler := listener.Handler()

	assert.Equal(t, http.StatusUnauthorized, postHook(t, handler, "/hooks/ci", `{}`, nil).Code)
	assert.Equal(t, http.StatusUnauthorized, postHook(t, handler, "/hooks/ci", `{}`, map[string]string{SecretHeader: "wrong"}).Code)
	assert.Equal(t, http.StatusAccepted, postHook(t, handler, "/hooks/ci", `{}`, map[string]string{SecretHeader: "s3cret"}).Code)
}

func TestWebhookListenerFansOutToMatchingNodes(t *testing.T) {
	flows := fakeInstances(
		webhookInstance(t, "alpha", "hook-a", "shared", ""),
		webhookInstance(t, "beta", "hook-b", "shared", "s3cret"),
	)
	listener, db, _ := newWebhookTestListener(t, flows)
	handler := listener.Handler()

	// Without the secret only alpha's node accepts; with it both do.
	rec := postHook(t, handler, "/hooks/shared", `{"id":"1"}`, nil)
	require.Equal(t, http.StatusAccepted, rec.Code)
	assert.JSONEq(t, `{"delivered":1}`, rec.Body.String())

	rec = postHook(t, handler, "/hooks/shared", `{"id":"2"}`, map[string]string{SecretHeader: "s3cret"})
	require.Equal(t, http.StatusAccepted, rec.Code)
	assert.JSONEq(t, `{"delivered":2}`, rec.Body.String())

	ctx := t.Context()
	_, err := db.GetInboxItemByExternalID(ctx, queries.GetInboxItemByExternalIDParams{
		ProfileID: "alpha", SourceKind: SourceKind, SourceScope: "hook-a", ExternalID: "2",
	})
	require.NoError(t, err)
	_, err = db.GetInboxItemByExternalID(ctx, queries.GetInboxItemByExternalIDParams{
		ProfileID: "beta", SourceKind: SourceKind, SourceScope: "hook-b", ExternalID: "2",
	})
	require.NoError(t, err)
}

// Disabled flows and disabled nodes are not tested here any more: the
// listener is handed the instances that already exist, and deciding which
// nodes are live is the resolver's job — see
// TestResolverSkipsDisabledFlowsAndNodes in cmd/desktop/internal/app/ingest.
func TestWebhookListenerRejections(t *testing.T) {
	listener, _, _ := newWebhookTestListener(t, fakeInstances(webhookInstance(t, "triage", "hook", "ci", "")))
	handler := listener.Handler()

	assert.Equal(t, http.StatusNotFound, postHook(t, handler, "/hooks/nope", `{}`, nil).Code)
	assert.Equal(t, http.StatusBadRequest, postHook(t, handler, "/hooks/ci", `{not json`, nil).Code)
	assert.Equal(t, http.StatusRequestEntityTooLarge, postHook(t, handler, "/hooks/ci", `{"pad":"`+strings.Repeat("x", maxBodyBytes)+`"}`, nil).Code)

	req := httptest.NewRequest(http.MethodGet, "/hooks/ci", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	assert.Equal(t, http.MethodPost, rec.Header().Get("Allow"))
}

func TestWebhookListenerStartStop(t *testing.T) {
	listener, _, _ := newWebhookTestListener(t, fakeInstances(webhookInstance(t, "triage", "hook", "ci", "")))
	require.NoError(t, listener.Start(t.Context()))
	defer func() { require.NoError(t, listener.Stop(t.Context())) }()

	require.True(t, listener.Running())
	require.Positive(t, listener.Port())

	resp, err := http.Post(fmt.Sprintf("http://127.0.0.1:%d/hooks/ci", listener.Port()), "application/json", strings.NewReader(`{"id":"live"}`))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	payload, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, resp.StatusCode, string(payload))
}

func TestWebhookListenerStopWithoutStartIsNoop(t *testing.T) {
	listener, _, _ := newWebhookTestListener(t, fakeInstances())
	require.NoError(t, listener.Stop(t.Context()))
}

func TestWebhookListenerStopIsIdempotent(t *testing.T) {
	listener, _, _ := newWebhookTestListener(t, fakeInstances())
	require.NoError(t, listener.Start(t.Context()))

	require.NoError(t, listener.Stop(t.Context()))
	// A second Stop -- concurrent teardown paths calling it more than once,
	// or a caller that stops twice defensively -- must not panic or block.
	require.NoError(t, listener.Stop(t.Context()))
}

func TestWebhookListenerStopIsConcurrencySafe(t *testing.T) {
	listener, _, _ := newWebhookTestListener(t, fakeInstances())
	require.NoError(t, listener.Start(t.Context()))

	var wg sync.WaitGroup
	errs := make([]error, 10)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = listener.Stop(t.Context())
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		assert.NoError(t, err)
	}
}

func TestMissingFeedItemFields(t *testing.T) {
	assert.Empty(t, MissingFeedItemFields([]byte(`{"id":"1","kind":"PR","repo":"o/r","title":"t","url":"https://x"}`)))
	assert.Equal(t, []string{"kind", "repo"}, MissingFeedItemFields([]byte(`{"id":"1","kind":7,"title":"t","url":"https://x"}`)))
	assert.Equal(t, []string{"id", "kind", "repo", "title", "url"}, MissingFeedItemFields([]byte(`[1,2,3]`)))
}

// --- Listener-level tests: real SQLite through IngestObservation + applyTransition. ---

func TestWebhookListenerRedeliveryEntersTerminalArchivesItem(t *testing.T) {
	listener, db, _ := newWebhookTestListener(t, fakeInstances(webhookInstance(t, "triage", "hook", "ci", "")))
	handler := listener.Handler()

	require.Equal(t, http.StatusAccepted, postHook(t, handler, "/hooks/ci", `{"id":"x","title":"t","state":"open"}`, nil).Code)
	time.Sleep(2 * time.Millisecond) // distinct ObservedAt so the two deliveries get distinct occurrence keys
	require.Equal(t, http.StatusAccepted, postHook(t, handler, "/hooks/ci", `{"id":"x","title":"t","state":"Resolved"}`, nil).Code)

	ctx := t.Context()
	item, err := db.GetInboxItemByExternalID(ctx, queries.GetInboxItemByExternalIDParams{
		ProfileID: "triage", SourceKind: SourceKind, SourceScope: "hook", ExternalID: "x",
	})
	require.NoError(t, err)
	assert.True(t, item.ArchivedAt.Valid)
	assert.Equal(t, "system", item.ArchivedActor.String)
	assert.Equal(t, "resolved", item.ArchivedReason.String)
	assert.Equal(t, "resolved", item.SourceState.String)
	assert.Equal(t, "terminal", item.Lifecycle)

	events, err := stores.New(db, stores.Options{}).InboxItems.Events(ctx, item.ID, 10)
	require.NoError(t, err)
	require.NotEmpty(t, events)
	assert.Equal(t, "resolved", events[0].Kind, "latest event is first (ORDER BY id DESC)")
}

func TestWebhookListenerRedeliveryLeavesTerminalResurfaces(t *testing.T) {
	listener, db, _ := newWebhookTestListener(t, fakeInstances(webhookInstance(t, "triage", "hook", "ci", "")))
	handler := listener.Handler()

	require.Equal(t, http.StatusAccepted, postHook(t, handler, "/hooks/ci", `{"id":"x","title":"t","state":"open"}`, nil).Code)
	time.Sleep(2 * time.Millisecond) // distinct ObservedAt so each delivery gets a distinct occurrence key
	require.Equal(t, http.StatusAccepted, postHook(t, handler, "/hooks/ci", `{"id":"x","title":"t","state":"closed"}`, nil).Code)
	time.Sleep(2 * time.Millisecond)
	require.Equal(t, http.StatusAccepted, postHook(t, handler, "/hooks/ci", `{"id":"x","title":"t","state":"open"}`, nil).Code)

	ctx := t.Context()
	item, err := db.GetInboxItemByExternalID(ctx, queries.GetInboxItemByExternalIDParams{
		ProfileID: "triage", SourceKind: SourceKind, SourceScope: "hook", ExternalID: "x",
	})
	require.NoError(t, err)
	assert.False(t, item.ArchivedAt.Valid, "reopen resurfaces the item")
	assert.NotEqual(t, int64(0), item.Unread)
	assert.Equal(t, "active", item.Lifecycle)
	assert.Equal(t, "open", item.SourceState.String)

	events, err := stores.New(db, stores.Options{}).InboxItems.Events(ctx, item.ID, 10)
	require.NoError(t, err)
	require.NotEmpty(t, events)
	assert.Equal(t, "reopened", events[0].Kind)
}

func TestWebhookListenerFirstDeliveryTerminalNotArchived(t *testing.T) {
	listener, db, _ := newWebhookTestListener(t, fakeInstances(webhookInstance(t, "triage", "hook", "ci", "")))
	handler := listener.Handler()

	require.Equal(t, http.StatusAccepted, postHook(t, handler, "/hooks/ci", `{"id":"x","title":"t","state":"done"}`, nil).Code)

	ctx := t.Context()
	item, err := db.GetInboxItemByExternalID(ctx, queries.GetInboxItemByExternalIDParams{
		ProfileID: "triage", SourceKind: SourceKind, SourceScope: "hook", ExternalID: "x",
	})
	require.NoError(t, err)
	assert.Equal(t, "terminal", item.Lifecycle)
	assert.False(t, item.ArchivedAt.Valid, "terminal on arrival is not auto-archived")
}
