package canvas

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testStore returns a store over a temp root whose clock ticks one
// millisecond per call, so every mutation gets a distinct, ordered stamp.
func testStore(t *testing.T) *Store {
	t.Helper()
	s := NewStore(t.TempDir())
	var tick int64
	s.now = func() time.Time {
		tick++
		return time.UnixMilli(tick)
	}
	return s
}

func TestLoadAbsentIsNotAnError(t *testing.T) {
	s := testStore(t)
	_, ok, err := s.Load("ws", "plan")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestUpsertAppendsThenUpdatesInPlace(t *testing.T) {
	s := testStore(t)

	c, err := s.Upsert("ws", "plan", 1, "The Plan", "", Block{ID: "plan", Kind: KindMarkdown, Body: "v1"})
	require.NoError(t, err)
	require.Len(t, c.Blocks, 1)
	created := c.Blocks[0].CreatedAt
	assert.NotZero(t, created)
	assert.NotZero(t, c.CreatedAt)
	assert.Equal(t, int64(1), c.Session)
	assert.Equal(t, "The Plan", c.Title)

	c, err = s.Upsert("ws", "plan", 1, "", "", Block{ID: "result", Kind: KindLink, Title: "PR", URL: "https://example.com"})
	require.NoError(t, err)
	require.Len(t, c.Blocks, 2)
	assert.Equal(t, "The Plan", c.Title, "an empty title leaves the stored one")

	c, err = s.Upsert("ws", "plan", 2, "Revised", "", Block{ID: "plan", Kind: KindMarkdown, Body: "v2"})
	require.NoError(t, err)
	require.Len(t, c.Blocks, 2)
	assert.Equal(t, "plan", c.Blocks[0].ID, "an updated block keeps its position")
	assert.Equal(t, "v2", c.Blocks[0].Body)
	assert.Equal(t, created, c.Blocks[0].CreatedAt, "an updated block keeps its CreatedAt")
	assert.Greater(t, c.Blocks[0].UpdatedAt, created)
	assert.Equal(t, int64(1), c.Session, "the creating session never changes")
	assert.Equal(t, "Revised", c.Title)

	reloaded, ok, err := s.Load("ws", "plan")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, c, reloaded)
}

func TestUpsertBeforeAnchorInsertsAndMoves(t *testing.T) {
	s := testStore(t)
	_, err := s.Upsert("ws", "plan", 1, "", "", Block{ID: "a", Kind: KindMarkdown, Body: "a"})
	require.NoError(t, err)
	c, err := s.Upsert("ws", "plan", 1, "", "", Block{ID: "b", Kind: KindMarkdown, Body: "b"})
	require.NoError(t, err)
	created := c.Blocks[1].CreatedAt

	c, err = s.Upsert("ws", "plan", 1, "", "a", Block{ID: "intro", Kind: KindMarkdown, Body: "i"})
	require.NoError(t, err)
	assert.Equal(t, []string{"intro", "a", "b"}, blockIDs(c), "a new block inserts before the anchor")

	c, err = s.Upsert("ws", "plan", 1, "", "intro", Block{ID: "b", Kind: KindMarkdown, Body: "b2"})
	require.NoError(t, err)
	assert.Equal(t, []string{"b", "intro", "a"}, blockIDs(c), "a reused id moves before the anchor")
	assert.Equal(t, created, c.Blocks[0].CreatedAt, "a moved block keeps its CreatedAt")
	assert.Equal(t, "b2", c.Blocks[0].Body)

	_, err = s.Upsert("ws", "plan", 1, "", "ghost", Block{ID: "x", Kind: KindMarkdown, Body: "x"})
	require.ErrorIs(t, err, ErrAnchorNotFound)
	c, _, err = s.Load("ws", "plan")
	require.NoError(t, err)
	assert.Equal(t, []string{"b", "intro", "a"}, blockIDs(c), "a rejected anchor writes nothing")

	_, err = s.Upsert("ws", "plan", 1, "", "b", Block{ID: "b", Kind: KindMarkdown, Body: "b3"})
	require.ErrorIs(t, err, ErrAnchorNotFound, "a block cannot anchor before itself")
}

