package cliengine_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/core/kv"
	"github.com/colonyops/hive/internal/data/db"
	"github.com/colonyops/hive/internal/data/stores"
	"github.com/colonyops/hive/internal/sources"
	"github.com/colonyops/hive/internal/sources/cliengine"
	"github.com/colonyops/hive/pkg/executil/executiltest"
)

// stubDriver is a minimal cliengine.Driver for engine-behavior tests. It
// echoes a fixed argv and decodes a simple {id,title} JSON list.
type stubDriver struct {
	id     string
	binary string
}

func (d stubDriver) Config() cliengine.Config {
	return cliengine.Config{ID: d.id, DisplayName: d.id, Binary: d.binary}
}

func (stubDriver) ListArgs(scope, query string, limit int) []string {
	args := []string{"list", "--repo", scope, "--limit", strconv.Itoa(limit)}
	if query != "" {
		args = append(args, "--query", query)
	}
	return args
}

func (stubDriver) ParseList(out []byte) ([]sources.Item, error) {
	entries, err := cliengine.DecodeList[struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}](out)
	if err != nil {
		return nil, err
	}
	items := make([]sources.Item, 0, len(entries))
	for _, e := range entries {
		items = append(items, sources.Item{ID: e.ID, Title: e.Title})
	}
	return items, nil
}

// stubDetailDriver adds a detail view to stubDriver.
type stubDetailDriver struct{ stubDriver }

func (stubDetailDriver) DetailArgs(scope, id string) []string {
	return []string{"view", id, "--repo", scope}
}

func (stubDetailDriver) ParseDetail(out []byte) (sources.Detail, error) {
	return sources.Detail{Markdown: &sources.MarkdownDetail{Content: string(out)}}, nil
}

func newTestKV(t *testing.T) kv.KV {
	t.Helper()
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	return stores.NewKVStore(database)
}

func newStub(t *testing.T, exec cliengine.Executor, store kv.KV, opts cliengine.Options) *cliengine.Source {
	t.Helper()
	if store == nil {
		store = newTestKV(t)
	}
	c, err := cliengine.New(stubDriver{id: "issues", binary: "stub"}, exec, store, opts)
	require.NoError(t, err)
	return c
}

func TestNewValidation(t *testing.T) {
	exec := &executiltest.Exec{}

	_, err := cliengine.New(stubDriver{binary: "stub"}, exec, newTestKV(t), cliengine.Options{})
	require.Error(t, err, "missing id")

	_, err = cliengine.New(stubDriver{id: "issues"}, exec, newTestKV(t), cliengine.Options{})
	require.Error(t, err, "missing binary")

	_, err = cliengine.New(stubDriver{id: "issues", binary: "stub"}, exec, nil, cliengine.Options{})
	require.Error(t, err, "missing kv store")
}

func TestOptionsOverrideDefaults(t *testing.T) {
	exec := &executiltest.Exec{Responses: []executiltest.Response{{Out: []byte(`[]`)}}}
	c := newStub(t, exec, nil, cliengine.Options{SearchLimit: 5})

	_, err := c.Search(context.Background(), sources.SearchParams{Scope: "o/r"})
	require.NoError(t, err)
	require.Len(t, exec.Calls(), 1)
	assert.Equal(t, "stub", exec.Calls()[0].Cmd)
	assert.Contains(t, exec.Calls()[0].Args, "5", "configured search limit must reach the argv")
	assert.NotContains(t, exec.Calls()[0].Args, "30")
}

func TestManifestReflectsDetailCapability(t *testing.T) {
	store := newTestKV(t)

	plain, err := cliengine.New(stubDriver{id: "prs", binary: "stub"}, &executiltest.Exec{}, store, cliengine.Options{})
	require.NoError(t, err)
	m, err := plain.Initialize(context.Background())
	require.NoError(t, err)
	assert.False(t, m.Capabilities.FetchDetail)

	detail, err := cliengine.New(stubDetailDriver{stubDriver{id: "issues", binary: "stub"}}, &executiltest.Exec{}, store, cliengine.Options{})
	require.NoError(t, err)
	m, err = detail.Initialize(context.Background())
	require.NoError(t, err)
	assert.True(t, m.Capabilities.FetchDetail)
}

