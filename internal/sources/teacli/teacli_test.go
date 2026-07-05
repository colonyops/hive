package teacli

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

func newSource(t *testing.T, driver cliengine.Driver, exec *executiltest.Exec) *cliengine.Source {
	t.Helper()
	c, err := cliengine.New(driver, exec, newTestKV(t), cliengine.Options{})
	require.NoError(t, err)
	return c
}

func TestIssuesManifestNoDetail(t *testing.T) {
	c := newSource(t, Issues(), &executiltest.Exec{})
	m, err := c.Initialize(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "issues", m.ID)
	assert.Equal(t, "Issues", m.DisplayName)
	assert.False(t, m.Capabilities.FetchDetail, "tea has no single-issue view; body rides in Fields instead")
}

func TestSearchIssues(t *testing.T) {
	fixClock(t, time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC))
	// Shape mirrors real `tea issues list --output json` output: every value
	// is a string, labels/assignees are comma-joined.
	exec := &executiltest.Exec{Responses: []executiltest.Response{{Out: []byte(`[
		{"index":"413","title":"Fix Renovate config","state":"open","author":"Renovate Bot","url":"https://gitea.example.com/o/r/issues/413","created":"2026-07-01T00:00:00Z","assignees":"alice, bob","labels":"bug, infra","body":"the body"},
		{"index":"274","title":"Grafana alerting","state":"closed","author":"hay-kot","url":"https://gitea.example.com/o/r/issues/274","created":"2026-07-03T00:00:00Z","assignees":"","labels":"","body":""}
	]`)}}}
	c := newSource(t, Issues(), exec)

	result, err := c.Search(context.Background(), sources.SearchParams{Scope: "o/r", Query: "renovate", Dir: "/repo"})
	require.NoError(t, err)
	require.Len(t, exec.Calls(), 1)

	call := exec.Calls()[0]
	assert.Equal(t, "tea", call.Cmd)
	assert.Equal(t, "/repo", call.Dir, "tea must run in the repo dir to resolve its login")
	assert.Equal(t, []string{
		"issues", "list",
		"--repo", "o/r",
		"--output", "json",
		"--fields", "index,title,state,author,url,created,assignees,labels,body",
		"--limit", "30",
		"--keyword", "renovate",
	}, call.Args)

	require.Len(t, result.Items, 2)
	assert.Equal(t, "413", result.Items[0].ID)
	assert.Equal(t, "Fix Renovate config", result.Items[0].Title)
	assert.Equal(t, "#413 · open", result.Items[0].Subtitle)
	assert.Equal(t, "https://gitea.example.com/o/r/issues/413", result.Items[0].URI)
	assert.Equal(t, map[string]any{
		"number":         413,
		"title":          "Fix Renovate config",
		"state":          "open",
		"url":            "https://gitea.example.com/o/r/issues/413",
		"author":         "Renovate Bot",
		"labels":         []string{"bug", "infra"},
		"age":            "3d",
		"assignee":       "alice",
		"assignee_count": 2,
		"body":           "the body",
	}, result.Items[0].Fields)

	assert.Equal(t, "1d", result.Items[1].Fields["age"])
	assert.Empty(t, result.Items[1].Fields["assignee"])
	assert.Equal(t, 0, result.Items[1].Fields["assignee_count"])
	assert.Empty(t, result.Items[1].Fields["labels"])
}

func TestSearchIssuesOmitsKeywordWhenEmpty(t *testing.T) {
	exec := &executiltest.Exec{Responses: []executiltest.Response{{Out: []byte(`[]`)}}}
	c := newSource(t, Issues(), exec)

	_, err := c.Search(context.Background(), sources.SearchParams{Scope: "o/r"})
	require.NoError(t, err)
	require.Len(t, exec.Calls(), 1)
	assert.NotContains(t, exec.Calls()[0].Args, "--keyword")
}

func TestSearchPRs(t *testing.T) {
	fixClock(t, time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC))
	exec := &executiltest.Exec{Responses: []executiltest.Response{{Out: []byte(`[
		{"index":"428","title":"Upgrade immich","state":"open","author":"hay-kot","url":"https://gitea.example.com/o/r/pulls/428","created":"2026-06-29T00:00:00Z","labels":"deps","head":"chore/immich-v3","base":"main","mergeable":"true"},
		{"index":"427","title":"Update sonarr","state":"open","author":"Renovate Bot","url":"https://gitea.example.com/o/r/pulls/427","created":"2026-07-02T00:00:00Z","labels":"","head":"renovate/sonarr","base":"main","mergeable":"false"},
		{"index":"426","title":"Merged one","state":"merged","author":"hay-kot","url":"https://gitea.example.com/o/r/pulls/426","created":"2026-07-01T00:00:00Z","labels":"","head":"feat/x","base":"main","mergeable":"false"}
	]`)}}}
	c := newSource(t, PRs(), exec)

	result, err := c.Search(context.Background(), sources.SearchParams{Scope: "o/r", Query: "immich"})
	require.NoError(t, err)

	call := exec.Calls()[0]
	assert.Equal(t, "tea", call.Cmd)
	assert.Equal(t, []string{
		"pulls", "list",
		"--repo", "o/r",
		"--output", "json",
		"--fields", "index,title,state,author,url,created,labels,head,base,mergeable",
		"--limit", "30",
		"--keyword", "immich",
	}, call.Args)

	require.Len(t, result.Items, 3)
	assert.Equal(t, "428", result.Items[0].ID)
	assert.Equal(t, "#428 · open", result.Items[0].Subtitle)
	assert.Equal(t, map[string]any{
		"number": 428,
		"title":  "Upgrade immich",
		"state":  "open",
		"url":    "https://gitea.example.com/o/r/pulls/428",
		"author": "hay-kot",
		"labels": []string{"deps"},
		"review": "open",
		"branch": "chore/immich-v3",
		"base":   "main",
		"age":    "5d",
	}, result.Items[0].Fields)

	assert.Equal(t, "conflict", result.Items[1].Fields["review"], "open + unmergeable reads as conflict")
	assert.Equal(t, "merged", result.Items[2].Fields["review"])
}

func TestReviewLabel(t *testing.T) {
	tests := []struct {
		name string
		pr   teaPull
		want string
	}{
		{"merged", teaPull{State: "merged"}, "merged"},
		{"closed", teaPull{State: "closed"}, "closed"},
		{"open mergeable", teaPull{State: "open", Mergeable: "true"}, "open"},
		{"open unmergeable", teaPull{State: "open", Mergeable: "false"}, "conflict"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, reviewLabel(tt.pr))
		})
	}
}

func TestSplitCSV(t *testing.T) {
	assert.Nil(t, splitCSV(""))
	assert.Nil(t, splitCSV("   "))
	assert.Equal(t, []string{"a", "b", "c"}, splitCSV("a, b ,c"))
	assert.Equal(t, []string{"a", "b"}, splitCSV("a,,b,"))
}

func TestTeaAgeInvalid(t *testing.T) {
	assert.Empty(t, teaAge(""))
	assert.Empty(t, teaAge("not-a-date"))
}
