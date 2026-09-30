package posthog

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/posthog/client"
)

func TestErrorsConfigValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		config  ErrorsConfig
		wantErr bool
	}{
		{name: "credential only", config: ErrorsConfig{Credential: "posthog/us.posthog.com-1"}},
		{name: "fully specified", config: ErrorsConfig{
			Credential: "posthog/us.posthog.com-1", Status: statusAll, OrderBy: orderOccurrences, DateFrom: "-24h", Limit: 50,
		}},
		{name: "zero config", config: ErrorsConfig{}, wantErr: true},
		{name: "wrong provider", config: ErrorsConfig{Credential: "grafana/host-1"}, wantErr: true},
		{name: "unknown status", config: ErrorsConfig{Credential: "posthog/us.posthog.com-1", Status: "burning"}, wantErr: true},
		{name: "unknown order", config: ErrorsConfig{Credential: "posthog/us.posthog.com-1", OrderBy: "vibes"}, wantErr: true},
		{name: "negative limit", config: ErrorsConfig{Credential: "posthog/us.posthog.com-1", Limit: -1}, wantErr: true},
		{name: "limit over the page cap", config: ErrorsConfig{Credential: "posthog/us.posthog.com-1", Limit: 101}, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.config.Validate()
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}

// The zero config must fetch something sensible rather than an empty query:
// PostHog rejects a blank status and would rank by its own default, not the
// recency a feed wants.
func TestErrorsConfigRequestDefaults(t *testing.T) {
	t.Parallel()

	req := (&ErrorsConfig{Credential: "posthog/us.posthog.com-1"}).request()
	assert.Equal(t, statusActive, req.Status)
	assert.Equal(t, orderLastSeen, req.OrderBy)
	assert.Equal(t, "DESC", req.OrderDirection)
	assert.Equal(t, defaultDateFrom, req.DateRange.DateFrom)
	assert.Equal(t, defaultLimit, req.Limit)
	assert.True(t, req.FilterTestAccounts, "internal traffic is excluded unless opted into")
}

// include_test_accounts is phrased as an opt-in, so it has to invert onto
// PostHog's filterTestAccounts — getting this backwards silently changes whose
// errors a feed reports.
func TestErrorsConfigIncludeTestAccountsInverts(t *testing.T) {
	t.Parallel()

	req := (&ErrorsConfig{Credential: "posthog/us.posthog.com-1", IncludeTestAccounts: true}).request()
	assert.False(t, req.FilterTestAccounts)
}

func issuesServer(t *testing.T, body string) (*httptest.Server, *string) {
	t.Helper()
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		assert.Equal(t, "/api/projects/42/error_tracking/query/issues/", r.URL.Path)
		_, _ = w.Write([]byte(body))
	}))
	return server, &gotBody
}