func TestUpsertBatchIsOneWrite(t *testing.T) {
	s := testStore(t)
	c, err := s.Upsert("ws", "plan", 1, "The Plan", "",
		Block{ID: "a", Kind: KindMarkdown, Body: "a"},
		Block{ID: "b", Kind: KindMarkdown, Body: "b"},
		Block{ID: "c", Kind: KindLink, Title: "PR", URL: "https://example.com"},
	)
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, blockIDs(c), "a batch applies in order")
	assert.Equal(t, c.Blocks[0].UpdatedAt, c.Blocks[2].UpdatedAt, "one write, one stamp")
}

func blockIDs(c Canvas) []string {
	ids := make([]string, 0, len(c.Blocks))
	for _, b := range c.Blocks {
		ids = append(ids, b.ID)
	}
	return ids
}

func TestRemove(t *testing.T) {
	s := testStore(t)
	_, err := s.Upsert("ws", "plan", 1, "", "", Block{ID: "a", Kind: KindMarkdown, Body: "x"})
	require.NoError(t, err)

	c, removed, err := s.Remove("ws", "plan", "a")
	require.NoError(t, err)
	assert.True(t, removed)
	assert.Empty(t, c.Blocks)

	_, removed, err = s.Remove("ws", "plan", "a")
	require.NoError(t, err)
	assert.False(t, removed)

	_, _, err = s.Remove("ws", "other", "a")
	require.ErrorIs(t, err, ErrNotFound, "a never-written canvas is not found")
}

