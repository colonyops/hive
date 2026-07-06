package ghcli

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/core/kv"
	"github.com/colonyops/hive/internal/data/db"
	"github.com/colonyops/hive/internal/data/stores"
	"github.com/colonyops/hive/internal/sources"
	"github.com/colonyops/hive/internal/sources/cliengine"
	"github.com/colonyops/hive/pkg/executil/executiltest"
)

// fixClock pins cliengine.TimeNow so ShortAge output is deterministic.
func fixClock(t *testing.T, now time.Time) {
	t.Helper()
	prev := cliengine.TimeNow
	cliengine.TimeNow = func() time.Time { return now }
	t.Cleanup(func() { cliengine.TimeNow = prev })
}

func newTestKV(t *testing.T) kv.KV {
	t.Helper()
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	return stores.NewKVStore(database)
}

func newIssues(t *testing.T, exec *executiltest.Exec) *cliengine.Source {
	t.Helper()
	c, err := cliengine.New(Issues(), exec, newTestKV(t), cliengine.Options{})
	require.NoError(t, err)
	return c
}

func newPRs(t *testing.T, exec *executiltest.Exec) *cliengine.Source {
	t.Helper()
	c, err := cliengine.New(PRs(), exec, newTestKV(t), cliengine.Options{})
	require.NoError(t, err)
	return c
}

func TestIssuesManifest(t *testing.T) {
	c := newIssues(t, &executiltest.Exec{})

	manifest, err := c.Initialize(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "issues", manifest.ID)
	assert.Equal(t, "Issues", manifest.DisplayName)
	assert.True(t, manifest.Capabilities.FetchDetail)
}

func TestPRsManifest(t *testing.T) {
	c := newPRs(t, &executiltest.Exec{})

	manifest, err := c.Initialize(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "prs", manifest.ID)
	assert.Equal(t, "Pull Requests", manifest.DisplayName)
	assert.False(t, manifest.Capabilities.FetchDetail, "prs source has no detail view")
}

func TestSearchIssues(t *testing.T) {
	fixClock(t, time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC))
	exec := &executiltest.Exec{
		Responses: []executiltest.Response{
			{Out: []byte(`[
				{"number":1,"title":"First issue","state":"OPEN","author":{"login":"alice"},"labels":[{"name":"api"},{"name":"public"}],"url":"https://github.com/o/r/issues/1","createdAt":"2026-07-01T00:00:00Z","assignees":[{"login":"carol"}],"closedByPullRequestsReferences":[{"number":42}]},
				{"number":2,"title":"Second issue","state":"CLOSED","author":{"login":"bob"},"labels":[],"url":"https://github.com/o/r/issues/2","createdAt":"2026-07-03T00:00:00Z","assignees":[],"closedByPullRequestsReferences":[]}
			]`)},
		},
	}
	c := newIssues(t, exec)

	result, err := c.Search(context.Background(), sources.SearchParams{Scope: "o/r", Query: "bug"})
	require.NoError(t, err)
	require.Len(t, exec.Calls(), 1)

	call := exec.Calls()[0]
	assert.Equal(t, "gh", call.Cmd)
	assert.Equal(t, []string{
		"issue", "list",
		"--repo", "o/r",
		"--json", "number,title,state,author,labels,url,createdAt,assignees,closedByPullRequestsReferences",
		"--limit", "30",
		"--search", "bug",
	}, call.Args)

	require.Len(t, result.Items, 2)
	assert.Equal(t, "1", result.Items[0].ID)
	assert.Equal(t, "First issue", result.Items[0].Title)
	assert.Equal(t, "#1 · OPEN", result.Items[0].Subtitle)
	assert.Equal(t, "https://github.com/o/r/issues/1", result.Items[0].URI)
	assert.Equal(t, map[string]any{
		"number":          1,
		"title":           "First issue",
		"state":           "OPEN",
		"url":             "https://github.com/o/r/issues/1",
		"author":          "alice",
		"labels":          []string{"api", "public"},
		"age":             "3d",
		"linked_pr":       42,
		"linked_pr_count": 1,
		"assignee":        "carol",
		"assignee_count":  1,
	}, result.Items[0].Fields)

	assert.Equal(t, "1d", result.Items[1].Fields["age"])
	assert.Equal(t, 0, result.Items[1].Fields["linked_pr"])
	assert.Empty(t, result.Items[1].Fields["assignee"])
}

