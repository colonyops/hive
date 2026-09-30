package client

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOnCallURLReadsPluginSettings(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/plugins/grafana-irm-app/settings", r.URL.Path)
		assert.Equal(t, "Bearer t", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"jsonData":{"onCallApiUrl":"https://oncall.example.com/oncall/"}}`))
	}))
	defer server.Close()

	base, err := NewClient(server.URL, "t").OnCallURL(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "https://oncall.example.com/oncall", base, "the trailing slash is trimmed so path joins stay single-slashed")
}

// A stack without IRM answers the settings call but names no OnCall host. That
// is a configuration problem, not a decode failure, so it must say so.
func TestOnCallURLRejectsAnUnconfiguredPlugin(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"jsonData":{}}`))
	}))
	defer server.Close()

	_, err := NewClient(server.URL, "t").OnCallURL(t.Context())
	assert.ErrorContains(t, err, "no OnCall API URL")
}

func TestAlertGroupsFetchesEachStateWithScopeAndAuth(t *testing.T) {
	t.Parallel()

	var states [][]string
	var integrations, teams, auths, grafanaURLs []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/alert_groups/", r.URL.Path)
		query := r.URL.Query()
		states = append(states, query["state"])
		integrations = append(integrations, query.Get("integration_id"))
		teams = append(teams, query.Get("team_id"))
		auths = append(auths, r.Header.Get("Authorization"))
		grafanaURLs = append(grafanaURLs, r.Header.Get("X-Grafana-URL"))
		_, _ = fmt.Fprintf(w, `{"results":[{"id":%q,"state":%q}],"next":null}`, query.Get("state"), query.Get("state"))
	}))
	defer server.Close()

	groups, err := NewOnCallClient(server.URL, "https://stack.example.com", "t").AlertGroups(t.Context(), AlertGroupQuery{
		States:        []string{"new", "acknowledged", "silenced"},
		IntegrationID: "CFRPV98RPR1U8",
		TeamID:        "T3HRAP3K2FE1J",
	})
	require.NoError(t, err)

	assert.Equal(t, [][]string{{"new"}, {"acknowledged"}, {"silenced"}}, states, "each request carries exactly one state")
	assert.Equal(t, []string{"CFRPV98RPR1U8", "CFRPV98RPR1U8", "CFRPV98RPR1U8"}, integrations)
	assert.Equal(t, []string{"T3HRAP3K2FE1J", "T3HRAP3K2FE1J", "T3HRAP3K2FE1J"}, teams)
	assert.Equal(t, []string{"Bearer t", "Bearer t", "Bearer t"}, auths)
	assert.Equal(t, []string{"https://stack.example.com", "https://stack.example.com", "https://stack.example.com"}, grafanaURLs,
		"OnCall resolves the token's org from the stack URL")
	require.Len(t, groups, 3)
	assert.Equal(t, []string{"new", "acknowledged", "silenced"}, []string{groups[0].ID, groups[1].ID, groups[2].ID})
}

// An empty scope must not send empty params — the API reads integration_id=""
// as "match the integration named empty string", which matches nothing.
func TestAlertGroupsOmitsAnEmptyScope(t *testing.T) {
	t.Parallel()

	var query string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"results":[],"next":null}`))
	}))
	defer server.Close()

	_, err := NewOnCallClient(server.URL, "https://stack.example.com", "t").AlertGroups(t.Context(), AlertGroupQuery{})
	require.NoError(t, err)
	assert.NotContains(t, query, "integration_id")
	assert.NotContains(t, query, "team_id")
	assert.NotContains(t, query, "state=")
}

func TestAlertGroupsDecodesAGroup(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{
			"id":"I68T24C13IFW1",
			"integration_id":"CFRPV98RPR1U8",
			"team_id":"T3HRAP3K2FE1J",
			"alerts_count":6,
			"state":"acknowledged",
			"title":"Memory above 90% threshold",
			"created_at":"2026-08-01T12:00:00Z",
			"acknowledged_at":"2026-08-01T12:04:00Z",
			"silenced_at":null,
			"permalinks":{"slack":"https://slack.example.com/thread","telegram":null},
			"labels":[
				{"key":{"id":"k1","name":"severity"},"value":{"id":"v1","name":"critical"}},
				{"key":{"id":"k2","name":"squad"},"value":{"id":"v2","name":"adaptive-telemetry"}}
			],
			"last_alert":{
				"id":"AA74DN7T4JQB6",
				"alert_group_id":"I68T24C13IFW1",
				"created_at":"2026-08-01T12:03:00Z",
				"payload":{"commonLabels":{"cluster":"production-east"}}
			}
		}],"next":null}`))
	}))
	defer server.Close()

	groups, err := NewOnCallClient(server.URL, "https://stack.example.com", "t").AlertGroups(t.Context(), AlertGroupQuery{})
	require.NoError(t, err)
	require.Len(t, groups, 1)

	group := groups[0]
	assert.Equal(t, "I68T24C13IFW1", group.ID)
	assert.Equal(t, "acknowledged", group.State)
	assert.Equal(t, 6, group.AlertsCount)
	assert.Equal(t, "T3HRAP3K2FE1J", group.TeamID)
	// A null permalink and a null timestamp are ordinary, not a decode failure.
	assert.Empty(t, group.Permalinks.Telegram)
	assert.Empty(t, group.SilencedAt)
	assert.Equal(t, "https://slack.example.com/thread", group.URL())
	assert.Equal(t, map[string]string{"severity": "critical", "squad": "adaptive-telemetry"}, group.LabelMap())
	require.NotNil(t, group.LastAlert)
	assert.Equal(t, "AA74DN7T4JQB6", group.LastAlert.ID)
	assert.JSONEq(t, `{"commonLabels":{"cluster":"production-east"}}`, string(group.LastAlert.Payload))
}

