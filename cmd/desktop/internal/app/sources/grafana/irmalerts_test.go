package grafana

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/credentials"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/canonical"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/grafana/client"
)

func TestIRMAlertsConfigValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		config  IRMAlertsConfig
		wantErr bool
	}{
		{name: "credential alone is enough", config: IRMAlertsConfig{Credential: "grafana/host-1"}},
		{name: "scoped", config: IRMAlertsConfig{Credential: "grafana/host-1", Integration: "CFRPV98RPR1U8", Team: "T1"}},
		{name: "zero config", config: IRMAlertsConfig{}, wantErr: true},
		{name: "wrong provider", config: IRMAlertsConfig{Credential: "github/octocat"}, wantErr: true},
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

// irmServer stands in for a stack: the plugin settings call that names the
// OnCall host, and the alert groups listing on that same test server.
func irmServer(t *testing.T, groupsBody string) (*httptest.Server, *int) {
	t.Helper()
	settingsHits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/plugins/grafana-irm-app/settings" {
			settingsHits++
			_, _ = w.Write([]byte(`{"jsonData":{"onCallApiUrl":"` + "http://" + r.Host + `"}}`))
			return
		}
		_, _ = w.Write([]byte(groupsBody))
	}))
	t.Cleanup(server.Close)
	return server, &settingsHits
}

func TestIRMAlertsProduceEmitsOnePerAlertGroup(t *testing.T) {
	t.Parallel()

	server, _ := irmServer(t, `{"results":[
		{"id":"I1","state":"acknowledged","title":"Memory above 90%","integration_id":"CINT","team_id":"TSQ","alerts_count":6,
		 "created_at":"2026-08-01T12:00:00Z","acknowledged_at":"2026-08-01T12:04:00Z",
		 "permalinks":{"slack":"https://slack.example.com/thread"},
		 "labels":[{"key":{"name":"irm_priority"},"value":{"name":"high"}}],
		 "last_alert":{"id":"A1","alert_group_id":"I1","created_at":"2026-08-01T12:03:00Z","payload":{
		   "commonLabels":{"alertname":"MemoryHigh","cluster":"production-east","namespace":"payments","severity":"critical"},
		   "commonAnnotations":{"description":"Memory is above its safe threshold.","runbook_url":"https://runbooks.example.com/memory"},
		   "alerts":[{"labels":{"pod":"api-0"},"annotations":{"summary":"Memory high"}}]
		 }}},
		{"id":"I2","state":"new","title":"Disk full"}
	],"next":null}`)
	fx, _ := connectedFetcher(t, server.URL)

	src := &irmAlertsSource{fetcher: fx, topic: "source:flow/node"}

	var msgs []models.Msg
	require.NoError(t, src.Produce(t.Context(), func(m models.Msg) error {
		msgs = append(msgs, m)
		return nil
	}))

	require.Len(t, msgs, 2, "one message per alert group")
	assert.Equal(t, "I1", msgs[0].Key, "keyed by the IRM alert group id")
	assert.Equal(t, SourceKind, msgs[0].SourceKind)

	var first irmAlertPayload
	require.NoError(t, json.Unmarshal(msgs[0].Payload, &first))
	assert.Equal(t, "Memory above 90%", first.Title)
	assert.Equal(t, stateAcknowledged, first.State)
	assert.Equal(t, "https://slack.example.com/thread", first.URL)
	assert.Equal(t, "critical", first.Severity, "severity is lifted out of the source labels")
	assert.Equal(t, "production-east", first.Cluster)
	assert.Equal(t, "payments", first.Namespace)
	assert.Equal(t, "CINT", first.Integration)
	assert.Equal(t, "TSQ", first.Team)
	assert.Equal(t, 6, first.AlertsCount)
	assert.Equal(t, "2026-08-01T12:04:00Z", first.AcknowledgedAt)
	assert.Equal(t, ItemKind, first.Kind, "an IRM group is an Alert, like its Alertmanager sibling")
	assert.Equal(t, []string{
		"alertname=MemoryHigh",
		"cluster=production-east",
		"irm_priority=high",
		"namespace=payments",
		"severity=critical",
	}, first.Labels, "source labels augment IRM's own labels as sorted canonical tags")
	assert.Equal(t, "production-east", first.AlertLabels["cluster"], "source labels survive as provider enrichment")
	assert.Equal(t, "Memory is above its safe threshold.", first.Annotations["description"])
	assert.Contains(t, first.Body, "Memory is above its safe threshold.")
	assert.Contains(t, first.Body, `cluster = "production-east"`)
	assert.Contains(t, first.Body, `namespace = "payments"`)
	assert.NotContains(t, first.Body, "pod", "a non-common instance label must not describe the whole group")
	assert.Contains(t, first.Body, "**Alerts in group** 6")
	assert.Contains(t, first.Body, "**Firing since** 2026-08-01T12:00:00Z")
	assert.Contains(t, first.Body, "**Acknowledged** 2026-08-01T12:04:00Z")

	var second irmAlertPayload
	require.NoError(t, json.Unmarshal(msgs[1].Payload, &second))
	assert.Equal(t, stateFiring, second.State, "upstream's 'new' is emitted as 'firing', matching the sibling node")
	assert.Empty(t, second.URL, "a group with no permalinks carries no url")
	assert.Empty(t, second.Body, "a group with nothing to say gets no body, not an empty bullet list")
}