func TestSearchPRs(t *testing.T) {
	fixClock(t, time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC))
	exec := &executiltest.Exec{
		Responses: []executiltest.Response{
			{Out: []byte(`[
				{"number":10,"title":"Add feature","state":"OPEN","author":{"login":"alice"},"labels":[{"name":"api"}],"url":"https://github.com/o/r/pull/10","isDraft":false,"reviewDecision":"APPROVED","headRefName":"feat/add","statusCheckRollup":[{"__typename":"CheckRun","status":"COMPLETED","conclusion":"SUCCESS"}],"createdAt":"2026-06-29T00:00:00Z","assignees":[{"login":"dave"}],"closingIssuesReferences":[{"number":13}]},
				{"number":11,"title":"WIP thing","state":"OPEN","author":{"login":"bob"},"labels":[],"url":"https://github.com/o/r/pull/11","isDraft":true,"reviewDecision":"","headRefName":"wip/thing","statusCheckRollup":[],"createdAt":"2026-07-02T00:00:00Z","assignees":[],"closingIssuesReferences":[]}
			]`)},
		},
	}
	c := newPRs(t, exec)

	result, err := c.Search(context.Background(), sources.SearchParams{Scope: "o/r", Query: "feat"})
	require.NoError(t, err)
	require.Len(t, exec.Calls(), 1)

	call := exec.Calls()[0]
	assert.Equal(t, "gh", call.Cmd)
	assert.Equal(t, []string{
		"pr", "list",
		"--repo", "o/r",
		"--json", "number,title,state,author,labels,url,isDraft,reviewDecision,headRefName,statusCheckRollup,createdAt,assignees,closingIssuesReferences",
		"--limit", "30",
		"--search", "feat",
	}, call.Args)

	require.Len(t, result.Items, 2)
	assert.Equal(t, "10", result.Items[0].ID)
	assert.Equal(t, "Add feature", result.Items[0].Title)
	assert.Equal(t, "#10 · approved", result.Items[0].Subtitle)
	assert.Equal(t, map[string]any{
		"number":             10,
		"title":              "Add feature",
		"state":              "OPEN",
		"url":                "https://github.com/o/r/pull/10",
		"author":             "alice",
		"labels":             []string{"api"},
		"draft":              false,
		"review":             "approved",
		"ci":                 "passing",
		"branch":             "feat/add",
		"age":                "5d",
		"linked_issue":       13,
		"linked_issue_count": 1,
		"assignee":           "dave",
		"assignee_count":     1,
	}, result.Items[0].Fields)

	assert.Equal(t, "#11 · draft", result.Items[1].Subtitle, "draft wins over reviewDecision")
	assert.Equal(t, "draft", result.Items[1].Fields["review"])
	assert.Empty(t, result.Items[1].Fields["ci"], "no checks renders a blank CI cell")
}

func TestCILabel(t *testing.T) {
	tests := []struct {
		name   string
		checks []prCheck
		want   string
	}{
		{"no checks", nil, ""},
		{"all passed", []prCheck{{Status: "COMPLETED", Conclusion: "SUCCESS"}, {State: "SUCCESS"}}, "passing"},
		{"skipped and neutral count as passing", []prCheck{{Status: "COMPLETED", Conclusion: "SKIPPED"}, {Status: "COMPLETED", Conclusion: "NEUTRAL"}}, "passing"},
		{"check run failure wins", []prCheck{{Status: "COMPLETED", Conclusion: "SUCCESS"}, {Status: "COMPLETED", Conclusion: "FAILURE"}}, "failing"},
		{"status context failure wins", []prCheck{{State: "FAILURE"}}, "failing"},
		{"failure beats pending", []prCheck{{Status: "IN_PROGRESS"}, {Status: "COMPLETED", Conclusion: "TIMED_OUT"}}, "failing"},
		{"in progress is pending", []prCheck{{Status: "COMPLETED", Conclusion: "SUCCESS"}, {Status: "IN_PROGRESS"}}, "pending"},
		{"queued is pending", []prCheck{{Status: "QUEUED"}}, "pending"},
		{"status context pending", []prCheck{{State: "PENDING"}}, "pending"},
		{"cancelled is failing", []prCheck{{Status: "COMPLETED", Conclusion: "CANCELLED"}}, "failing"},
		{"stale is pending, not passing", []prCheck{{Status: "COMPLETED", Conclusion: "STALE"}}, "pending"},
		{"completed without conclusion is pending", []prCheck{{Status: "COMPLETED"}}, "pending"},
		{"unknown future conclusion is pending", []prCheck{{Status: "COMPLETED", Conclusion: "SOMETHING_NEW"}}, "pending"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ciLabel(tt.checks))
		})
	}
}

func TestReviewLabel(t *testing.T) {
	tests := []struct {
		name string
		pr   prListItem
		want string
	}{
		{"draft wins", prListItem{IsDraft: true, ReviewDecision: "APPROVED"}, "draft"},
		{"approved", prListItem{ReviewDecision: "APPROVED"}, "approved"},
		{"changes requested", prListItem{ReviewDecision: "CHANGES_REQUESTED"}, "changes requested"},
		{"review required", prListItem{ReviewDecision: "REVIEW_REQUIRED"}, "review required"},
		{"no decision", prListItem{}, "open"},
		{"unknown decision passes through", prListItem{ReviewDecision: "SOMETHING_NEW"}, "SOMETHING_NEW"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, reviewLabel(tt.pr))
		})
	}
}

func TestFetchDetailIssues(t *testing.T) {
	exec := &executiltest.Exec{
		Responses: []executiltest.Response{
			{Out: []byte(`{"number":1,"title":"First issue","body":"issue body text","url":"https://github.com/o/r/issues/1","state":"OPEN"}`)},
		},
	}
	c := newIssues(t, exec)

	detail, err := c.FetchDetail(context.Background(), sources.FetchDetailParams{ID: "1", Scope: "o/r"})
	require.NoError(t, err)
	require.NotNil(t, detail.Markdown)
	assert.Equal(t, "issue body text", detail.Markdown.Content)

	require.Len(t, exec.Calls(), 1)
	assert.Equal(t, []string{
		"issue", "view", "1",
		"--repo", "o/r",
		"--json", "number,title,body,url,state",
	}, exec.Calls()[0].Args)
}
