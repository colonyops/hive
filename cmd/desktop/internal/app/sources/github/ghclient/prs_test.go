package ghclient

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// prServer answers one GraphQL POST with body, capturing the request that
// produced it.
func prServer(t *testing.T, body string) (*httptest.Server, *struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
},
) {
	t.Helper()
	var request struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// assert, not require: a require inside a handler aborts the server's
		// goroutine rather than the test, which hangs the client instead of
		// failing it.
		payload, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		assert.NoError(t, json.Unmarshal(payload, &request))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server, &request
}

func TestPullRequestsByBranchAnswersEachRefByPosition(t *testing.T) {
	t.Parallel()

	server, request := prServer(t, `{"data":{
      "r0": {"pullRequests":{"nodes":[{"number":311,"title":"Session top bar","state":"OPEN","isDraft":false,
        "url":"https://github.com/acme/site/pull/311","reviewDecision":"APPROVED","additions":420,"deletions":37,
        "commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"SUCCESS"}}}]}}]}},
      "r1": {"pullRequests":{"nodes":[]}}
    }}`)

	results, err := NewClient(WithAPIBase(server.URL)).WithTokenCopy("tok").PullRequestsByBranch(t.Context(), []BranchRef{
		{Owner: "acme", Repo: "site", Branch: "feat/bar"},
		{Owner: "acme", Repo: "site", Branch: "feat/unopened"},
	})
	require.NoError(t, err)
	require.Len(t, results, 2)

	assert.Equal(t, PullRequest{
		Found: true, Number: 311, Title: "Session top bar", State: "OPEN",
		URL: "https://github.com/acme/site/pull/311", ReviewDecision: "APPROVED", Checks: CheckStatePassing,
		// The pull request's own counts, not the working tree's.
		Additions: 420, Deletions: 37,
	}, results[0])
	// A branch with no pull request is data, not an error.
	assert.False(t, results[1].Found)

	// Branch names reach GitHub as variables, never as document text: a branch
	// called `") { id } }` must not be able to close the query.
	assert.NotContains(t, request.Query, "feat/bar")
	assert.Equal(t, "feat/bar", request.Variables["b0"])
	assert.Equal(t, "feat/unopened", request.Variables["b1"])
	assert.Equal(t, "acme", request.Variables["o0"])
	assert.Equal(t, "site", request.Variables["n0"])
}

// A repository this token cannot see resolves to a null alias beside a
// NOT_FOUND error. The batch must still answer for every other ref, or one
// private repo in a sidebar blanks the rest.
func TestPullRequestsByBranchToleratesAnInaccessibleRepository(t *testing.T) {
	t.Parallel()

	server, _ := prServer(t, `{
      "data": {"r0": null, "r1": {"pullRequests":{"nodes":[{"number":7,"state":"MERGED","isDraft":false,
        "commits":{"nodes":[{"commit":{"statusCheckRollup":null}}]}}]}}},
      "errors": [{"type":"NOT_FOUND","message":"Could not resolve to a Repository"}]
    }`)

	results, err := NewClient(WithAPIBase(server.URL)).WithTokenCopy("tok").PullRequestsByBranch(t.Context(), []BranchRef{
		{Owner: "acme", Repo: "private", Branch: "main"},
		{Owner: "acme", Repo: "site", Branch: "feat/bar"},
	})
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.False(t, results[0].Found)
	assert.True(t, results[1].Found)
	assert.Equal(t, "MERGED", results[1].State)
	// A head commit with no checks configured reports none, not passing.
	assert.Equal(t, CheckStateNone, results[1].Checks)
}

func TestReviewStatePrefersDraftAndNormalizesDecisions(t *testing.T) {
	t.Parallel()

	assert.Equal(t, ReviewStateDraft, reviewState(true, "APPROVED"))
	assert.Equal(t, ReviewStateApproved, reviewState(false, "APPROVED"))
	assert.Equal(t, ReviewStateChangesRequested, reviewState(false, "CHANGES_REQUESTED"))
	assert.Equal(t, ReviewStateReviewRequired, reviewState(false, "REVIEW_REQUIRED"))
	assert.Equal(t, ReviewStateOpen, reviewState(false, ""))
	assert.Equal(t, ReviewStateOpen, reviewState(false, "A_FUTURE_STATE"))
}

func TestCheckStateNeverReadsAnUnknownRollupAsPassing(t *testing.T) {
	t.Parallel()

	assert.Equal(t, CheckStatePassing, checkState("SUCCESS"))
	assert.Equal(t, CheckStateFailing, checkState("FAILURE"))
	assert.Equal(t, CheckStateFailing, checkState("ERROR"))
	assert.Equal(t, CheckStatePending, checkState("PENDING"))
	assert.Equal(t, CheckStatePending, checkState("EXPECTED"))
	assert.Equal(t, CheckStateNone, checkState(""))
	assert.Equal(t, CheckStatePending, checkState("SOMETHING_GITHUB_ADDS_LATER"))
}