func TestErrorsProduceEmitsOnePerIssue(t *testing.T) {
	t.Parallel()

	server, gotBody := issuesServer(t, `{"results":[
		{"id":"11111111-1111-1111-1111-111111111111","name":"TypeError","description":"x is not a function","status":"active",
		 "first_seen":"2026-08-01T10:00:00Z","last_seen":"2026-08-03T09:30:00Z","library":"posthog-js",
		 "source":"app/checkout.tsx",
		 "aggregations":{"occurrences":1204,"users":37,"sessions":52}},
		{"id":"22222222-2222-2222-2222-222222222222","name":"","description":"boom","status":"resolved"}
	],"hasMore":false}`)
	defer server.Close()

	fx, _ := connectedFetcher(t, server.URL)
	config := &ErrorsConfig{Credential: "posthog/us.posthog.com-42", Limit: 10}
	src := &errorsSource{fetcher: fx, request: config.request(), topic: "source:flow/node"}

	var msgs []models.Msg
	require.NoError(t, src.Produce(t.Context(), func(m models.Msg) error {
		msgs = append(msgs, m)
		return nil
	}))

	require.Len(t, msgs, 2, "one message per issue")
	assert.Equal(t, "11111111-1111-1111-1111-111111111111", msgs[0].Key, "keyed by issue id, never by event")
	assert.Equal(t, SourceKind, msgs[0].SourceKind)
	assert.Equal(t, "source:flow/node", msgs[0].Topic)

	var first issuePayload
	require.NoError(t, json.Unmarshal(msgs[0].Payload, &first))
	assert.Equal(t, "TypeError: x is not a function", first.Title, "exception class alone collides across unrelated issues")
	assert.Equal(t, ItemKind, first.Kind, "an untyped item can only be targeted as the catch-all Item")
	assert.Equal(t, statusActive, first.State)

	// The body is the detail pane's only content; without it an item renders
	// as a bare title.
	assert.Contains(t, first.Body, "**Occurrences** 1204")
	assert.Contains(t, first.Body, "**Users** 37")
	assert.Contains(t, first.Body, "**Source** app/checkout.tsx", "the file is the most locating fact the list response carries")
	assert.Contains(t, first.Body, "**Library** posthog-js")
	assert.Contains(t, first.Body, "**Project** Acme")
	assert.InDelta(t, 1204, first.Occurrences, 0.001)
	assert.InDelta(t, 37, first.Users, 0.001)
	assert.Equal(t, "posthog-js", first.Library)
	assert.Equal(t, "Acme", first.Project)
	assert.Equal(t, server.URL+"/project/42/error_tracking/11111111-1111-1111-1111-111111111111", first.URL)

	// last_seen drives updatedAt so the item's recency is PostHog's, not the
	// poll clock's.
	assert.Equal(t, epochMillis("2026-08-03T09:30:00Z"), first.UpdatedAt)

	var second issuePayload
	require.NoError(t, json.Unmarshal(msgs[1].Payload, &second))
	assert.Equal(t, "boom", second.Title, "title falls back to the description")
	assert.Equal(t, statusResolved, second.State)

	// The request carries the node's scoping, not the endpoint's defaults.
	assert.Contains(t, *gotBody, `"status":"active"`)
	assert.Contains(t, *gotBody, `"orderBy":"last_seen"`)
	assert.Contains(t, *gotBody, `"limit":10`)
}

// PostHog's name is the exception class, so real projects carry many issues
// sharing one — the description is what tells them apart in a feed row.
func TestIssueTitlePairsNameAndDescription(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name, issueName, description, want string
	}{
		{"both", "TypeError", "Failed to fetch", "TypeError: Failed to fetch"},
		{"name only", "TypeError", "", "TypeError"},
		{"description only", "", "Failed to fetch", "Failed to fetch"},
		{"whitespace is not a value", "  ", "  ", "PostHog issue"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, issueTitle(client.Issue{Name: tt.issueName, Description: tt.description}))
		})
	}
}

// An issue with neither a name nor a description must not fall back to being
// keyed by its UUID in the feed, which reads as noise.
func TestIssueTitleNeverEmpty(t *testing.T) {
	t.Parallel()

	server, _ := issuesServer(t, `{"results":[{"id":"33333333-3333-3333-3333-333333333333","status":"active"}]}`)
	defer server.Close()

	fx, _ := connectedFetcher(t, server.URL)
	src := &errorsSource{fetcher: fx, request: (&ErrorsConfig{}).request(), topic: "t"}

	var msgs []models.Msg
	require.NoError(t, src.Produce(t.Context(), func(m models.Msg) error {
		msgs = append(msgs, m)
		return nil
	}))
	require.Len(t, msgs, 1)

	var payload issuePayload
	require.NoError(t, json.Unmarshal(msgs[0].Payload, &payload))
	assert.Equal(t, "PostHog issue", payload.Title)
}

// A fetch error must leave the previous snapshot in place rather than emitting
// an empty one, which the producer would read as "every issue is gone".
func TestErrorsProduceFailsWithoutEmitting(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	fx, _ := connectedFetcher(t, server.URL)
	src := &errorsSource{fetcher: fx, request: (&ErrorsConfig{}).request(), topic: "t"}

	emitted := 0
	err := src.Produce(t.Context(), func(models.Msg) error {
		emitted++
		return nil
	})
	require.Error(t, err)
	assert.Zero(t, emitted)
}

func observation(id, state, lastSeen string) models.Observation {
	payload, _ := json.Marshal(issuePayload{ID: id, Title: "TypeError", State: state, LastSeen: lastSeen})
	return models.Observation{ExternalID: id, Title: "TypeError", Payload: payload}
}

func TestErrorsClassifierNewIssue(t *testing.T) {
	t.Parallel()

	got := errorsClassifier{}.Classify(nil, observation("i1", statusActive, "2026-08-03T09:00:00Z"))
	assert.Equal(t, "new", got.Kind)
	assert.Equal(t, models.LifecycleActive, got.Lifecycle)
	assert.Equal(t, models.AttentionActivity, got.Attention)
}

