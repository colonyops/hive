package core_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/platform/driveapi"
)

func mustPut(t *testing.T, e *env, p core.Principal, space drive.Space, path, body string) {
	t.Helper()
	_, err := e.docs.Put(t.Context(), p, space, path, []byte(body))
	require.NoError(t, err)
}

func searchPaths(t *testing.T, e *env, p core.Principal, q string, space *drive.Space) []string {
	t.Helper()
	hits, err := e.docs.Search(t.Context(), p, q, space, 50)
	require.NoError(t, err)
	paths := make([]string, len(hits))
	for i, h := range hits {
		paths[i] = h.Document.Path
	}
	return paths
}

func TestPutFillsTitle(t *testing.T) {
	e := newEnv(t, devPolicy())
	p := principalFor(t, e, "dev@example.com")
	doc, err := e.docs.Put(t.Context(), p, drive.Shared, "a.md", []byte("# Hello\n"))
	require.NoError(t, err)
	assert.Equal(t, "Hello", doc.Title)

	docs, err := e.docs.List(t.Context(), p, drive.Shared)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "Hello", docs[0].Title)
}

func TestSearchRanksTitleAboveBody(t *testing.T) {
	e := newEnv(t, devPolicy())
	p := principalFor(t, e, "dev@example.com")
	mustPut(t, e, p, drive.Shared, "a-body.md", "# Notes\n\nwe talk about kubernetes here, and kubernetes there")
	mustPut(t, e, p, drive.Shared, "z-title.md", "# Kubernetes\n\nshort")

	assert.Equal(t, []string{"z-title.md", "a-body.md"}, searchPaths(t, e, p, "kubernetes", nil))
}

func TestSearchMatchesPathAboveBody(t *testing.T) {
	e := newEnv(t, devPolicy())
	p := principalFor(t, e, "dev@example.com")
	mustPut(t, e, p, drive.Shared, "a-body.md", "# Notes\n\nthe roadmap is here")
	mustPut(t, e, p, drive.Shared, "roadmap/z.md", "# Plans\n\nnothing")

	assert.Equal(t, []string{"roadmap/z.md", "a-body.md"}, searchPaths(t, e, p, "roadmap", nil))
}

func TestSearchPrefixesTheLastTermOnly(t *testing.T) {
	e := newEnv(t, devPolicy())
	p := principalFor(t, e, "dev@example.com")
	mustPut(t, e, p, drive.Shared, "a.md", "# Doc\n\nthe quick brown fox")

	assert.Equal(t, []string{"a.md"}, searchPaths(t, e, p, "quick bro", nil))
	assert.Equal(t, []string{"a.md"}, searchPaths(t, e, p, "qui", nil))
	assert.Empty(t, searchPaths(t, e, p, "qui brown", nil))
}

func TestSearchSnippetMarksMatches(t *testing.T) {
	e := newEnv(t, devPolicy())
	p := principalFor(t, e, "dev@example.com")
	mustPut(t, e, p, drive.Shared, "a.md", "# Doc\n\nthe quick brown fox jumps")

	hits, err := e.docs.Search(t.Context(), p, "brown", nil, 10)
	require.NoError(t, err)
	require.Len(t, hits, 1)
	assert.Contains(t, hits[0].Snippet, driveapi.SnippetMatchStart+"brown"+driveapi.SnippetMatchEnd)
	assert.Equal(t, "Doc", hits[0].Document.Title)
}

func TestSearchTitleOnlyMatchStillHasSnippet(t *testing.T) {
	e := newEnv(t, devPolicy())
	p := principalFor(t, e, "dev@example.com")
	mustPut(t, e, p, drive.Shared, "a.md", "# Zebra\n\nunrelated text")

	hits, err := e.docs.Search(t.Context(), p, "zebra", nil, 10)
	require.NoError(t, err)
	require.Len(t, hits, 1)
	assert.NotEmpty(t, hits[0].Snippet)
}

func TestSearchHostileInputDoesNotError(t *testing.T) {
	e := newEnv(t, devPolicy())
	p := principalFor(t, e, "dev@example.com")
	mustPut(t, e, p, drive.Shared, "a.md", "# Doc\n\nand or not near")

	for _, q := range []string{`"`, `*`, `AND`, `-`, `(`, `a AND`, `"unterminated`, `foo*`, `NEAR(a b)`, `col:x`, `-x (y`, `^a`, "\x00", `'; DROP TABLE documents; --`} {
		_, err := e.docs.Search(t.Context(), p, q, nil, 10)
		require.NoError(t, err, q)
	}
	assert.Equal(t, []string{"a.md"}, searchPaths(t, e, p, "AND", nil))
}

func TestSearchRejectsBlankText(t *testing.T) {
	e := newEnv(t, devPolicy())
	p := principalFor(t, e, "dev@example.com")
	for _, q := range []string{"", "   ", "\t\n"} {
		_, err := e.docs.Search(t.Context(), p, q, nil, 10)
		requireKind(t, err, core.KindInvalid)
	}
}

func TestSearchFollowsReuploadAndDelete(t *testing.T) {
	e := newEnv(t, devPolicy())
	p := principalFor(t, e, "dev@example.com")
	mustPut(t, e, p, drive.Shared, "a.md", "# Alpha\n\nfirst version")
	assert.Equal(t, []string{"a.md"}, searchPaths(t, e, p, "alpha", nil))

	mustPut(t, e, p, drive.Shared, "a.md", "# Beta\n\nsecond version")
	assert.Empty(t, searchPaths(t, e, p, "alpha", nil))
	assert.Empty(t, searchPaths(t, e, p, "first", nil))
	assert.Equal(t, []string{"a.md"}, searchPaths(t, e, p, "beta", nil))

	require.NoError(t, e.docs.Delete(t.Context(), p, drive.Shared, "a.md"))
	assert.Empty(t, searchPaths(t, e, p, "beta", nil))
	assert.Empty(t, searchPaths(t, e, p, "second", nil))
}

func TestSearchSpaceFilterAndLimit(t *testing.T) {
	e := newEnv(t, devPolicy())
	p := principalFor(t, e, "dev@example.com")
	ws, err := drive.WorkspaceSpace("chats")
	require.NoError(t, err)
	mustPut(t, e, p, drive.Shared, "s.md", "# S\n\nshared needle")
	mustPut(t, e, p, ws, "w.md", "# W\n\nworkspace needle")

	assert.ElementsMatch(t, []string{"s.md", "w.md"}, searchPaths(t, e, p, "needle", nil))
	assert.Equal(t, []string{"w.md"}, searchPaths(t, e, p, "needle", &ws))

	hits, err := e.docs.Search(t.Context(), p, "needle", nil, 1)
	require.NoError(t, err)
	assert.Len(t, hits, 1)
}

func TestSearchIndexesHTML(t *testing.T) {
	e := newEnv(t, devPolicy())
	p := principalFor(t, e, "dev@example.com")
	mustPut(t, e, p, drive.Shared, "r.html", "<html><title>Quarterly</title><body>"+strings.Repeat("x ", 10)+"revenue</body></html>")

	assert.Equal(t, []string{"r.html"}, searchPaths(t, e, p, "quarterly", nil))
	assert.Equal(t, []string{"r.html"}, searchPaths(t, e, p, "revenue", nil))
}