func TestClearKeepsTheCanvas(t *testing.T) {
	s := testStore(t)
	first, err := s.Upsert("ws", "plan", 1, "The Plan", "", Block{ID: "a", Kind: KindMarkdown, Body: "x"})
	require.NoError(t, err)

	c, err := s.Clear("ws", "plan")
	require.NoError(t, err)
	assert.Empty(t, c.Blocks)
	assert.Equal(t, first.CreatedAt, c.CreatedAt, "clear keeps the canvas's CreatedAt")
	assert.Equal(t, "The Plan", c.Title, "clear keeps the title")

	reloaded, ok, err := s.Load("ws", "plan")
	require.NoError(t, err)
	require.True(t, ok, "the file survives a clear")
	assert.Empty(t, reloaded.Blocks)

	_, err = s.Clear("ws", "other")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestListOrdersByUpdatedAtDesc(t *testing.T) {
	s := testStore(t)
	_, err := s.Upsert("ws", "plan", 1, "The Plan", "", Block{ID: "a", Kind: KindMarkdown, Body: "x"})
	require.NoError(t, err)
	_, err = s.Upsert("ws", "report", 2, "", "", Block{ID: "a", Kind: KindMarkdown, Body: "x"})
	require.NoError(t, err)
	_, err = s.Upsert("ws", "plan", 1, "", "", Block{ID: "b", Kind: KindMarkdown, Body: "y"})
	require.NoError(t, err)

	metas, err := s.List("ws")
	require.NoError(t, err)
	require.Len(t, metas, 2)
	assert.Equal(t, "plan", metas[0].Name, "the most recently updated canvas lists first")
	assert.Equal(t, "The Plan", metas[0].Title)
	assert.Equal(t, int64(1), metas[0].Session)
	assert.Equal(t, 2, metas[0].BlockCount)

	empty, err := s.List("other")
	require.NoError(t, err)
	assert.Empty(t, empty)
}

func TestDeleteReportsExistence(t *testing.T) {
	s := testStore(t)
	_, err := s.Upsert("ws", "plan", 1, "", "", Block{ID: "a", Kind: KindMarkdown, Body: "x"})
	require.NoError(t, err)

	existed, err := s.Delete("ws", "plan")
	require.NoError(t, err)
	assert.True(t, existed)

	existed, err = s.Delete("ws", "plan")
	require.NoError(t, err)
	assert.False(t, existed)

	_, ok, err := s.Load("ws", "plan")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestMarkdownRendersTitleBlocksAndLinks(t *testing.T) {
	c := Canvas{
		Title: "The Plan",
		Blocks: []Block{
			{ID: "intro", Kind: KindMarkdown, Title: "Intro", Body: "hello"},
			{ID: "body", Kind: KindMarkdown, Body: "world"},
			{ID: "pr", Kind: KindLink, Title: "The PR", URL: "https://example.com/pr/1"},
		},
	}
	want := "# The Plan\n\n## Intro\n\nhello\n\nworld\n\n[The PR](https://example.com/pr/1)\n"
	assert.Equal(t, want, Markdown(c))

	assert.Equal(t, "hello\n", Markdown(Canvas{Blocks: []Block{{Kind: KindMarkdown, Body: "hello"}}}),
		"no canvas title means no heading")
}

// An export leaves the app, so it carries the sanitized markup rather than
// what the agent wrote — the file is opened elsewhere, by something with no
// policy of its own.
func TestMarkdownEmitsSanitizedHTMLBlocks(t *testing.T) {
	c := Canvas{
		Blocks: []Block{
			{ID: "stats", Kind: KindHTML, Title: "Run", Body: `<p class="hv-muted">green</p><script>alert(1)</script>`},
		},
	}
	assert.Equal(t, "## Run\n\n<p class=\"hv-muted\">green</p>\n", Markdown(c))
}

func TestInvalidWorkspaceRefused(t *testing.T) {
	s := testStore(t)
	for _, dir := range []string{"", ".", "..", "a/b", "../escape"} {
		_, err := s.Upsert(dir, "plan", 1, "", "", Block{ID: "a", Kind: KindMarkdown, Body: "x"})
		require.ErrorIs(t, err, ErrInvalidWorkspace, "dir %q", dir)
		_, err = s.List(dir)
		require.ErrorIs(t, err, ErrInvalidWorkspace, "dir %q", dir)
	}
}

func TestInvalidNameRefused(t *testing.T) {
	s := testStore(t)
	long := make([]byte, maxNameLength+1)
	for i := range long {
		long[i] = 'a'
	}
	for _, name := range []string{"", ".", "..", "a/b", "../escape", ".hidden", "-plan", "plan-", "Report", "has space", string(long)} {
		_, err := s.Upsert("ws", name, 1, "", "", Block{ID: "a", Kind: KindMarkdown, Body: "x"})
		require.ErrorIs(t, err, ErrInvalidName, "name %q", name)
	}
	for _, name := range []string{"plan", "release-notes", "perf.report", "a", "v2_draft"} {
		_, err := s.Upsert("ws", name, 1, "", "", Block{ID: "a", Kind: KindMarkdown, Body: "x"})
		require.NoError(t, err, "name %q", name)
	}
}

func TestCorruptFileIsAnErrorNotABlankCanvas(t *testing.T) {
	s := testStore(t)
	_, err := s.Upsert("ws", "plan", 1, "", "", Block{ID: "a", Kind: KindMarkdown, Body: "x"})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(s.root, "ws", canvasesDirName, "plan.json"), []byte("{not json"), 0o600))

	_, _, err = s.Load("ws", "plan")
	require.Error(t, err)
	_, err = s.List("ws")
	require.Error(t, err)
}

func TestWritesLeaveNoTempFiles(t *testing.T) {
	s := testStore(t)
	_, err := s.Upsert("ws", "plan", 1, "", "", Block{ID: "a", Kind: KindMarkdown, Body: "x"})
	require.NoError(t, err)
	_, err = s.Clear("ws", "plan")
	require.NoError(t, err)

	entries, err := os.ReadDir(filepath.Join(s.root, "ws", canvasesDirName))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "plan.json", entries[0].Name())

	metas, err := s.List("ws")
	require.NoError(t, err)
	assert.Len(t, metas, 1, "a stray non-canvas file must not break the listing")
}
