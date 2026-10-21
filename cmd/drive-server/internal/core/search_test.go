package core_test

import (
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

func TestSearchRanksTitleAndPathAboveBody(t *testing.T) {
	e := newEnv(t, devPolicy())
	p := principalFor(t, e, "dev@example.com")
	mustPut(t, e, p, drive.Shared, "a-body.md", "# Notes\n\nwe talk about kubernetes here, and kubernetes there")
	mustPut(t, e, p, drive.Shared, "z-title.md", "# Kubernetes\n\nshort")

	assert.Equal(t, []string{"z-title.md", "a-body.md"}, searchPaths(t, e, p, "kubernetes", nil))

	mustPut(t, e, p, drive.Shared, "roadmap/z.md", "# Plans\n\nnothing")
	mustPut(t, e, p, drive.Shared, "b-body.md", "# Notes\n\nthe roadmap is here")
	assert.Equal(t, []string{"roadmap/z.md", "b-body.md"}, searchPaths(t, e, p, "roadmap", nil))

	hits, err := e.docs.Search(t.Context(), p, "kubernetes", nil, 10)
	require.NoError(t, err)
	assert.Contains(t, hits[1].Snippet, driveapi.SnippetMatchStart+"kubernetes"+driveapi.SnippetMatchEnd)
	assert.NotEmpty(t, hits[0].Snippet, "a title-only match still has a snippet")
}

func TestSearchPrefixesTheLastTermOnly(t *testing.T) {
	e := newEnv(t, devPolicy())
	p := principalFor(t, e, "dev@example.com")
	mustPut(t, e, p, drive.Shared, "a.md", "# Doc\n\nthe quick brown fox")

	assert.Equal(t, []string{"a.md"}, searchPaths(t, e, p, "quick bro", nil))
	assert.Equal(t, []string{"a.md"}, searchPaths(t, e, p, "qui", nil))
	assert.Empty(t, searchPaths(t, e, p, "qui brown", nil))
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

func TestSearchIndexesHTMLText(t *testing.T) {
	e := newEnv(t, devPolicy())
	p := principalFor(t, e, "dev@example.com")
	mustPut(t, e, p, drive.Shared, "page.html", `<!doctype html><html><head><style>.zebra{color:red}</style>
<script>var secret = "hidden";</script></head>
<body><!-- note --><div class="x">hello &amp; welcome</div></body></html>`)

	assert.Equal(t, []string{"page.html"}, searchPaths(t, e, p, "hello", nil))
	assert.Equal(t, []string{"page.html"}, searchPaths(t, e, p, "welcome", nil))
	for _, q := range []string{"div", "class", "zebra", "secret", "note", "amp"} {
		assert.Empty(t, searchPaths(t, e, p, q, nil), q)
	}

	hits, err := e.docs.Search(t.Context(), p, "hello", nil, 10)
	require.NoError(t, err)
	require.Len(t, hits, 1)
	assert.NotContains(t, hits[0].Snippet, "<")
	assert.Contains(t, hits[0].Snippet, driveapi.SnippetMatchStart+"hello"+driveapi.SnippetMatchEnd)
}