// A group with no title would otherwise reach the feed as a blank row.
func TestLatestAlertDetailsFallsBackForSingleAlertPayloads(t *testing.T) {
	t.Parallel()

	details := latestAlertDetails(&client.IRMAlert{Payload: json.RawMessage(`{
		"alerts":[{"labels":{"cluster":"staging"},"annotations":{"description":"Pod is restarting."}}]
	}`)})

	assert.Equal(t, map[string]string{"cluster": "staging"}, details.Labels)
	assert.Equal(t, "Pod is restarting.", details.Annotations["description"])
}

func TestLatestAlertDetailsIgnoresUnknownPayloads(t *testing.T) {
	t.Parallel()

	assert.Empty(t, latestAlertDetails(nil))
	assert.Empty(t, latestAlertDetails(&client.IRMAlert{Payload: json.RawMessage(`"plain text"`)}))
}

func TestIRMAlertsProduceFallsBackToATitle(t *testing.T) {
	t.Parallel()

	server, _ := irmServer(t, `{"results":[{"id":"I1","state":"new","title":"   "}],"next":null}`)
	fx, _ := connectedFetcher(t, server.URL)

	var msgs []models.Msg
	require.NoError(t, (&irmAlertsSource{fetcher: fx, topic: "t"}).Produce(t.Context(), func(m models.Msg) error {
		msgs = append(msgs, m)
		return nil
	}))

	require.Len(t, msgs, 1)
	var payload irmAlertPayload
	require.NoError(t, json.Unmarshal(msgs[0].Payload, &payload))
	assert.Equal(t, "Grafana IRM alert", payload.Title)
}

// The OnCall host is plugin configuration, not per-poll data. Resolving it on
// every tick would double this source's request count for a value that only
// changes when the stack does.
func TestIRMAlertsResolvesTheOnCallURLOnce(t *testing.T) {
	t.Parallel()

	server, settingsHits := irmServer(t, `{"results":[],"next":null}`)
	fx, _ := connectedFetcher(t, server.URL)

	for range 3 {
		_, err := fx.AlertGroups(t.Context(), client.AlertGroupQuery{})
		require.NoError(t, err)
	}
	assert.Equal(t, 1, *settingsHits, "the OnCall URL is cached per stack")

	// Reconnecting may point the account at a different stack, so the cached
	// host must not survive an invalidation.
	fx.invalidate()
	_, err := fx.AlertGroups(t.Context(), client.AlertGroupQuery{})
	require.NoError(t, err)
	assert.Equal(t, 2, *settingsHits, "invalidating drops the cached OnCall URL")
}

func TestIRMAlertsClassifierAcknowledgeIsActivity(t *testing.T) {
	t.Parallel()

	firing := models.Observation{ExternalID: "I1", Title: "Memory", Payload: []byte(`{"state":"firing"}`)}
	acked := models.Observation{ExternalID: "I1", Title: "Memory", Payload: []byte(`{"state":"acknowledged"}`)}

	first := irmAlertsClassifier{}.Classify(nil, firing)
	assert.Equal(t, stateFiring, first.Kind)
	assert.Equal(t, models.LifecycleActive, first.Lifecycle)

	// The whole point of this connector: someone picking an alert up is a real
	// event, not the trivial re-observation the Alertmanager source reports.
	transition := irmAlertsClassifier{}.Classify(&firing, acked)
	assert.Equal(t, stateAcknowledged, transition.Kind)
	assert.Equal(t, "Acknowledged", transition.Summary)
	assert.Equal(t, models.AttentionActivity, transition.Attention)
	assert.Equal(t, models.LifecycleActive, transition.Lifecycle, "an acknowledged alert is still live")
	assert.Equal(t, models.TransitionNone, transition.Transition)
}