// Slack is where the on-call conversation is, so it wins; the IRM web page is
// the fallback when the group was never posted to a channel.
func TestAlertGroupURLPrefersSlackThenWeb(t *testing.T) {
	t.Parallel()

	both := AlertGroup{Permalinks: Permalinks{Slack: "https://slack", Web: "https://web"}}
	assert.Equal(t, "https://slack", both.URL())

	webOnly := AlertGroup{Permalinks: Permalinks{Web: "https://web"}}
	assert.Equal(t, "https://web", webOnly.URL())

	assert.Empty(t, AlertGroup{}.URL(), "a group with no links has no url rather than a broken one")
}

func TestAlertGroupsWalksEveryPageForEachState(t *testing.T) {
	t.Parallel()

	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state, page := r.URL.Query().Get("state"), r.URL.Query().Get("page")
		requests = append(requests, state+"/"+page)
		if page == "1" {
			_, _ = fmt.Fprintf(w, `{"results":[{"id":%q}],"next":"%s/api/v1/alert_groups/?page=2"}`, state+page, r.Host)
			return
		}
		_, _ = fmt.Fprintf(w, `{"results":[{"id":%q}],"next":null}`, state+page)
	}))
	defer server.Close()

	groups, err := NewOnCallClient(server.URL, "https://stack.example.com", "t").AlertGroups(t.Context(), AlertGroupQuery{
		States: []string{"new", "acknowledged"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"new/1", "new/2", "acknowledged/1", "acknowledged/2"}, requests)
	require.Len(t, groups, 4)
	assert.Equal(t, []string{"new1", "new2", "acknowledged1", "acknowledged2"},
		[]string{groups[0].ID, groups[1].ID, groups[2].ID, groups[3].ID})
}

func TestAlertGroupsDeduplicatesByIDDeterministically(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("state") {
		case "new":
			_, _ = w.Write([]byte(`{"results":[{"id":"shared","state":"new"},{"id":"new-only","state":"new"}],"next":null}`))
		case "acknowledged":
			_, _ = w.Write([]byte(`{"results":[{"id":"shared","state":"acknowledged"},{"id":"ack-only","state":"acknowledged"}],"next":null}`))
		}
	}))
	defer server.Close()

	groups, err := NewOnCallClient(server.URL, "https://stack.example.com", "t").AlertGroups(t.Context(), AlertGroupQuery{
		States: []string{"new", "acknowledged"},
	})
	require.NoError(t, err)
	require.Len(t, groups, 3)
	assert.Equal(t, []string{"shared", "new-only", "ack-only"}, []string{groups[0].ID, groups[1].ID, groups[2].ID})
	assert.Equal(t, "acknowledged", groups[0].State, "the later request has the freshest state")
}

func TestAlertGroupsReturnsNoPartialResultsOnError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") == "acknowledged" {
			http.Error(w, "failed", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"results":[{"id":"new"}],"next":null}`))
	}))
	defer server.Close()

	groups, err := NewOnCallClient(server.URL, "https://stack.example.com", "t").AlertGroups(t.Context(), AlertGroupQuery{
		States: []string{"new", "acknowledged"},
	})
	require.Error(t, err)
	assert.Nil(t, groups, "a failed state must discard results from earlier states")
}

// A short snapshot would read as "these groups are gone" and archive live
// alerts, so running out of pages has to fail the poll instead.
func TestAlertGroupsFailsRatherThanTruncate(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"id":"a"}],"next":"http://next.example.com"}`))
	}))
	defer server.Close()

	groups, err := NewOnCallClient(server.URL, "https://stack.example.com", "t").AlertGroups(t.Context(), AlertGroupQuery{})
	require.Error(t, err)
	assert.Nil(t, groups, "a truncated set must not be returned alongside the error")
	assert.ErrorContains(t, err, "scope this source", "the error says how to fix it")
}
