package ghcli

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/core/kv"
	"github.com/colonyops/hive/internal/data/db"
	"github.com/colonyops/hive/internal/data/stores"
	"github.com/colonyops/hive/internal/sources"
	"github.com/colonyops/hive/pkg/executil/executiltest"
)

// newIssues constructs the issues source for tests. A nil store gets a
// fresh per-test KV (New requires one).
func newIssues(t *testing.T, exec *executiltest.Exec, store kv.KV) *Source {
	t.Helper()
	if store == nil {
		store = newTestKV(t)
	}
	c, err := New(Issues(), exec, store, Options{})
	require.NoError(t, err)
	return c
}

// newPRs constructs the prs source for tests. A nil store gets a fresh
// per-test KV (New requires one).
func newPRs(t *testing.T, exec *executiltest.Exec, store kv.KV) *Source {
	t.Helper()
	if store == nil {
		store = newTestKV(t)
	}
	c, err := New(PRs(), exec, store, Options{})
	require.NoError(t, err)
	return c
}

// newTestKV returns a real SQLite-backed kv.KV in a temp dir, matching the
// store the production wiring passes to New.
func newTestKV(t *testing.T) kv.KV {
	t.Helper()
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	return stores.NewKVStore(database)
}

// emptyIDDriver is a Driver whose Config carries no ID, for New
// validation tests.
type emptyIDDriver struct{ Driver }

func (emptyIDDriver) Config() Config { return Config{} }

func TestNewValidation(t *testing.T) {
	exec := &executiltest.Exec{}

	_, err := New(emptyIDDriver{}, exec, newTestKV(t), Options{})
	require.Error(t, err, "missing id")

	_, err = New(Issues(), exec, nil, Options{})
	require.Error(t, err, "missing kv store")
}

func TestOptionsOverrideDefaults(t *testing.T) {
	exec := &executiltest.Exec{
		Responses: []executiltest.Response{{Out: []byte(`[]`)}},
	}
	c, err := New(Issues(), exec, newTestKV(t), Options{SearchLimit: 5})
	require.NoError(t, err)

	_, err = c.Search(context.Background(), sources.SearchParams{Scope: "o/r"})
	require.NoError(t, err)
	require.Len(t, exec.Calls(), 1)
	assert.Contains(t, exec.Calls()[0].Args, "5", "configured search limit must reach the gh argv")
	assert.NotContains(t, exec.Calls()[0].Args, "30")
}

func TestIssuesManifest(t *testing.T) {
	c := newIssues(t, &executiltest.Exec{}, nil)

	manifest, err := c.Initialize(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "issues", manifest.ID)
	assert.True(t, manifest.Capabilities.FetchDetail)
	assert.False(t, manifest.Picker.HidePreview)
	assert.Equal(t, sources.LayoutModeList, manifest.Picker.Layout)
	assert.Equal(t, sources.SearchModeRemote, manifest.Picker.Search.Mode)
}

func TestPRsManifest(t *testing.T) {
	c := newPRs(t, &executiltest.Exec{}, nil)

	manifest, err := c.Initialize(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "prs", manifest.ID)
	assert.False(t, manifest.Capabilities.FetchDetail, "prs source has no detail view")
	assert.True(t, manifest.Picker.HidePreview)
	assert.Equal(t, sources.LayoutModeTable, manifest.Picker.Layout)
	assert.NotEmpty(t, manifest.Picker.Columns)
}

func TestSearchIssues(t *testing.T) {
	exec := &executiltest.Exec{
		Responses: []executiltest.Response{
			{Out: []byte(`[
				{"number":1,"title":"First issue","state":"OPEN","author":{"login":"alice"},"labels":[{"name":"api"},{"name":"public"}],"url":"https://github.com/o/r/issues/1"},
				{"number":2,"title":"Second issue","state":"CLOSED","author":{"login":"bob"},"labels":[],"url":"https://github.com/o/r/issues/2"}
			]`)},
		},
	}
	c := newIssues(t, exec, nil)

	result, err := c.Search(context.Background(), sources.SearchParams{Scope: "o/r", Query: "bug"})
	require.NoError(t, err)
	require.Len(t, exec.Calls(), 1)

	call := exec.Calls()[0]
	assert.Equal(t, "gh", call.Cmd)
	assert.Equal(t, []string{
		"issue", "list",
		"--repo", "o/r",
		"--json", "number,title,state,author,labels,url",
		"--limit", "30",
		"--search", "bug",
	}, call.Args)

	require.Len(t, result.Items, 2)
	assert.Equal(t, "1", result.Items[0].ID)
	assert.Equal(t, "First issue", result.Items[0].Title)
	assert.Equal(t, "#1 · OPEN", result.Items[0].Subtitle)
	assert.Equal(t, "https://github.com/o/r/issues/1", result.Items[0].URI)
	assert.Equal(t, map[string]any{
		"number": 1,
		"title":  "First issue",
		"state":  "OPEN",
		"url":    "https://github.com/o/r/issues/1",
		"author": "alice",
		"labels": []string{"api", "public"},
	}, result.Items[0].Fields)
}