func TestIRMAlertsClassifierResolvedIsTerminal(t *testing.T) {
	t.Parallel()

	acked := models.Observation{ExternalID: "I1", Title: "Memory", Payload: []byte(`{"state":"acknowledged"}`)}
	resolved := models.Observation{ExternalID: "I1", Title: "Memory", Payload: []byte(`{"state":"resolved"}`)}

	closed := irmAlertsClassifier{}.Classify(&acked, resolved)
	assert.Equal(t, stateResolved, closed.Kind)
	assert.Equal(t, models.LifecycleTerminal, closed.Lifecycle)
	assert.Equal(t, models.TransitionEnteredTerminal, closed.Transition)
	assert.Equal(t, stateResolved, closed.ArchivedReason)

	reopened := irmAlertsClassifier{}.Classify(&resolved, acked)
	assert.Equal(t, models.TransitionLeftTerminal, reopened.Transition)
	assert.Equal(t, "Firing again", reopened.Summary)
}

// A group re-observed at an unchanged state must stay trivial, or an
// acknowledged alert would re-raise attention on every poll for its lifetime.
func TestIRMAlertsClassifierReobservedStaysTrivial(t *testing.T) {
	t.Parallel()

	acked := models.Observation{ExternalID: "I1", Title: "Memory", Payload: []byte(`{"state":"acknowledged"}`)}

	same := irmAlertsClassifier{}.Classify(&acked, acked)
	assert.Equal(t, "updated", same.Kind)
	assert.Equal(t, models.AttentionTrivial, same.Attention)
	assert.Equal(t, models.TransitionNone, same.Transition)
}

func TestIRMAlertsAbsenceMarksResolvedAndTerminal(t *testing.T) {
	t.Parallel()

	previous := []models.Observation{{
		ExternalID: "I1",
		Title:      "Memory",
		Payload:    []byte(`{"title":"Memory","kind":"Alert","state":"acknowledged","labels":["severity=critical"]}`),
	}}

	verdicts, err := irmAlertsAbsence{}.ConfirmAbsence(t.Context(), previous)
	require.NoError(t, err)

	verdict, ok := verdicts["I1"]
	require.True(t, ok, "an absent alert group gets a verdict")
	assert.True(t, verdict.Terminal, "the listing is the complete active set, so absence is authoritative")
	require.NotNil(t, verdict.Current)
	assert.Equal(t, stateResolved, canonical.State(verdict.Current.Payload))

	var payload irmAlertPayload
	require.NoError(t, json.Unmarshal(verdict.Current.Payload, &payload))
	assert.Equal(t, "Memory", payload.Title, "the archived item keeps its identity")
	assert.Equal(t, []string{"severity=critical"}, payload.Labels)
}

// The factory is where a node's scope becomes the query, so an unscoped node
// must not send a query that matches nothing.
func TestIRMAlertsFactoryBuildsTheQueryFromConfig(t *testing.T) {
	t.Parallel()

	fetchers := NewFetchers(NewStackStore(filepath.Join(t.TempDir(), "stacks.json")), credentials.NewMemoryStore(), zerolog.Nop())
	factory := NewIRMAlertsFactory(fetchers)

	instance, err := factory.New(
		connector.Node{FlowID: "f", NodeID: "n"},
		&IRMAlertsConfig{Credential: "grafana/host-1", Integration: "  CINT  ", Team: ""},
	)
	require.NoError(t, err)

	src, ok := instance.Pull.(*irmAlertsSource)
	require.True(t, ok)
	assert.Equal(t, "CINT", src.query.IntegrationID, "a pasted id keeps its surrounding whitespace out of the query")
	assert.Empty(t, src.query.TeamID)
	assert.Equal(t, activeAlertGroupStates, src.query.States, "resolved groups are settled by absence, not fetched")
}
