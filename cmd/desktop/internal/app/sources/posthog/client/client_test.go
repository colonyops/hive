package client

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/sourcehttp"
)

func TestProjectsListsAndAuthenticates(t *testing.T) {
	t.Parallel()

	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/projects/", r.URL.Path)
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"count":2,"results":[{"id":1,"uuid":"u1","name":"Dev"},{"id":2,"uuid":"u2","name":"Prod"}]}`))
	}))
	defer server.Close()

	projects, err := NewClient(server.URL, "phx-key").Projects(t.Context())
	require.NoError(t, err)
	require.Len(t, projects, 2)
	assert.Equal(t, 2, projects[1].ID)
	assert.Equal(t, "Prod", projects[1].Name)
	assert.Equal(t, "Bearer phx-key", gotAuth, "the personal API key is sent as a bearer")
}

func TestProjectsUnauthorized(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := NewClient(server.URL, "bad").Projects(t.Context())
	assert.ErrorIs(t, err, sourcehttp.ErrUnauthorized)
}

func TestIssuesPostsTheTypedQuery(t *testing.T) {
	t.Parallel()

	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/projects/7/error_tracking/query/issues/", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		raw, _ := io.ReadAll(r.Body)
		assert.NoError(t, json.Unmarshal(raw, &body))
		_, _ = w.Write([]byte(`{"results":[
			{"id":"i1","name":"TypeError","description":"x is not a function","status":"active",
			 "first_seen":"2026-08-01T10:00:00Z","last_seen":"2026-08-03T09:30:00Z","library":"posthog-js",
			 "aggregations":{"occurrences":1204,"users":37,"sessions":52}}
		],"hasMore":true,"limit":25,"offset":0,"nextOffset":25}`))
	}))
	defer server.Close()

	issues, err := NewClient(server.URL, "phx-key").Issues(t.Context(), 7, IssuesRequest{
		DateRange:          DateRange{DateFrom: "-24h"},
		Status:             "active",
		OrderBy:            "last_seen",
		OrderDirection:     "DESC",
		Limit:              25,
		FilterTestAccounts: true,
	})
	require.NoError(t, err)
	require.Len(t, issues, 1)

	assert.Equal(t, "i1", issues[0].ID)
	assert.Equal(t, "TypeError", issues[0].Name)
	assert.Equal(t, "active", issues[0].Status)
	assert.Equal(t, "posthog-js", issues[0].Library)
	require.NotNil(t, issues[0].Aggregations)
	assert.InDelta(t, 1204, issues[0].Aggregations.Occurrences, 0.001)

	// The endpoint validates these names, so a rename here is a 400 at runtime.
	assert.Equal(t, "active", body["status"])
	assert.Equal(t, "last_seen", body["orderBy"])
	assert.Equal(t, "DESC", body["orderDirection"])
	assert.InDelta(t, 25, body["limit"], 0.001)
	assert.Equal(t, true, body["filterTestAccounts"])
	assert.Equal(t, map[string]any{"date_from": "-24h"}, body["dateRange"])
}

// An issue with no aggregations must decode rather than fail: PostHog omits
// them when the query asks for compact counts and there are none.
func TestIssuesToleratesMissingAggregations(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"id":"i1","name":null,"description":null,"status":"active"}]}`))
	}))
	defer server.Close()

	issues, err := NewClient(server.URL, "k").Issues(t.Context(), 7, IssuesRequest{})
	require.NoError(t, err)
	require.Len(t, issues, 1)
	assert.Nil(t, issues[0].Aggregations)
	assert.Empty(t, issues[0].Name, "a null name decodes to empty, not to an error")
}

func TestIssuesRateLimited(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	_, err := NewClient(server.URL, "k").Issues(t.Context(), 7, IssuesRequest{})
	assert.ErrorIs(t, err, sourcehttp.ErrRateLimited)
}

// A 400 from the query endpoint is what a schema drift looks like, so its
// message has to reach the log rather than being flattened to "HTTP 400".
func TestIssuesSurfacesTheServerMessage(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"detail":"orderBy: this field is required"}`))
	}))
	defer server.Close()

	_, err := NewClient(server.URL, "k").Issues(t.Context(), 7, IssuesRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "orderBy: this field is required")
}

func TestAlertsListsEveryAlert(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/projects/7/alerts/", r.URL.Path)
		assert.Equal(t, "100", r.URL.Query().Get("limit"))
		_, _ = w.Write([]byte(`{"next":null,"results":[
			{"id":"a1","name":"Signups fell","state":"Firing","enabled":true,
			 "insight":{"id":9,"short_id":"AbCdEf12","name":"Daily signups"},
			 "condition":{"type":"absolute_value"},"last_value":4}
		]}`))
	}))
	defer server.Close()

	alerts, truncated, err := NewClient(server.URL, "k").Alerts(t.Context(), 7)
	require.NoError(t, err)
	require.Len(t, alerts, 1)
	assert.False(t, truncated)

	assert.Equal(t, "a1", alerts[0].ID)
	assert.Equal(t, "Firing", alerts[0].State, "the client carries PostHog's display string verbatim")
	assert.Equal(t, "AbCdEf12", alerts[0].Insight.ShortID)
	require.NotNil(t, alerts[0].LastValue)
	assert.InDelta(t, 4, *alerts[0].LastValue, 0.001)
}

// A project past one page must be reported, not silently truncated.
func TestAlertsReportsTruncation(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"next":"https://us.posthog.com/api/projects/7/alerts/?offset=100","results":[]}`))
	}))
	defer server.Close()

	_, truncated, err := NewClient(server.URL, "k").Alerts(t.Context(), 7)
	require.NoError(t, err)
	assert.True(t, truncated)
}

func TestIssueAndInsightURLs(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "https://us.posthog.com/project/42/error_tracking/i1",
		IssueURL("https://us.posthog.com", 42, "i1"))
	assert.Equal(t, "https://us.posthog.com/project/42/insights/AbCdEf12",
		InsightURL("https://us.posthog.com", 42, "AbCdEf12"))
	assert.Equal(t, "https://us.posthog.com/project/42/insights",
		InsightURL("https://us.posthog.com", 42, ""), "an alert with no insight short id still links somewhere")
}