func TestErrorsClassifierResolvedAndRegressed(t *testing.T) {
	t.Parallel()

	active := observation("i1", statusActive, "2026-08-03T09:00:00Z")
	resolved := observation("i1", statusResolved, "2026-08-03T09:00:00Z")

	closed := errorsClassifier{}.Classify(&active, resolved)
	assert.Equal(t, statusResolved, closed.Kind)
	assert.Equal(t, models.LifecycleTerminal, closed.Lifecycle)
	assert.Equal(t, models.TransitionEnteredTerminal, closed.Transition)
	assert.Equal(t, statusResolved, closed.ArchivedReason)
	assert.Equal(t, "Resolved", closed.Summary)

	// A regression is the signal the whole connector exists for.
	regressed := errorsClassifier{}.Classify(&resolved, active)
	assert.Equal(t, "regressed", regressed.Kind)
	assert.Equal(t, models.TransitionLeftTerminal, regressed.Transition)
	assert.Equal(t, models.AttentionActivity, regressed.Attention)
}

// suppressed is terminal for the same reason resolved is: the user has said
// they do not want to hear about it.
func TestErrorsClassifierSuppressedIsTerminal(t *testing.T) {
	t.Parallel()

	active := observation("i1", statusActive, "2026-08-03T09:00:00Z")
	suppressed := observation("i1", statusSuppressed, "2026-08-03T09:00:00Z")

	got := errorsClassifier{}.Classify(&active, suppressed)
	assert.Equal(t, models.LifecycleTerminal, got.Lifecycle)
	assert.Equal(t, models.TransitionEnteredTerminal, got.Transition)
}

// This is the roll-up guarantee: an issue that is still erroring reports a new
// occurrence only when its lastSeen advances, so a steady error does not
// re-notify on every tick.
func TestErrorsClassifierOccurrenceTracksLastSeen(t *testing.T) {
	t.Parallel()

	first := observation("i1", statusActive, "2026-08-03T09:00:00Z")
	same := observation("i1", statusActive, "2026-08-03T09:00:00Z")
	later := observation("i1", statusActive, "2026-08-03T09:30:00Z")

	unchanged := errorsClassifier{}.Classify(&first, same)
	assert.Equal(t, "updated", unchanged.Kind)
	assert.Equal(t, models.AttentionTrivial, unchanged.Attention, "a re-observed issue must not re-raise attention")

	advanced := errorsClassifier{}.Classify(&first, later)
	assert.Equal(t, "occurred", advanced.Kind)
	assert.Equal(t, models.AttentionActivity, advanced.Attention)
	assert.NotEqual(t, unchanged.OccurrenceKey, advanced.OccurrenceKey, "a fresh burst is a distinct occurrence")
}

// If PostHog ever sends a timestamp this connector cannot read, the occurrence
// key must degrade to "no new occurrence" rather than to a fresh one every
// tick, which would notify on every poll forever.
func TestOccurrenceStampFallsBackToObservedAt(t *testing.T) {
	t.Parallel()

	obs := models.Observation{ExternalID: "i1", ObservedAt: 1700, Payload: []byte(`{"id":"i1"}`)}
	assert.Equal(t, "1700", occurrenceStamp(obs))

	unparseable := models.Observation{ExternalID: "i1", ObservedAt: 1700, Payload: []byte(`{"lastSeen":"not-a-date"}`)}
	assert.Equal(t, "not-a-date", occurrenceStamp(unparseable),
		"an unparseable stamp is still stable, so it does not mint a new occurrence each poll")
}

func TestEpochMillis(t *testing.T) {
	t.Parallel()

	assert.NotZero(t, epochMillis("2026-08-03T09:30:00Z"))
	assert.NotZero(t, epochMillis("2026-08-03T09:30:00.123456Z"), "DRF sends fractional seconds")
	assert.Zero(t, epochMillis(""))
	assert.Zero(t, epochMillis("not-a-date"))
}

// An unrecognized status must not read as active: a value PostHog adds later
// should be visible rather than silently counted as a live issue.
func TestIssueStatePassesUnknownValuesThrough(t *testing.T) {
	t.Parallel()

	assert.Equal(t, statusActive, issueState(""))
	assert.Equal(t, statusResolved, issueState("Resolved"))
	assert.Equal(t, "pending_release", issueState("pending_release"))
	assert.False(t, isTerminalIssueState("pending_release"), "an unresolved legacy status is still live")
}
