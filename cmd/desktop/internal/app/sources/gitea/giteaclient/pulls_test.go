package giteaclient

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pullServer routes on the escaped path, which is what a branch name with a
// slash in it arrives as.
func pullServer(t *testing.T, routes map[string]string) (*Client, map[string]int) {
	t.Helper()
	seen := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		seen[path]++
		body, ok := routes[path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"not found"}`))
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return NewClient(server.URL, "tok"), seen
}

const openPull = `{"number":12,"title":"Add the thing","state":"open","merged":false,"draft":true,
	"html_url":"https://git.example.com/acme/site/pulls/12","additions":31,"deletions":4,
	"head":{"ref":"feat/bar","sha":"cafe"},"requested_reviewers":[]}`

func TestPullRequestForBranchReadsThePullRequestItsChecksAndItsReviews(t *testing.T) {
	t.Parallel()

	client, seen := pullServer(t, map[string]string{
		"/api/v1/repos/acme/site":                       `{"default_branch":"main"}`,
		"/api/v1/repos/acme/site/pulls/main/feat%2Fbar": openPull,
		"/api/v1/repos/acme/site/commits/cafe/status":   `{"state":"success","total_count":2}`,
		"/api/v1/repos/acme/site/pulls/12/reviews": `[
			{"state":"REQUEST_CHANGES","user":{"login":"hubot"},"submitted_at":"2026-08-01T10:00:00Z"},
			{"state":"APPROVED","user":{"login":"hubot"},"submitted_at":"2026-08-02T10:00:00Z"}
		]`,
	})

	pull, found, err := client.PullRequestForBranch(t.Context(), "acme", "site", "feat/bar")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, PullRequest{
		Number: 12, Title: "Add the thing", State: "OPEN", IsDraft: true,
		URL: "https://git.example.com/acme/site/pulls/12", ReviewDecision: ReviewApproved,
		Checks: CheckStatePassing, Additions: 31, Deletions: 4,
	}, pull)
	assert.Equal(t, 0, seen["/api/v1/repos/acme/site/pulls"], "the direct lookup answered, so nothing was scanned")
}

// A merge deletes the head branch and rewrites the pull request's head ref, so
// the by-base-and-head lookup is the only one that still finds it. Its reviews
// are not read: the bar paints a merged pull request by its state.
func TestPullRequestForBranchFindsAMergedPullRequestWhoseBranchIsGone(t *testing.T) {
	t.Parallel()

	client, seen := pullServer(t, map[string]string{
		"/api/v1/repos/acme/site": `{"default_branch":"main"}`,
		"/api/v1/repos/acme/site/pulls/main/feat%2Fbar": `{"number":12,"state":"closed","merged":true,
			"html_url":"https://git.example.com/acme/site/pulls/12","head":{"ref":"refs/pull/12/head","sha":"cafe"}}`,
		"/api/v1/repos/acme/site/commits/cafe/status": `{"state":"success","total_count":1}`,
	})

	pull, found, err := client.PullRequestForBranch(t.Context(), "acme", "site", "feat/bar")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "MERGED", pull.State)
	assert.Equal(t, 0, seen["/api/v1/repos/acme/site/pulls/12/reviews"])
}

// Gitea's pull list filters by base and never by head, so a pull request onto
// another base is only found by scanning. The list omits the diff stats, which
// is why the match is re-read whole.
func TestPullRequestForBranchScansTheOpenListForAnotherBase(t *testing.T) {
	t.Parallel()

	client, _ := pullServer(t, map[string]string{
		"/api/v1/repos/acme/site": `{"default_branch":"main"}`,
		"/api/v1/repos/acme/site/pulls": `[
			{"number":8,"head":{"ref":"feat/other","sha":"beef"}},
			{"number":12,"head":{"ref":"feat/bar","sha":"cafe"}}
		]`,
		"/api/v1/repos/acme/site/pulls/12":            openPull,
		"/api/v1/repos/acme/site/commits/cafe/status": `{"state":"pending","total_count":1}`,
		"/api/v1/repos/acme/site/pulls/12/reviews":    `[]`,
	})

	pull, found, err := client.PullRequestForBranch(t.Context(), "acme", "site", "feat/bar")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, 12, pull.Number)
	assert.Equal(t, 31, pull.Additions, "the list carries no diff stats, so the match is re-read")
	assert.Equal(t, CheckStatePending, pull.Checks)
}

// A branch with no pull request is a verdict, not a failure: the bar shows
// nothing rather than an error.
func TestPullRequestForBranchReportsNoneWhenNothingMatches(t *testing.T) {
	t.Parallel()

	client, _ := pullServer(t, map[string]string{
		"/api/v1/repos/acme/site":       `{"default_branch":"main"}`,
		"/api/v1/repos/acme/site/pulls": `[]`,
	})

	_, found, err := client.PullRequestForBranch(t.Context(), "acme", "site", "feat/bar")
	require.NoError(t, err)
	assert.False(t, found)
}

// A commit no CI reported on answers with an empty status list, which is not
// the same as a run that has not finished.
func TestPullRequestForBranchReportsNoChecksForACommitWithNoStatuses(t *testing.T) {
	t.Parallel()

	client, _ := pullServer(t, map[string]string{
		"/api/v1/repos/acme/site":                       `{"default_branch":"main"}`,
		"/api/v1/repos/acme/site/pulls/main/feat%2Fbar": openPull,
		"/api/v1/repos/acme/site/commits/cafe/status":   `{"state":"pending","total_count":0}`,
		"/api/v1/repos/acme/site/pulls/12/reviews":      `[]`,
	})

	pull, found, err := client.PullRequestForBranch(t.Context(), "acme", "site", "feat/bar")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, CheckStateNone, pull.Checks)
}

func TestFoldReviewDecision(t *testing.T) {
	t.Parallel()

	review := func(login, state string, day int, dismissed bool) pullReview {
		var r pullReview
		r.State, r.Dismissed = state, dismissed
		r.User.Login = login
		r.SubmittedAt = time.Date(2026, 8, day, 0, 0, 0, 0, time.UTC)
		return r
	}

	tests := []struct {
		name      string
		reviews   []pullReview
		requested int
		want      string
	}{
		{name: "nothing at all", want: ""},
		{name: "a request alone is a required review", requested: 1, want: ReviewRequired},
		{
			name:    "one reviewer's later approval replaces their change request",
			reviews: []pullReview{review("hubot", "REQUEST_CHANGES", 1, false), review("hubot", "APPROVED", 2, false)},
			want:    ReviewApproved,
		},
		{
			name:    "another reviewer's standing change request outweighs an approval",
			reviews: []pullReview{review("octocat", "APPROVED", 2, false), review("hubot", "REQUEST_CHANGES", 1, false)},
			want:    ReviewChangesRequested,
		},
		{
			name:    "a dismissed review counts for nothing",
			reviews: []pullReview{review("hubot", "REQUEST_CHANGES", 1, true)},
			want:    "",
		},
		{
			name:      "comments state no position",
			reviews:   []pullReview{review("hubot", "COMMENT", 1, false)},
			requested: 1,
			want:      ReviewRequired,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, foldReviewDecision(tt.reviews, tt.requested))
		})
	}
}
