package grafana

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/canonical"
)

func TestAlertsConfigValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		config  AlertsConfig
		wantErr bool
	}{
		{name: "valid", config: AlertsConfig{Credential: "grafana/host-1"}},
		{name: "zero config", config: AlertsConfig{}, wantErr: true},
		{name: "wrong provider", config: AlertsConfig{Credential: "github/octocat"}, wantErr: true},
		{
			name:   "every matcher operator",
			config: AlertsConfig{Credential: "grafana/host-1", Matchers: []string{"a=b", "c!=d", "e=~f|g", "h!~i"}},
		},
		{
			// The value owns everything after the leftmost operator, including
			// characters that are operators themselves.
			name:   "an operator inside the value",
			config: AlertsConfig{Credential: "grafana/host-1", Matchers: []string{"path=/a!=b"}},
		},
		{
			name:   "an empty value is a matcher for the empty string",
			config: AlertsConfig{Credential: "grafana/host-1", Matchers: []string{"squad="}},
		},
		{
			name:    "no operator",
			config:  AlertsConfig{Credential: "grafana/host-1", Matchers: []string{"squad"}},
			wantErr: true,
		},
		{
			name:    "no label name",
			config:  AlertsConfig{Credential: "grafana/host-1", Matchers: []string{"=platform"}},
			wantErr: true,
		},
		{
			name:    "blank matcher",
			config:  AlertsConfig{Credential: "grafana/host-1", Matchers: []string{"squad=platform", "  "}},
			wantErr: true,
		},
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

func alertsServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
}

func TestAlertsProduceEmitsOnePerFiringAlert(t *testing.T) {
	t.Parallel()

	server := alertsServer(t, `[
		{"fingerprint":"abc","labels":{"alertname":"HighLatency"},"annotations":{"summary":"p99 over 1s"},"status":{"state":"active"}},
		{"fingerprint":"def","labels":{"alertname":"DiskFull"},"annotations":{},"status":{"state":"active"}}
	]`)
	defer server.Close()
	fx, _ := connectedFetcher(t, server.URL)

	src := &alertsSource{fetcher: fx, topic: "source:flow/node"}

	var msgs []models.Msg
	require.NoError(t, src.Produce(t.Context(), func(m models.Msg) error {
		msgs = append(msgs, m)
		return nil
	}))

	require.Len(t, msgs, 2, "one message per firing alert")
	assert.Equal(t, "abc", msgs[0].Key, "keyed by fingerprint")
	assert.Equal(t, SourceKind, msgs[0].SourceKind)

	var first alertPayload
	require.NoError(t, json.Unmarshal(msgs[0].Payload, &first))
	assert.Equal(t, "p99 over 1s", first.Title, "title prefers the summary annotation")
	assert.Equal(t, stateFiring, first.State)

	var second alertPayload
	require.NoError(t, json.Unmarshal(msgs[1].Payload, &second))
	assert.Equal(t, "DiskFull", second.Title, "title falls back to the alertname")
}

// hay-kot/hive-desktop#323: an alert carries everything the canonical item contract asks for, so it
// renders as an alert rather than as a heading and a timestamp.
func TestAlertsProduceFillsTheCanonicalContract(t *testing.T) {
	t.Parallel()

	server := alertsServer(t, `[{
		"fingerprint": "abc",
		"labels": {
			"alertname": "HighLatency",
			"grafana_folder": "Platform",
			"severity": "critical",
			"instance": "eu-west-1",
			"__alert_rule_uid__": "rule-uid-1",
			"__alert_rule_namespace_uid__": "ns-uid-1"
		},
		"annotations": {
			"summary": "p99 over 1s",
			"description": "The p99 latency crossed the threshold.",
			"__value_string__": "[ var='B' labels={} value=1.42 ]"
		},
		"startsAt": "2026-08-21T18:00:00Z",
		"status": {"state": "active"}
	}]`)
	defer server.Close()
	fx, _ := connectedFetcher(t, server.URL)

	src := &alertsSource{fetcher: fx, topic: "source:flow/node"}

	var msgs []models.Msg
	require.NoError(t, src.Produce(t.Context(), func(m models.Msg) error {
		msgs = append(msgs, m)
		return nil
	}))
	require.Len(t, msgs, 1)

	var payload alertPayload
	require.NoError(t, json.Unmarshal(msgs[0].Payload, &payload))

	assert.Equal(t, ItemKind, payload.Kind, "an alert is an Alert, not the default Item kind")
	assert.Equal(t, "p99 over 1s (eu-west-1)", payload.Title, "the instance separates rows one rule fired on several instances")
	assert.Equal(t, "Platform", payload.Repo, "the folder is the container label")
	assert.Equal(t, server.URL+"/alerting/grafana/rule-uid-1/view", payload.URL)
	assert.Contains(t, payload.Body, "The p99 latency crossed the threshold.")
	assert.Contains(t, payload.Body, "**Firing since** 2026-08-21T18:00:00Z")
	assert.Contains(t, payload.Body, "value=1.42")

	assert.Equal(t, []string{
		"alertname=HighLatency",
		"grafana_folder=Platform",
		"instance=eu-west-1",
		"severity=critical",
	}, payload.Labels, "canonical labels are sorted string tags with Grafana's own plumbing dropped")
	assert.Equal(t, "rule-uid-1", payload.AlertLabels["__alert_rule_uid__"], "the raw map survives as provider enrichment")
}

