package posthog

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
)

func TestPostHogAlertsConfigValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		config  AlertsConfig
		wantErr bool
	}{
		{name: "valid", config: AlertsConfig{Credential: "posthog/us.posthog.com-1"}},
		{name: "zero config", config: AlertsConfig{}, wantErr: true},
		{name: "wrong provider", config: AlertsConfig{Credential: "grafana/host-1"}, wantErr: true},
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
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/projects/42/alerts/", r.URL.Path)
		_, _ = w.Write([]byte(body))
	}))
}

const twoAlerts = `{"results":[
	{"id":"a1","name":"Signups fell","state":"Firing","enabled":true,
	 "insight":{"id":9,"short_id":"AbCdEf12","name":"Daily signups"},
	 "threshold":{"configuration":{"bounds":{"lower":10}}},"condition":{"type":"absolute_value"},
	 "calculation_interval":"hourly","last_value":4,"last_notified_at":"2026-08-03T09:00:00Z"},
	{"id":"a2","name":"Latency","state":"Not firing","enabled":true,
	 "insight":{"id":10,"short_id":"ZzYyXx99","name":"p99"},"last_checked_at":"2026-08-03T09:05:00Z"}
],"next":null}`

func TestPostHogAlertsProduceEmitsOnePerAlert(t *testing.T) {
	t.Parallel()

	server := alertsServer(t, twoAlerts)
	defer server.Close()

	fx, _ := connectedFetcher(t, server.URL)
	src := &alertsSource{fetcher: fx, topic: "source:flow/node"}

	var msgs []models.Msg
	require.NoError(t, src.Produce(t.Context(), func(m models.Msg) error {
		msgs = append(msgs, m)
		return nil
	}))

	require.Len(t, msgs, 2, "every alert is emitted, so one that stops firing updates its item")
	assert.Equal(t, "a1", msgs[0].Key, "keyed by alert id")
	assert.Equal(t, SourceKind, msgs[0].SourceKind)

	var firing alertPayload
	require.NoError(t, json.Unmarshal(msgs[0].Payload, &firing))
	assert.Equal(t, "Signups fell", firing.Title)
	assert.Equal(t, stateFiring, firing.State)
	assert.Equal(t, server.URL+"/project/42/insights/AbCdEf12", firing.URL, "an alert deep-links to its insight")
	assert.Equal(t, "Daily signups", firing.Insight)
	assert.Equal(t, "hourly", firing.CalculationInterval)
	require.NotNil(t, firing.LastValue)
	assert.InDelta(t, 4, *firing.LastValue, 0.001)
	assert.JSONEq(t, `{"type":"absolute_value"}`, string(firing.Condition), "the condition rides along verbatim")

	var quiet alertPayload
	require.NoError(t, json.Unmarshal(msgs[1].Payload, &quiet))
	assert.Equal(t, stateNotFiring, quiet.State, `"Not firing" is normalized`)
}

func TestPostHogAlertsFiringOnlySkipsQuietAlerts(t *testing.T) {
	t.Parallel()

	server := alertsServer(t, twoAlerts)
	defer server.Close()

	fx, _ := connectedFetcher(t, server.URL)
	src := &alertsSource{fetcher: fx, firingOnly: true, topic: "t"}

	var msgs []models.Msg
	require.NoError(t, src.Produce(t.Context(), func(m models.Msg) error {
		msgs = append(msgs, m)
		return nil
	}))

	require.Len(t, msgs, 1)
	assert.Equal(t, "a1", msgs[0].Key)
}