func TestSearchPassesDirToExecutor(t *testing.T) {
	exec := &executiltest.Exec{Responses: []executiltest.Response{{Out: []byte(`[]`)}}}
	c := newStub(t, exec, nil, cliengine.Options{})

	_, err := c.Search(context.Background(), sources.SearchParams{Scope: "o/r", Dir: "/repo/path"})
	require.NoError(t, err)
	require.Len(t, exec.Calls(), 1)
	assert.Equal(t, "/repo/path", exec.Calls()[0].Dir, "the working dir must reach the executor")
}

func TestSearchIgnoresSuccessfulStderr(t *testing.T) {
	exec := &executiltest.Exec{Responses: []executiltest.Response{{
		Out:    []byte(`[{"id":"1","title":"First"}]`),
		Stderr: []byte("warning: update available\n"),
	}}}
	c := newStub(t, exec, nil, cliengine.Options{})

	result, err := c.Search(context.Background(), sources.SearchParams{Scope: "o/r"})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
}

func TestSearchIncludesStderrOnFailure(t *testing.T) {
	exec := &executiltest.Exec{Responses: []executiltest.Response{{
		Stderr: []byte("authentication required\n"), Err: fmt.Errorf("exit status 1"),
	}}}
	c := newStub(t, exec, nil, cliengine.Options{})

	_, err := c.Search(context.Background(), sources.SearchParams{Scope: "o/r"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exit status 1")
	assert.Contains(t, err.Error(), "authentication required")
}

func TestSearchRequiresOwnerRepoScope(t *testing.T) {
	for _, scope := range []string{"", "no-slash", "too/many/slashes", "/name", "owner/"} {
		t.Run(scope, func(t *testing.T) {
			exec := &executiltest.Exec{}
			c := newStub(t, exec, nil, cliengine.Options{})

			_, err := c.Search(context.Background(), sources.SearchParams{Scope: scope})
			require.Error(t, err)
			assert.Empty(t, exec.Calls(), "no call should be made for an invalid scope")
		})
	}
}

func TestSearchMalformedJSON(t *testing.T) {
	exec := &executiltest.Exec{Responses: []executiltest.Response{{Out: []byte("not json")}}}
	c := newStub(t, exec, nil, cliengine.Options{})

	_, err := c.Search(context.Background(), sources.SearchParams{Scope: "o/r"})
	require.Error(t, err)
}

func TestSearchEmptyResults(t *testing.T) {
	exec := &executiltest.Exec{Responses: []executiltest.Response{{Out: []byte(`[]`)}}}
	c := newStub(t, exec, nil, cliengine.Options{})

	result, err := c.Search(context.Background(), sources.SearchParams{Scope: "o/r"})
	require.NoError(t, err)
	assert.Empty(t, result.Items)
}

func TestSearchCacheKeyedByScopeDirQuery(t *testing.T) {
	payload := []byte(`[{"id":"1","title":"First"}]`)
	exec := &executiltest.Exec{Responses: []executiltest.Response{
		{Out: payload}, {Out: payload}, {Out: payload}, {Out: payload},
	}}
	c := newStub(t, exec, newTestKV(t), cliengine.Options{})
	ctx := context.Background()

	_, err := c.Search(ctx, sources.SearchParams{Scope: "o/r", Query: "bug", Dir: "/a"})
	require.NoError(t, err)
	require.Len(t, exec.Calls(), 1)

	_, err = c.Search(ctx, sources.SearchParams{Scope: "o/r", Query: "bug", Dir: "/a"})
	require.NoError(t, err)
	assert.Len(t, exec.Calls(), 1, "identical search is served from cache")

	_, err = c.Search(ctx, sources.SearchParams{Scope: "o/r", Query: "feature", Dir: "/a"})
	require.NoError(t, err)
	assert.Len(t, exec.Calls(), 2, "a different query misses the cache")

	_, err = c.Search(ctx, sources.SearchParams{Scope: "o/r", Query: "bug", Dir: "/b"})
	require.NoError(t, err)
	assert.Len(t, exec.Calls(), 3, "a different dir misses the cache")

	_, err = c.Search(ctx, sources.SearchParams{Scope: "o/other", Query: "bug", Dir: "/a"})
	require.NoError(t, err)
	assert.Len(t, exec.Calls(), 4, "a different scope misses the cache")
}

func TestCacheIsolatedPerBinary(t *testing.T) {
	store := newTestKV(t)
	ghExec := &executiltest.Exec{Responses: []executiltest.Response{{Out: []byte(`[{"id":"1","title":"gh item"}]`)}}}
	teaExec := &executiltest.Exec{Responses: []executiltest.Response{{Out: []byte(`[{"id":"2","title":"tea item"}]`)}}}

	gh, err := cliengine.New(stubDriver{id: "issues", binary: "gh"}, ghExec, store, cliengine.Options{})
	require.NoError(t, err)
	tea, err := cliengine.New(stubDriver{id: "issues", binary: "tea"}, teaExec, store, cliengine.Options{})
	require.NoError(t, err)
	ctx := context.Background()

	ghResult, err := gh.Search(ctx, sources.SearchParams{Scope: "o/r"})
	require.NoError(t, err)
	teaResult, err := tea.Search(ctx, sources.SearchParams{Scope: "o/r"})
	require.NoError(t, err)

	assert.Equal(t, "gh item", ghResult.Items[0].Title)
	assert.Equal(t, "tea item", teaResult.Items[0].Title, "same id+scope must not collide across binaries")
}

func TestFetchDetailUnsupported(t *testing.T) {
	exec := &executiltest.Exec{}
	c := newStub(t, exec, nil, cliengine.Options{}) // plain stubDriver, no detail

	_, err := c.FetchDetail(context.Background(), sources.FetchDetailParams{ID: "1", Scope: "o/r"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")
	assert.Empty(t, exec.Calls())
}

func TestFetchDetailRejectsNonNumericID(t *testing.T) {
	for _, id := range []string{"", "--web", "-1", "0", "+1x"} {
		t.Run(id, func(t *testing.T) {
			exec := &executiltest.Exec{}
			c, err := cliengine.New(stubDetailDriver{stubDriver{id: "issues", binary: "stub"}}, exec, newTestKV(t), cliengine.Options{})
			require.NoError(t, err)

			_, err = c.FetchDetail(context.Background(), sources.FetchDetailParams{ID: id, Scope: "o/r"})
			require.Error(t, err)
			assert.Empty(t, exec.Calls(), "no call should be made for an invalid id")
		})
	}
}

func TestFetchDetailSuccess(t *testing.T) {
	exec := &executiltest.Exec{Responses: []executiltest.Response{{Out: []byte("body markdown")}}}
	c, err := cliengine.New(stubDetailDriver{stubDriver{id: "issues", binary: "stub"}}, exec, newTestKV(t), cliengine.Options{})
	require.NoError(t, err)

	detail, err := c.FetchDetail(context.Background(), sources.FetchDetailParams{ID: "7", Scope: "o/r", Dir: "/repo"})
	require.NoError(t, err)
	require.NotNil(t, detail.Markdown)
	assert.Equal(t, "body markdown", detail.Markdown.Content)
	require.Len(t, exec.Calls(), 1)
	assert.Equal(t, []string{"view", "7", "--repo", "o/r"}, exec.Calls()[0].Args)
	assert.Equal(t, "/repo", exec.Calls()[0].Dir)
}

func TestAvailableChecksBinary(t *testing.T) {
	c := newStub(t, &executiltest.Exec{}, nil, cliengine.Options{})
	t.Setenv("PATH", t.TempDir())
	assert.False(t, c.Available(context.Background()), "stub binary is not on PATH")
}