func TestSearchIgnoresSuccessfulGhStderr(t *testing.T) {
	exec := &executiltest.Exec{
		Responses: []executiltest.Response{
			{
				Out:    []byte(`[{"number":1,"title":"First issue","state":"OPEN","author":{"login":"alice"},"labels":[],"url":"https://github.com/o/r/issues/1"}]`),
				Stderr: []byte("warning: extension update available\n"),
			},
		},
	}
	c := newIssues(t, exec, nil)

	result, err := c.Search(context.Background(), sources.SearchParams{Scope: "o/r"})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "First issue", result.Items[0].Title)
}

func TestSearchIncludesGhStderrOnFailure(t *testing.T) {
	exec := &executiltest.Exec{
		Responses: []executiltest.Response{{Stderr: []byte("authentication required\n"), Err: fmt.Errorf("exit status 1")}},
	}
	c := newIssues(t, exec, nil)

	_, err := c.Search(context.Background(), sources.SearchParams{Scope: "o/r"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exit status 1")
	assert.Contains(t, err.Error(), "authentication required")
}

func TestSearchPRs(t *testing.T) {
	exec := &executiltest.Exec{
		Responses: []executiltest.Response{
			{Out: []byte(`[
				{"number":10,"title":"Add feature","state":"OPEN","author":{"login":"alice"},"labels":[{"name":"api"}],"url":"https://github.com/o/r/pull/10","isDraft":false,"reviewDecision":"APPROVED","headRefName":"feat/add","statusCheckRollup":[{"__typename":"CheckRun","status":"COMPLETED","conclusion":"SUCCESS"}]},
				{"number":11,"title":"WIP thing","state":"OPEN","author":{"login":"bob"},"labels":[],"url":"https://github.com/o/r/pull/11","isDraft":true,"reviewDecision":"","headRefName":"wip/thing","statusCheckRollup":[]}
			]`)},
		},
	}
	c := newPRs(t, exec, nil)

	result, err := c.Search(context.Background(), sources.SearchParams{Scope: "o/r", Query: "feat"})
	require.NoError(t, err)
	require.Len(t, exec.Calls(), 1)

	call := exec.Calls()[0]
	assert.Equal(t, "gh", call.Cmd)
	assert.Equal(t, []string{
		"pr", "list",
		"--repo", "o/r",
		"--json", "number,title,state,author,labels,url,isDraft,reviewDecision,headRefName,statusCheckRollup",
		"--limit", "30",
		"--search", "feat",
	}, call.Args)

	require.Len(t, result.Items, 2)
	assert.Equal(t, "10", result.Items[0].ID)
	assert.Equal(t, "Add feature", result.Items[0].Title)
	assert.Equal(t, "#10 · approved", result.Items[0].Subtitle)
	assert.Equal(t, map[string]any{
		"number": 10,
		"title":  "Add feature",
		"state":  "OPEN",
		"url":    "https://github.com/o/r/pull/10",
		"author": "alice",
		"labels": []string{"api"},
		"draft":  false,
		"review": "approved",
		"ci":     "passing",
		"branch": "feat/add",
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
	c := newIssues(t, exec, nil)

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

func TestFetchDetailUnsupportedForPRs(t *testing.T) {
	exec := &executiltest.Exec{}
	c := newPRs(t, exec, nil)

	_, err := c.FetchDetail(context.Background(), sources.FetchDetailParams{ID: "1", Scope: "o/r"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")
	assert.Empty(t, exec.Calls())
}

func TestAvailableChecksGH(t *testing.T) {
	c := newIssues(t, &executiltest.Exec{}, nil)

	t.Setenv("PATH", t.TempDir())
	assert.False(t, c.Available(context.Background()))
}

func TestSearchRequiresOwnerRepoScope(t *testing.T) {
	cases := []string{"", "no-slash", "too/many/slashes", "/name", "owner/"}

	for _, scope := range cases {
		t.Run(scope, func(t *testing.T) {
			exec := &executiltest.Exec{}
			c := newIssues(t, exec, nil)

			_, err := c.Search(context.Background(), sources.SearchParams{Scope: scope})
			require.Error(t, err)
			assert.Empty(t, exec.Calls(), "no gh call should be made for an invalid scope")
		})
	}
}

func TestSearchGHNonZeroExit(t *testing.T) {
	exec := &executiltest.Exec{
		Responses: []executiltest.Response{
			{Out: []byte(""), Err: fmt.Errorf("exec gh: exit status 1")},
		},
	}
	c := newIssues(t, exec, nil)

	result, err := c.Search(context.Background(), sources.SearchParams{Scope: "o/r"})
	require.Error(t, err)
	assert.Nil(t, result.Items)
}

func TestSearchEmptyResults(t *testing.T) {
	exec := &executiltest.Exec{
		Responses: []executiltest.Response{
			{Out: []byte(`[]`)},
		},
	}
	c := newIssues(t, exec, nil)

	result, err := c.Search(context.Background(), sources.SearchParams{Scope: "o/r"})
	require.NoError(t, err)
	assert.Nil(t, result.Items)
}

func TestSearchMalformedJSON(t *testing.T) {
	exec := &executiltest.Exec{
		Responses: []executiltest.Response{
			{Out: []byte("not json")},
		},
	}
	c := newIssues(t, exec, nil)

	_, err := c.Search(context.Background(), sources.SearchParams{Scope: "o/r"})
	require.Error(t, err)
}

func TestSearchUsesCache(t *testing.T) {
	payload := []byte(`[{"number":1,"title":"First issue","state":"OPEN","author":{"login":"alice"},"labels":[],"url":"https://github.com/o/r/issues/1"}]`)
	exec := &executiltest.Exec{
		Responses: []executiltest.Response{{Out: payload}, {Out: payload}, {Out: payload}},
	}
	c := newIssues(t, exec, newTestKV(t))
	ctx := context.Background()

	first, err := c.Search(ctx, sources.SearchParams{Scope: "o/r", Query: "bug"})
	require.NoError(t, err)
	require.Len(t, exec.Calls(), 1)

	second, err := c.Search(ctx, sources.SearchParams{Scope: "o/r", Query: "bug"})
	require.NoError(t, err)
	assert.Len(t, exec.Calls(), 1, "identical search must be served from cache without a second gh call")
	assert.Equal(t, first.Items, second.Items, "cached raw output must parse to identical items (including Field types)")

	_, err = c.Search(ctx, sources.SearchParams{Scope: "o/r", Query: "feature"})
	require.NoError(t, err)
	assert.Len(t, exec.Calls(), 2, "a different query must miss the cache")

	_, err = c.Search(ctx, sources.SearchParams{Scope: "o/other", Query: "bug"})
	require.NoError(t, err)
	assert.Len(t, exec.Calls(), 3, "a different scope must miss the cache")
}

func TestCacheIsolatedPerSource(t *testing.T) {
	store := newTestKV(t)
	issuesPayload := []byte(`[{"number":1,"title":"An issue","state":"OPEN","author":{"login":"a"},"labels":[],"url":"u"}]`)
	prsPayload := []byte(`[{"number":2,"title":"A PR","state":"OPEN","author":{"login":"b"},"labels":[],"url":"u","isDraft":false,"reviewDecision":"","headRefName":"x"}]`)

	issues := newIssues(t, &executiltest.Exec{Responses: []executiltest.Response{{Out: issuesPayload}}}, store)
	prs := newPRs(t, &executiltest.Exec{Responses: []executiltest.Response{{Out: prsPayload}}}, store)
	ctx := context.Background()

	issuesResult, err := issues.Search(ctx, sources.SearchParams{Scope: "o/r"})
	require.NoError(t, err)
	prsResult, err := prs.Search(ctx, sources.SearchParams{Scope: "o/r"})
	require.NoError(t, err)

	assert.Equal(t, "An issue", issuesResult.Items[0].Title)
	assert.Equal(t, "A PR", prsResult.Items[0].Title, "same scope+query must not collide across source caches")
}

func TestFetchDetailRejectsNonNumericID(t *testing.T) {
	for _, id := range []string{"--web", "-1", "0", "+1x"} {
		t.Run(id, func(t *testing.T) {
			exec := &executiltest.Exec{}
			c := newIssues(t, exec, nil)

			_, err := c.FetchDetail(context.Background(), sources.FetchDetailParams{ID: id, Scope: "o/r"})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid id")
			assert.Empty(t, exec.Calls(), "no gh call should be made for an invalid id")
		})
	}
}