// An alert whose insight carries no short id still has to link somewhere.
func TestPostHogAlertWithoutInsightShortIDLinksToTheProject(t *testing.T) {
	t.Parallel()

	server := alertsServer(t, `{"results":[{"id":"a1","name":"Orphan","state":"Firing"}]}`)
	defer server.Close()

	fx, _ := connectedFetcher(t, server.URL)
	src := &alertsSource{fetcher: fx, topic: "t"}

	var msgs []models.Msg
	require.NoError(t, src.Produce(t.Context(), func(m models.Msg) error {
		msgs = append(msgs, m)
		return nil
	}))
	require.Len(t, msgs, 1)

	var payload alertPayload
	require.NoError(t, json.Unmarshal(msgs[0].Payload, &payload))
	assert.Equal(t, server.URL+"/project/42/insights", payload.URL)
}

func alertObservation(id, state string) models.Observation {
	payload, _ := json.Marshal(alertPayload{ID: id, Title: "Signups fell", State: state})
	return models.Observation{ExternalID: id, Title: "Signups fell", Payload: payload}
}

func TestPostHogAlertsClassifierFiringThenResolved(t *testing.T) {
	t.Parallel()

	firing := alertObservation("a1", stateFiring)
	quiet := alertObservation("a1", stateNotFiring)

	first := alertsClassifier{}.Classify(nil, firing)
	assert.Equal(t, stateFiring, first.Kind)
	assert.Equal(t, models.LifecycleActive, first.Lifecycle)

	resolved := alertsClassifier{}.Classify(&firing, quiet)
	assert.Equal(t, models.LifecycleTerminal, resolved.Lifecycle)
	assert.Equal(t, models.TransitionEnteredTerminal, resolved.Transition)
	assert.Equal(t, "Resolved", resolved.Summary)

	refiring := alertsClassifier{}.Classify(&quiet, firing)
	assert.Equal(t, models.TransitionLeftTerminal, refiring.Transition)
	assert.Equal(t, "Firing", refiring.Summary)
}

func TestPostHogAlertsClassifierReobservedStaysTrivial(t *testing.T) {
	t.Parallel()

	firing := alertObservation("a1", stateFiring)

	got := alertsClassifier{}.Classify(&firing, firing)
	assert.Equal(t, "updated", got.Kind)
	assert.Equal(t, models.AttentionTrivial, got.Attention, "a still-firing alert must not re-notify every poll")
}

// Only firing is a breach worth attention; snoozed and errored are not.
func TestPostHogAlertsClassifierNonFiringStatesAreTerminal(t *testing.T) {
	t.Parallel()

	for _, state := range []string{stateNotFiring, "snoozed", "errored"} {
		got := alertsClassifier{}.Classify(nil, alertObservation("a1", state))
		assert.Equalf(t, models.LifecycleTerminal, got.Lifecycle, "state %q", state)
	}
}

func TestAlertStateNormalization(t *testing.T) {
	t.Parallel()

	assert.Equal(t, stateFiring, alertState("Firing"))
	assert.Equal(t, stateNotFiring, alertState("Not firing"))
	assert.Equal(t, stateNotFiring, alertState(""))
	assert.Equal(t, "snoozed", alertState("Snoozed"))
	assert.Equal(t, "pending_resolve", alertState("Pending-resolve"))
}

// last_notified_at beats last_checked_at: an alert checked every 15 minutes
// would otherwise look freshly active on every poll.
func TestLastActivityPrefersNotification(t *testing.T) {
	t.Parallel()

	server := alertsServer(t, twoAlerts)
	defer server.Close()

	fx, _ := connectedFetcher(t, server.URL)
	src := &alertsSource{fetcher: fx, topic: "t"}

	var msgs []models.Msg
	require.NoError(t, src.Produce(t.Context(), func(m models.Msg) error {
		msgs = append(msgs, m)
		return nil
	}))

	var firing, quiet alertPayload
	require.NoError(t, json.Unmarshal(msgs[0].Payload, &firing))
	require.NoError(t, json.Unmarshal(msgs[1].Payload, &quiet))

	assert.Equal(t, epochMillis("2026-08-03T09:00:00Z"), firing.UpdatedAt, "the notification time wins")
	assert.Equal(t, epochMillis("2026-08-03T09:05:00Z"), quiet.UpdatedAt, "falling back to the check time")
}