// An alert with nothing but a rule name still has to be renderable — and must
// not carry a link into a rule page that does not exist.
func TestAlertsProduceWithoutAnnotationsOrRuleUID(t *testing.T) {
	t.Parallel()

	server := alertsServer(t, `[{"fingerprint":"abc","labels":{"alertname":"DiskFull"},"annotations":{},"status":{"state":"active"}}]`)
	defer server.Close()
	fx, _ := connectedFetcher(t, server.URL)

	src := &alertsSource{fetcher: fx, topic: "source:flow/node"}

	var msgs []models.Msg
	require.NoError(t, src.Produce(t.Context(), func(m models.Msg) error {
		msgs = append(msgs, m)
		return nil
	}))
	require.Len(t, msgs, 1)

	var payload alertPayload
	require.NoError(t, json.Unmarshal(msgs[0].Payload, &payload))
	assert.Equal(t, "DiskFull", payload.Title)
	assert.Empty(t, payload.Body, "no description and no evaluation is no body, not an empty bullet list")
	assert.Equal(t, server.URL+"/alerting/list", payload.URL, "an alert with no rule uid links to the alert list")
	assert.Empty(t, payload.Repo)
}

// The whole point of hay-kot/hive-desktop#240: the narrowing happens at the stack, not after the
// payload has already crossed the wire.
func TestAlertsProduceFiltersServerSide(t *testing.T) {
	t.Parallel()

	var filters []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		filters = r.URL.Query()["filter"]
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()
	fx, _ := connectedFetcher(t, server.URL)

	matchers := []string{"squad=adaptive-telemetry", "severity=critical"}
	src := &alertsSource{fetcher: fx, matchers: matchers, topic: "source:flow/node"}

	require.NoError(t, src.Produce(t.Context(), func(models.Msg) error { return nil }))
	assert.Equal(t, matchers, filters, "the node's matchers reach Alertmanager unchanged")
}

func TestAlertsClassifierFiringThenResolved(t *testing.T) {
	t.Parallel()

	firing := models.Observation{ExternalID: "abc", Title: "HighLatency", Payload: []byte(`{"state":"firing"}`)}
	resolved := models.Observation{ExternalID: "abc", Title: "HighLatency", Payload: []byte(`{"state":"resolved"}`)}

	first := alertsClassifier{}.Classify(nil, firing)
	assert.Equal(t, stateFiring, first.Kind)
	assert.Equal(t, models.LifecycleActive, first.Lifecycle)

	transition := alertsClassifier{}.Classify(&firing, resolved)
	assert.Equal(t, stateResolved, transition.Kind)
	assert.Equal(t, models.LifecycleTerminal, transition.Lifecycle)
	assert.Equal(t, models.TransitionEnteredTerminal, transition.Transition)
	assert.Equal(t, stateResolved, transition.ArchivedReason)

	reopened := alertsClassifier{}.Classify(&resolved, firing)
	assert.Equal(t, models.TransitionLeftTerminal, reopened.Transition, "a re-firing alert leaves the terminal state")
}

// A still-firing alert re-observed with a changed payload takes the default arm
// every poll. It must stay trivial, not re-raise attention — the opposite would
// notify on every poll for the lifetime of the alert.
func TestAlertsClassifierReobservedStaysTrivial(t *testing.T) {
	t.Parallel()

	firing := models.Observation{ExternalID: "abc", Title: "HighLatency", Payload: []byte(`{"state":"firing"}`)}
	resolved := models.Observation{ExternalID: "abc", Title: "HighLatency", Payload: []byte(`{"state":"resolved"}`)}

	stillFiring := alertsClassifier{}.Classify(&firing, firing)
	assert.Equal(t, "updated", stillFiring.Kind)
	assert.Equal(t, models.TransitionNone, stillFiring.Transition)
	assert.Equal(t, models.AttentionTrivial, stillFiring.Attention, "a re-observed firing alert must not re-raise attention")

	stillResolved := alertsClassifier{}.Classify(&resolved, resolved)
	assert.Equal(t, "updated", stillResolved.Kind)
	assert.Equal(t, models.AttentionTrivial, stillResolved.Attention)
}

func TestAlertsAbsenceMarksResolvedAndTerminal(t *testing.T) {
	t.Parallel()

	previous := []models.Observation{{ExternalID: "abc", Title: "HighLatency", Payload: []byte(`{"title":"HighLatency","kind":"Alert","state":"firing","labels":["alertname=HighLatency"]}`)}}

	verdicts, err := alertsAbsence{}.ConfirmAbsence(t.Context(), previous)
	require.NoError(t, err)

	verdict, ok := verdicts["abc"]
	require.True(t, ok, "an absent alert gets a verdict")
	assert.True(t, verdict.Terminal, "an absent alert is authoritatively resolved, so its source head is evicted")
	require.NotNil(t, verdict.Current)

	assert.Equal(t, stateResolved, canonical.State(verdict.Current.Payload), "the payload is rewritten to resolved")
	// Other fields survive the rewrite so the archived item keeps its identity.
	var payload alertPayload
	require.NoError(t, json.Unmarshal(verdict.Current.Payload, &payload))
	assert.Equal(t, "HighLatency", payload.Title)
	assert.Equal(t, ItemKind, payload.Kind)
	assert.Equal(t, []string{"alertname=HighLatency"}, payload.Labels)
}
