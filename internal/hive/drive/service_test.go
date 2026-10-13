package drive

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/hive/events"
	"github.com/colonyops/hive/internal/hive/events/testbus"
	"github.com/colonyops/hive/internal/platform/git"
)

type stubGit struct {
	git.Git
	remote string
	err    error
}

func (g stubGit) RemoteURL(context.Context, string) (string, error) { return g.remote, g.err }

type fixture struct {
	svc  *Service
	root string
	bus  *testbus.Bus
}

// newFixture is a service over a temp root whose clock ticks one millisecond
// per call, so every mutation gets a distinct, ordered stamp.
func newFixture(t *testing.T, gitClient git.Git) fixture {
	t.Helper()
	root := t.TempDir()
	cfg := &config.Config{Context: config.ContextConfig{BaseDir: root, SymlinkName: ".hive"}}
	bus := testbus.New(t)
	svc := NewService(zerolog.Nop(), cfg, gitClient, bus.EventBus)
	var tick int64
	svc.now = func() time.Time {
		tick++
		return time.UnixMilli(tick)
	}
	return fixture{svc: svc, root: root, bus: bus}
}

func (f fixture) write(t *testing.T, space drive.Space, path, body string) {
	t.Helper()
	full := filepath.Join(f.root, filepath.FromSlash(space.Key), filepath.FromSlash(path))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(body), 0o644))
}

func (f fixture) changes() []events.DriveChangedPayload {
	var out []events.DriveChangedPayload
	for _, recorded := range f.bus.Events() {
		if recorded.Event == events.EventDriveChanged {
			out = append(out, recorded.Payload.(events.DriveChangedPayload))
		}
	}
	return out
}

var (
	ws   = must(drive.WorkspaceSpace("ws"))
	acme = must(drive.RepositorySpace("acme", "site"))
)

func must(space drive.Space, err error) drive.Space {
	if err != nil {
		panic(err)
	}
	return space
}

const plan = "canvases/plan.canvas.json"

func blockIDs(c drive.Canvas) []string {
	ids := make([]string, 0, len(c.Blocks))
	for _, b := range c.Blocks {
		ids = append(ids, b.ID)
	}
	return ids
}

// skipIfCaseSensitive skips a test that needs the temp directory to fold
// case, as APFS and NTFS do by default.
func skipIfCaseSensitive(t *testing.T, dir string) {
	t.Helper()
	probe := filepath.Join(dir, "a")
	require.NoError(t, os.WriteFile(probe, nil, 0o644))
	_, err := os.Lstat(filepath.Join(dir, "A"))
	require.NoError(t, os.Remove(probe))
	if err != nil {
		t.Skip("the temp directory is on a case-sensitive file system")
	}
}

func paths(entries []drive.Entry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Path)
	}
	return out
}

// Canvas operations, by document path.

func TestLoadCanvasAbsentIsNotAnError(t *testing.T) {
	f := newFixture(t, nil)
	_, ok, err := f.svc.LoadCanvas(t.Context(), ws, plan)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestUpsertBlocksAppendsThenUpdatesInPlace(t *testing.T) {
	f := newFixture(t, nil)
	ctx := t.Context()

	c, err := f.svc.UpsertBlocks(ctx, ws, plan, drive.Author{Session: "chat-1"}, "The Plan", "", drive.Block{ID: "plan", Kind: drive.BlockKindMarkdown, Body: "v1"})
	require.NoError(t, err)
	require.Len(t, c.Blocks, 1)
	created := c.Blocks[0].CreatedAt
	assert.NotZero(t, created)
	assert.NotZero(t, c.CreatedAt)
	assert.Equal(t, "chat-1", c.Session)
	assert.Equal(t, "The Plan", c.Title)
	assert.Equal(t, ws.Key, c.Space)
	assert.Equal(t, plan, c.Path)

	c, err = f.svc.UpsertBlocks(ctx, ws, plan, drive.Author{Session: "chat-1"}, "", "", drive.Block{ID: "result", Kind: drive.BlockKindLink, Title: "PR", URL: "https://example.com"})
	require.NoError(t, err)
	require.Len(t, c.Blocks, 2)
	assert.Equal(t, "The Plan", c.Title, "an empty title leaves the stored one")

	c, err = f.svc.UpsertBlocks(ctx, ws, plan, drive.Author{Session: "chat-2"}, "Revised", "", drive.Block{ID: "plan", Kind: drive.BlockKindMarkdown, Body: "v2"})
	require.NoError(t, err)
	require.Len(t, c.Blocks, 2)
	assert.Equal(t, "plan", c.Blocks[0].ID, "an updated block keeps its position")
	assert.Equal(t, "v2", c.Blocks[0].Body)
	assert.Equal(t, created, c.Blocks[0].CreatedAt, "an updated block keeps its CreatedAt")
	assert.Greater(t, c.Blocks[0].UpdatedAt, created)
	assert.Equal(t, "chat-1", c.Session, "the creating session never changes")
	assert.Equal(t, "Revised", c.Title)

	reloaded, ok, err := f.svc.LoadCanvas(ctx, ws, plan)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, c, reloaded)
	assert.FileExists(t, filepath.Join(f.root, "workspaces", "ws", "canvases", "plan.canvas.json"))
}

func TestUpsertBlocksBeforeAnchorInsertsAndMoves(t *testing.T) {
	f := newFixture(t, nil)
	ctx := t.Context()
	author := drive.Author{Session: "chat-1"}
	_, err := f.svc.UpsertBlocks(ctx, ws, plan, author, "", "", drive.Block{ID: "a", Kind: drive.BlockKindMarkdown, Body: "a"})
	require.NoError(t, err)
	c, err := f.svc.UpsertBlocks(ctx, ws, plan, author, "", "", drive.Block{ID: "b", Kind: drive.BlockKindMarkdown, Body: "b"})
	require.NoError(t, err)
	created := c.Blocks[1].CreatedAt

	c, err = f.svc.UpsertBlocks(ctx, ws, plan, author, "", "a", drive.Block{ID: "intro", Kind: drive.BlockKindMarkdown, Body: "i"})
	require.NoError(t, err)
	assert.Equal(t, []string{"intro", "a", "b"}, blockIDs(c), "a new block inserts before the anchor")

	c, err = f.svc.UpsertBlocks(ctx, ws, plan, author, "", "intro", drive.Block{ID: "b", Kind: drive.BlockKindMarkdown, Body: "b2"})
	require.NoError(t, err)
	assert.Equal(t, []string{"b", "intro", "a"}, blockIDs(c), "a reused id moves before the anchor")
	assert.Equal(t, created, c.Blocks[0].CreatedAt, "a moved block keeps its CreatedAt")
	assert.Equal(t, "b2", c.Blocks[0].Body)

	_, err = f.svc.UpsertBlocks(ctx, ws, plan, author, "", "ghost", drive.Block{ID: "x", Kind: drive.BlockKindMarkdown, Body: "x"})
	require.ErrorIs(t, err, drive.ErrAnchorNotFound)
	c, _, err = f.svc.LoadCanvas(ctx, ws, plan)
	require.NoError(t, err)
	assert.Equal(t, []string{"b", "intro", "a"}, blockIDs(c), "a rejected anchor writes nothing")

	_, err = f.svc.UpsertBlocks(ctx, ws, plan, author, "", "b", drive.Block{ID: "b", Kind: drive.BlockKindMarkdown, Body: "b3"})
	require.ErrorIs(t, err, drive.ErrAnchorNotFound, "a block cannot anchor before itself")
}

func TestUpsertBlocksBatchIsOneWrite(t *testing.T) {
	f := newFixture(t, nil)
	c, err := f.svc.UpsertBlocks(t.Context(), ws, plan, drive.Author{Session: "chat-1"}, "The Plan", "",
		drive.Block{ID: "a", Kind: drive.BlockKindMarkdown, Body: "a"},
		drive.Block{ID: "b", Kind: drive.BlockKindMarkdown, Body: "b"},
		drive.Block{ID: "c", Kind: drive.BlockKindLink, Title: "PR", URL: "https://example.com"},
	)
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, blockIDs(c), "a batch applies in order")
	assert.Equal(t, c.Blocks[0].UpdatedAt, c.Blocks[2].UpdatedAt, "one write, one stamp")
	assert.Len(t, f.changes(), 1, "one write, one event")
}

func TestRemoveBlock(t *testing.T) {
	f := newFixture(t, nil)
	ctx := t.Context()
	_, err := f.svc.UpsertBlocks(ctx, ws, plan, drive.Author{Session: "chat-1"}, "", "", drive.Block{ID: "a", Kind: drive.BlockKindMarkdown, Body: "x"})
	require.NoError(t, err)

	c, removed, err := f.svc.RemoveBlock(ctx, ws, plan, "a", drive.Author{})
	require.NoError(t, err)
	assert.True(t, removed)
	assert.Empty(t, c.Blocks)

	_, removed, err = f.svc.RemoveBlock(ctx, ws, plan, "a", drive.Author{})
	require.NoError(t, err)
	assert.False(t, removed)

	_, _, err = f.svc.RemoveBlock(ctx, ws, "canvases/other.canvas.json", "a", drive.Author{})
	require.ErrorIs(t, err, drive.ErrNotFound, "a never-written canvas is not found")
	assert.Len(t, f.changes(), 2, "a removal that changed nothing publishes nothing")
}

func TestSetCanvasFrontmatterReplacesEditableFields(t *testing.T) {
	f := newFixture(t, nil)
	ctx := t.Context()
	first, err := f.svc.UpsertBlocks(ctx, ws, plan, drive.Author{Session: "chat-1"}, "The Plan", "", drive.Block{ID: "a", Kind: drive.BlockKindMarkdown, Body: "x"})
	require.NoError(t, err)

	fm := drive.Frontmatter{"tags": []any{"release", "desktop"}, "reviewed": true, "score": float64(7)}
	c, err := f.svc.SetCanvasFrontmatter(ctx, ws, plan, drive.Author{}, fm)
	require.NoError(t, err)
	assert.Equal(t, fm, c.Frontmatter)
	assert.Greater(t, c.UpdatedAt, first.UpdatedAt)
	fm["tags"].([]any)[0] = "changed"
	assert.Equal(t, "release", c.Frontmatter["tags"].([]any)[0], "the canvas holds its own copy")

	c, err = f.svc.SetCanvasFrontmatter(ctx, ws, plan, drive.Author{}, drive.Frontmatter{})
	require.NoError(t, err)
	assert.Empty(t, c.Frontmatter, "an empty mapping removes every editable field")

	_, err = f.svc.SetCanvasFrontmatter(ctx, ws, plan, drive.Author{}, drive.Frontmatter{"created_at": "yesterday"})
	require.ErrorIs(t, err, drive.ErrInvalidFrontmatter)

	_, err = f.svc.SetCanvasFrontmatter(ctx, ws, "canvases/ghost.canvas.json", drive.Author{}, drive.Frontmatter{"tags": []any{"x"}})
	require.ErrorIs(t, err, drive.ErrNotFound)
}

func TestClearCanvasKeepsTheCanvas(t *testing.T) {
	f := newFixture(t, nil)
	ctx := t.Context()
	first, err := f.svc.UpsertBlocks(ctx, ws, plan, drive.Author{Session: "chat-1"}, "The Plan", "", drive.Block{ID: "a", Kind: drive.BlockKindMarkdown, Body: "x"})
	require.NoError(t, err)

	c, err := f.svc.ClearCanvas(ctx, ws, plan, drive.Author{})
	require.NoError(t, err)
	assert.Empty(t, c.Blocks)
	assert.Equal(t, first.CreatedAt, c.CreatedAt, "clear keeps the canvas's CreatedAt")
	assert.Equal(t, "The Plan", c.Title, "clear keeps the title")

	reloaded, ok, err := f.svc.LoadCanvas(ctx, ws, plan)
	require.NoError(t, err)
	require.True(t, ok, "the file survives a clear")
	assert.Empty(t, reloaded.Blocks)

	_, err = f.svc.ClearCanvas(ctx, ws, "canvases/other.canvas.json", drive.Author{})
	require.ErrorIs(t, err, drive.ErrNotFound)
}

func TestListCanvasesOrdersByUpdatedAtDesc(t *testing.T) {
	f := newFixture(t, nil)
	ctx := t.Context()
	_, err := f.svc.UpsertBlocks(ctx, ws, plan, drive.Author{Session: "chat-1"}, "The Plan", "", drive.Block{ID: "a", Kind: drive.BlockKindMarkdown, Body: "x"})
	require.NoError(t, err)
	_, err = f.svc.UpsertBlocks(ctx, ws, "reports/2026/perf.canvas.json", drive.Author{Session: "chat-2"}, "", "", drive.Block{ID: "a", Kind: drive.BlockKindMarkdown, Body: "x"})
	require.NoError(t, err)
	_, err = f.svc.UpsertBlocks(ctx, ws, plan, drive.Author{Session: "chat-1"}, "", "", drive.Block{ID: "b", Kind: drive.BlockKindMarkdown, Body: "y"})
	require.NoError(t, err)

	metas, err := f.svc.ListCanvases(ctx, ws)
	require.NoError(t, err)
	require.Len(t, metas, 2)
	assert.Equal(t, plan, metas[0].Path, "the most recently updated canvas lists first")
	assert.Equal(t, ws.Key, metas[0].Space)
	assert.Equal(t, "The Plan", metas[0].Title)
	assert.Equal(t, "chat-1", metas[0].Session)
	assert.Equal(t, 2, metas[0].BlockCount)
	assert.Equal(t, "reports/2026/perf.canvas.json", metas[1].Path, "a canvas under a folder is listed too")

	empty, err := f.svc.ListCanvases(ctx, acme)
	require.NoError(t, err)
	assert.Empty(t, empty)
}

func TestCanvasPathMustBeACanvas(t *testing.T) {
	f := newFixture(t, nil)
	ctx := t.Context()
	for _, path := range []string{"plan.md", "canvases/plan.json", "../plan.canvas.json"} {
		_, err := f.svc.UpsertBlocks(ctx, ws, path, drive.Author{}, "", "", drive.Block{ID: "a", Kind: drive.BlockKindMarkdown, Body: "x"})
		require.ErrorIs(t, err, drive.ErrInvalidPath, "path %q", path)
	}
	_, err := f.svc.UpsertBlocks(ctx, drive.Space{Key: "../escape"}, plan, drive.Author{}, "", "", drive.Block{ID: "a", Kind: drive.BlockKindMarkdown, Body: "x"})
	require.ErrorIs(t, err, drive.ErrInvalidSpace)
}

func TestCorruptCanvasIsAnErrorNotABlankCanvas(t *testing.T) {
	f := newFixture(t, nil)
	ctx := t.Context()
	_, err := f.svc.UpsertBlocks(ctx, ws, plan, drive.Author{Session: "chat-1"}, "", "", drive.Block{ID: "a", Kind: drive.BlockKindMarkdown, Body: "x"})
	require.NoError(t, err)
	f.write(t, ws, plan, "{not json")

	_, _, err = f.svc.LoadCanvas(ctx, ws, plan)
	require.Error(t, err)
	_, err = f.svc.ListCanvases(ctx, ws)
	require.Error(t, err)
}

func TestCanvasWritesLeaveNoTempFiles(t *testing.T) {
	f := newFixture(t, nil)
	ctx := t.Context()
	_, err := f.svc.UpsertBlocks(ctx, ws, plan, drive.Author{Session: "chat-1"}, "", "", drive.Block{ID: "a", Kind: drive.BlockKindMarkdown, Body: "x"})
	require.NoError(t, err)
	_, err = f.svc.ClearCanvas(ctx, ws, plan, drive.Author{})
	require.NoError(t, err)

	entries, err := os.ReadDir(filepath.Join(f.root, "workspaces", "ws", "canvases"))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "plan.canvas.json", entries[0].Name())
}

func TestCanvasFileCarriesSpaceAndPath(t *testing.T) {
	f := newFixture(t, nil)
	_, err := f.svc.UpsertBlocks(t.Context(), acme, plan, drive.Author{HiveSession: "abc123"}, "", "", drive.Block{ID: "a", Kind: drive.BlockKindMarkdown, Body: "x"})
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(f.root, "acme", "site", "canvases", "plan.canvas.json"))
	require.NoError(t, err)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.Equal(t, "acme/site", raw["space"])
	assert.Equal(t, plan, raw["path"])
	assert.Equal(t, "abc123", raw["hiveSession"])
	assert.NotContains(t, raw, "workspace")
	assert.NotContains(t, raw, "name")
}

// The tree.

func TestTreeListsEveryKindAndSkipsDotEntries(t *testing.T) {
	f := newFixture(t, nil)
	f.write(t, acme, "plans/2026-10-11-plan.md", "---\ntitle: The Plan\ntype: plan\n---\n# hi\n")
	f.write(t, acme, "plans/notes.md", "no front matter\n")
	f.write(t, acme, "report.html", "<h1>hi</h1>")
	f.write(t, acme, "notes.txt", "x")
	f.write(t, acme, "canvases/perf.canvas.json", `{"title":"Perf","frontmatter":{"ready":true},"blocks":[]}`)
	f.write(t, acme, "canvases/stray.json", `{"not":"a canvas"}`)
	f.write(t, acme, ".git/HEAD", "ref")
	f.write(t, acme, ".DS_Store", "")
	f.write(t, acme, "kv.json", `{"entries":{}}`)
	f.write(t, acme, "kv.json.lock", "")

	root, err := f.svc.Tree(t.Context(), acme)
	require.NoError(t, err)
	assert.Equal(t, drive.KindDir, root.Kind)
	assert.Empty(t, root.Path)
	assert.Equal(t, []string{"canvases", "plans", "notes.txt", "report.html"}, paths(root.Children), "folders first, then natural order")

	entry, ok := drive.Find(root, "plans/2026-10-11-plan.md")
	require.True(t, ok)
	assert.Equal(t, drive.KindMarkdown, entry.Kind)
	assert.Equal(t, "The Plan", entry.Title)
	assert.Equal(t, drive.Frontmatter{"title": "The Plan", "type": "plan"}, entry.Front)
	assert.Equal(t, "2026-10-11-plan.md", entry.Name)
	assert.NotZero(t, entry.Size)
	assert.False(t, entry.Modified.IsZero())
	assert.False(t, entry.Created.IsZero())

	entry, ok = drive.Find(root, "plans/notes.md")
	require.True(t, ok)
	assert.Empty(t, entry.Title)
	assert.Nil(t, entry.Front)

	entry, ok = drive.Find(root, "report.html")
	require.True(t, ok)
	assert.Equal(t, drive.KindHtml, entry.Kind)

	entry, ok = drive.Find(root, "notes.txt")
	require.True(t, ok)
	assert.Equal(t, drive.KindFile, entry.Kind)

	entry, ok = drive.Find(root, "canvases/perf.canvas.json")
	require.True(t, ok)
	assert.Equal(t, drive.KindCanvas, entry.Kind)
	assert.Equal(t, "Perf", entry.Title)
	assert.Equal(t, drive.Frontmatter{"ready": true}, entry.Front)

	entry, ok = drive.Find(root, "canvases/stray.json")
	require.True(t, ok)
	assert.Equal(t, drive.KindFile, entry.Kind, "a json file without blocks is a plain file")

	_, ok = drive.Find(root, ".git")
	assert.False(t, ok)
	_, ok = drive.Find(root, ".DS_Store")
	assert.False(t, ok)
	_, ok = drive.Find(root, "kv.json")
	assert.False(t, ok, "an earlier hive's key-value store is not a document")
	_, ok = drive.Find(root, "kv.json.lock")
	assert.False(t, ok)
}

func TestTreeOfAMissingSpaceIsEmpty(t *testing.T) {
	f := newFixture(t, nil)
	root, err := f.svc.Tree(t.Context(), drive.Shared)
	require.NoError(t, err)
	assert.Equal(t, drive.Entry{Kind: drive.KindDir}, root)

	_, err = f.svc.Tree(t.Context(), drive.Space{Key: "../escape"})
	require.ErrorIs(t, err, drive.ErrInvalidSpace)
}

func TestTreeIsACopy(t *testing.T) {
	f := newFixture(t, nil)
	f.write(t, acme, "plans/a.md", "---\ntags: [x]\n---\n")

	root, err := f.svc.Tree(t.Context(), acme)
	require.NoError(t, err)
	root.Children[0].Children[0].Title = "mutated"
	root.Children[0].Children[0].Front["tags"].([]any)[0] = "mutated"
	root.Children = nil

	again, err := f.svc.Tree(t.Context(), acme)
	require.NoError(t, err)
	entry, ok := drive.Find(again, "plans/a.md")
	require.True(t, ok)
	assert.Empty(t, entry.Title)
	assert.Equal(t, drive.Frontmatter{"tags": []any{"x"}}, entry.Front)
}

func TestEntryAnswersOneEntryFromTheSnapshot(t *testing.T) {
	f := newFixture(t, nil)
	ctx := t.Context()
	f.write(t, acme, "plans/sub/a.md", "---\ntitle: A\n---\n")

	entry, err := f.svc.Entry(ctx, acme, "plans")
	require.NoError(t, err, "an unscanned space is scanned first")
	assert.Equal(t, drive.KindDir, entry.Kind)
	assert.Equal(t, []string{"plans/sub"}, paths(entry.Children))

	entry.Children[0].Name = "changed"
	again, err := f.svc.Entry(ctx, acme, "plans/sub/a.md")
	require.NoError(t, err)
	assert.Equal(t, "A", again.Title)
	sub, err := f.svc.Entry(ctx, acme, "plans/sub")
	require.NoError(t, err)
	assert.Equal(t, "sub", sub.Name, "the answer is a copy")

	root, err := f.svc.Entry(ctx, acme, "")
	require.NoError(t, err)
	assert.Equal(t, []string{"plans"}, paths(root.Children))

	_, err = f.svc.Entry(ctx, acme, "ghost.md")
	require.ErrorIs(t, err, drive.ErrNotFound)
	_, err = f.svc.Entry(ctx, acme, "../escape")
	require.ErrorIs(t, err, drive.ErrInvalidPath)
}

func TestLocateIsTheFileOnDisk(t *testing.T) {
	f := newFixture(t, nil)
	got, err := f.svc.Locate(acme, "plans/a.md")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(f.root, "acme", "site", "plans", "a.md"), got)
	got, err = f.svc.Locate(acme, "")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(f.root, "acme", "site"), got)
	_, err = f.svc.Locate(acme, "../escape")
	require.ErrorIs(t, err, drive.ErrInvalidPath)
	_, err = f.svc.Locate(drive.Space{Key: "../x"}, "a.md")
	require.ErrorIs(t, err, drive.ErrInvalidSpace)
}

func TestFirstScanRenamesLegacyCanvasesOnce(t *testing.T) {
	f := newFixture(t, nil)
	f.write(t, acme, "canvases/plan.json", `{"workspace":"acme/site","name":"plan","title":"Legacy","blocks":[]}`)
	f.write(t, acme, "canvases/taken.json", `{"blocks":[]}`)
	f.write(t, acme, "canvases/taken.canvas.json", `{"title":"Already here","blocks":[]}`)
	f.write(t, acme, "canvases/stray.json", `{"blocks":"no"}`)
	f.write(t, acme, "canvases/notes.json", `{"a":1}`)
	f.write(t, acme, "canvases/broken.json", `{`)
	f.write(t, acme, "other/plan.json", `{"blocks":[]}`)

	root, err := f.svc.Tree(t.Context(), acme)
	require.NoError(t, err)
	canvases, ok := drive.Find(root, "canvases")
	require.True(t, ok)
	assert.Equal(t, []string{
		"canvases/broken.json", "canvases/notes.json", "canvases/plan.canvas.json",
		"canvases/stray.canvas.json", "canvases/taken.canvas.json", "canvases/taken.json",
	}, paths(canvases.Children))
	for _, path := range []string{"canvases/notes.json", "canvases/broken.json"} {
		entry, _ := drive.Find(root, path)
		assert.Equal(t, drive.KindFile, entry.Kind, "%s holds no canvas and keeps its name", path)
	}
	entry, _ := drive.Find(root, "canvases/plan.canvas.json")
	assert.Equal(t, "Legacy", entry.Title)
	entry, _ = drive.Find(root, "canvases/taken.canvas.json")
	assert.Equal(t, "Already here", entry.Title, "an existing target is left alone")
	entry, _ = drive.Find(root, "canvases/stray.canvas.json")
	assert.Equal(t, drive.KindCanvas, entry.Kind, "a blocks key of any shape makes a canvas; the kind is by name")
	entry, _ = drive.Find(root, "other/plan.json")
	assert.Equal(t, drive.KindFile, entry.Kind, "only canvases/ is migrated")

	listing := func() []string {
		entries, err := os.ReadDir(filepath.Join(f.root, "acme", "site", "canvases"))
		require.NoError(t, err)
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		return names
	}
	f.write(t, acme, "canvases/late.json", `{"blocks":[]}`)
	f.svc.mu.Lock()
	delete(f.svc.spaces, acme.Key)
	f.svc.mu.Unlock()
	rebuilt, err := f.svc.Tree(t.Context(), acme)
	require.NoError(t, err)
	_, ok = drive.Find(rebuilt, "canvases/late.json")
	assert.True(t, ok, "a rebuilt snapshot does not run the rename again")
	require.NoError(t, os.Remove(filepath.Join(f.root, "acme", "site", "canvases", "late.json")))

	before := listing()
	again := newFixture(t, nil)
	again.svc.cfg = f.svc.cfg
	root2, err := again.svc.Tree(t.Context(), acme)
	require.NoError(t, err)
	assert.Equal(t, paths(flatten(root)), paths(flatten(root2)))
	assert.Equal(t, before, listing(), "a second scan renames nothing")
}

// Documents.

func TestWriteCreatesFoldersAndIndexesTheDocument(t *testing.T) {
	f := newFixture(t, nil)
	ctx := t.Context()
	_, err := f.svc.Tree(ctx, acme)
	require.NoError(t, err)

	entry, err := f.svc.Write(ctx, acme, "plans/2026/plan.md", drive.Author{}, []byte("---\ntitle: Plan\n---\nbody\n"))
	require.NoError(t, err)
	assert.Equal(t, drive.KindMarkdown, entry.Kind)
	assert.Equal(t, "Plan", entry.Title)
	assert.Equal(t, "plans/2026/plan.md", entry.Path)
	assert.Nil(t, entry.Children)

	page, err := f.svc.Write(ctx, acme, "page.html", drive.Author{}, []byte("<p>hi</p>"))
	require.NoError(t, err)
	assert.Equal(t, drive.KindHtml, page.Kind)

	root, err := f.svc.Tree(ctx, acme)
	require.NoError(t, err)
	assert.Equal(t, []string{"plans", "page.html"}, paths(root.Children))
	folder, ok := drive.Find(root, "plans/2026")
	require.True(t, ok)
	assert.Equal(t, drive.KindDir, folder.Kind)
	assert.False(t, folder.Modified.IsZero(), "a folder created on the way is indexed from disk")
	assert.Equal(t, []string{"plans/2026/plan.md"}, paths(folder.Children))

	got, data, err := f.svc.Read(ctx, acme, "plans/2026/plan.md")
	require.NoError(t, err)
	assert.Equal(t, "---\ntitle: Plan\n---\nbody\n", string(data))
	assert.Equal(t, "Plan", got.Title)

	changes := f.changes()
	require.Len(t, changes, 2)
	assert.Equal(t, events.DriveChangedPayload{Space: "acme/site", Paths: []string{"plans/2026/plan.md"}}, changes[0])
}

func TestWriteRefusesWhatIsNotMarkdownOrHTML(t *testing.T) {
	f := newFixture(t, nil)
	ctx := t.Context()
	for _, path := range []string{"canvases/plan.canvas.json", "notes.txt", "data.json", "plans"} {
		_, err := f.svc.Write(ctx, acme, path, drive.Author{}, []byte("x"))
		require.ErrorIs(t, err, drive.ErrNotWritable, "path %q", path)
	}
	for _, path := range []string{"", "../x.md", ".hidden/x.md", "a//b.md"} {
		_, err := f.svc.Write(ctx, acme, path, drive.Author{}, []byte("x"))
		require.ErrorIs(t, err, drive.ErrInvalidPath, "path %q", path)
	}
	_, err := f.svc.CreateFolder(ctx, acme, "plans", drive.Author{})
	require.NoError(t, err)
	_, err = f.svc.Write(ctx, acme, "plans", drive.Author{}, []byte("x"))
	require.ErrorIs(t, err, drive.ErrNotWritable, "a folder path is refused by kind before anything else")
	assert.Len(t, f.changes(), 1)
}

func TestReadMissingAndFolders(t *testing.T) {
	f := newFixture(t, nil)
	ctx := t.Context()
	f.write(t, acme, "plans/a.md", "x")

	_, _, err := f.svc.Read(ctx, acme, "plans/b.md")
	require.ErrorIs(t, err, drive.ErrNotFound)
	_, _, err = f.svc.Read(ctx, acme, "plans")
	require.ErrorIs(t, err, drive.ErrInvalidPath)
	_, _, err = f.svc.Read(ctx, acme, "../a.md")
	require.ErrorIs(t, err, drive.ErrInvalidPath)
	assert.Empty(t, f.changes(), "a read publishes nothing")
}

func TestCreateFolder(t *testing.T) {
	f := newFixture(t, nil)
	ctx := t.Context()
	_, err := f.svc.Tree(ctx, acme)
	require.NoError(t, err)

	entry, err := f.svc.CreateFolder(ctx, acme, "reports/2026", drive.Author{})
	require.NoError(t, err)
	assert.Equal(t, drive.KindDir, entry.Kind)
	assert.Equal(t, "2026", entry.Name)
	assert.DirExists(t, filepath.Join(f.root, "acme", "site", "reports", "2026"))

	_, err = f.svc.CreateFolder(ctx, acme, "reports/2026", drive.Author{})
	require.ErrorIs(t, err, drive.ErrExists)
	f.write(t, acme, "notes.md", "x")
	_, err = f.svc.CreateFolder(ctx, acme, "notes.md", drive.Author{})
	require.ErrorIs(t, err, drive.ErrExists, "a document in the way is a conflict too")

	root, err := f.svc.Tree(ctx, acme)
	require.NoError(t, err)
	assert.Equal(t, []string{"reports"}, paths(root.Children))
	assert.Equal(t, []string{"reports/2026"}, paths(root.Children[0].Children))
	assert.Len(t, f.changes(), 1)
}

func TestMoveRekeysAFolderAndRefusesConflicts(t *testing.T) {
	f := newFixture(t, nil)
	ctx := t.Context()
	f.write(t, acme, "plans/a.md", "---\ntitle: A\n---\n")
	f.write(t, acme, "plans/sub/b.md", "b")
	f.write(t, acme, "taken.md", "t")
	_, err := f.svc.Tree(ctx, acme)
	require.NoError(t, err)

	moved, err := f.svc.Move(ctx, acme, "plans", "archive/2026/plans", drive.Author{})
	require.NoError(t, err)
	assert.Equal(t, "archive/2026/plans", moved.Path)
	assert.Equal(t, "plans", moved.Name)
	assert.Equal(t, []string{"archive/2026/plans/sub", "archive/2026/plans/a.md"}, paths(moved.Children))
	assert.DirExists(t, filepath.Join(f.root, "acme", "site", "archive", "2026", "plans", "sub"))

	root, err := f.svc.Tree(ctx, acme)
	require.NoError(t, err)
	_, ok := drive.Find(root, "plans")
	assert.False(t, ok)
	entry, ok := drive.Find(root, "archive/2026/plans/sub/b.md")
	require.True(t, ok, "descendants follow the folder in the snapshot")
	assert.Equal(t, "b.md", entry.Name)
	entry, ok = drive.Find(root, "archive/2026/plans/a.md")
	require.True(t, ok)
	assert.Equal(t, "A", entry.Title, "a re-keyed entry keeps what the scan read")

	renamed, err := f.svc.Move(ctx, acme, "archive/2026/plans/a.md", "archive/b.md", drive.Author{})
	require.NoError(t, err)
	assert.Equal(t, "archive/b.md", renamed.Path)
	assert.Equal(t, "A", renamed.Title, "a renamed document is indexed from disk")

	_, err = f.svc.Move(ctx, acme, "archive/b.md", "taken.md", drive.Author{})
	require.ErrorIs(t, err, drive.ErrExists)
	_, err = f.svc.Move(ctx, acme, "ghost.md", "x.md", drive.Author{})
	require.ErrorIs(t, err, drive.ErrNotFound)
	_, err = f.svc.Move(ctx, acme, "archive", "archive/inner", drive.Author{})
	require.ErrorIs(t, err, drive.ErrInvalidPath)
	_, err = f.svc.Move(ctx, acme, "archive", "archive", drive.Author{})
	require.ErrorIs(t, err, drive.ErrInvalidPath)

	changes := f.changes()
	require.Len(t, changes, 2)
	assert.Equal(t, []string{"plans", "archive/2026/plans"}, changes[0].Paths)
}

func TestMoveRefusesAKindChange(t *testing.T) {
	f := newFixture(t, nil)
	ctx := t.Context()
	f.write(t, acme, "plans/a.md", "a")
	require.NoError(t, os.MkdirAll(filepath.Join(f.root, "acme", "site", "notes.md"), 0o755))

	for _, to := range []string{"plans/a.html", "plans/a.txt", "plans/a.canvas.json", "plans/a"} {
		_, err := f.svc.Move(ctx, acme, "plans/a.md", to, drive.Author{})
		require.ErrorIs(t, err, drive.ErrInvalidPath, to)
		require.ErrorContains(t, err, "keep the extension", to)
	}
	assert.FileExists(t, filepath.Join(f.root, "acme", "site", "plans", "a.md"))

	_, err := f.svc.Move(ctx, acme, "notes.md", "notes", drive.Author{})
	require.NoError(t, err, "a folder has no kind to keep")
	assert.Len(t, f.changes(), 1, "a refusal publishes nothing")
}

func TestMoveKeepsACanvasAddressable(t *testing.T) {
	f := newFixture(t, nil)
	ctx := t.Context()
	_, err := f.svc.UpsertBlocks(ctx, acme, plan, drive.Author{}, "", "", drive.Block{ID: "a", Kind: drive.BlockKindMarkdown, Body: "x"})
	require.NoError(t, err)

	for _, to := range []string{"plans/My Plan.canvas.json", "Plan.canvas.json", "plans/-x.canvas.json"} {
		_, err := f.svc.Move(ctx, acme, plan, to, drive.Author{})
		require.ErrorIs(t, err, drive.ErrInvalidPath, to)
		require.ErrorContains(t, err, "canvas name", to)
	}
	assert.FileExists(t, filepath.Join(f.root, "acme", "site", "canvases", "plan.canvas.json"))

	moved, err := f.svc.Move(ctx, acme, plan, "perf.canvas.json", drive.Author{})
	require.NoError(t, err, "a canvas at the space root is addressable as ./perf")
	assert.Equal(t, "./perf", drive.CanvasName(moved.Path))
	_, err = f.svc.Move(ctx, acme, "perf.canvas.json", "reports/2026/perf.canvas.json", drive.Author{})
	require.NoError(t, err)
}

func TestMoveRenamesCaseOnlyOnACaseInsensitiveFileSystem(t *testing.T) {
	f := newFixture(t, nil)
	ctx := t.Context()
	skipIfCaseSensitive(t, f.root)
	f.write(t, acme, "plan.md", "---\ntitle: P\n---\n")
	_, err := f.svc.Tree(ctx, acme)
	require.NoError(t, err)

	moved, err := f.svc.Move(ctx, acme, "plan.md", "Plan.md", drive.Author{})
	require.NoError(t, err)
	assert.Equal(t, "Plan.md", moved.Path)
	entries, err := os.ReadDir(filepath.Join(f.root, "acme", "site"))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "Plan.md", entries[0].Name())
	root, err := f.svc.Tree(ctx, acme)
	require.NoError(t, err)
	assert.Equal(t, []string{"Plan.md"}, paths(root.Children))

	f.write(t, acme, "other.md", "o")
	_, err = f.svc.Move(ctx, acme, "Plan.md", "other.md", drive.Author{})
	require.ErrorIs(t, err, drive.ErrExists, "a different file is still a conflict")
}

func TestMoveOfAnUnscannedSpaceIndexesFromDisk(t *testing.T) {
	f := newFixture(t, nil)
	ctx := t.Context()
	f.write(t, acme, "plans/sub/b.md", "b")

	moved, err := f.svc.Move(ctx, acme, "plans", "archive", drive.Author{})
	require.NoError(t, err)
	assert.Equal(t, []string{"archive/sub"}, paths(moved.Children))

	root, err := f.svc.Tree(ctx, acme)
	require.NoError(t, err)
	_, ok := drive.Find(root, "archive/sub/b.md")
	assert.True(t, ok)
}

func TestDeleteRefusesANonEmptyFolder(t *testing.T) {
	f := newFixture(t, nil)
	ctx := t.Context()
	f.write(t, acme, "plans/a.md", "a")
	f.write(t, acme, "empty/.keep", "")
	_, err := f.svc.CreateFolder(ctx, acme, "reports", drive.Author{})
	require.NoError(t, err)

	require.ErrorIs(t, f.svc.Delete(ctx, acme, "plans", drive.Author{}), drive.ErrNotEmpty)
	require.ErrorIs(t, f.svc.Delete(ctx, acme, "empty", drive.Author{}), drive.ErrNotEmpty, "a hidden file still counts")
	require.ErrorIs(t, f.svc.Delete(ctx, acme, "ghost.md", drive.Author{}), drive.ErrNotFound)
	require.NoError(t, f.svc.Delete(ctx, acme, "plans/a.md", drive.Author{}))
	require.NoError(t, f.svc.Delete(ctx, acme, "plans", drive.Author{}))
	require.NoError(t, f.svc.Delete(ctx, acme, "reports", drive.Author{}))
	assert.NoFileExists(t, filepath.Join(f.root, "acme", "site", "plans", "a.md"))
	assert.NoDirExists(t, filepath.Join(f.root, "acme", "site", "plans"))

	root, err := f.svc.Tree(ctx, acme)
	require.NoError(t, err)
	assert.Equal(t, []string{"empty"}, paths(root.Children))
	assert.Len(t, f.changes(), 4, "one event per change, none for a refusal")
}

func TestDeleteRemovesAFolderHoldingOnlyFinderMetadata(t *testing.T) {
	f := newFixture(t, nil)
	ctx := t.Context()
	f.write(t, acme, "opened/.DS_Store", "x")
	f.write(t, acme, "vault/.DS_Store", "x")
	f.write(t, acme, "vault/.obsidian/app.json", "{}")
	f.write(t, acme, "kept/.DS_Store", "x")
	f.write(t, acme, "kept/.gitkeep", "")

	require.NoError(t, f.svc.Delete(ctx, acme, "opened", drive.Author{}))
	assert.NoDirExists(t, filepath.Join(f.root, "acme", "site", "opened"))
	require.ErrorIs(t, f.svc.Delete(ctx, acme, "vault", drive.Author{}), drive.ErrNotEmpty)
	require.ErrorIs(t, f.svc.Delete(ctx, acme, "kept", drive.Author{}), drive.ErrNotEmpty)
	assert.FileExists(t, filepath.Join(f.root, "acme", "site", "kept", ".DS_Store"), "a refused folder keeps its metadata")
}

// Symbolic links.

func TestADirectoryLinkInsideASpaceIsRefused(t *testing.T) {
	f := newFixture(t, nil)
	ctx := t.Context()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.md"), []byte("secret"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(f.root, "acme", "site"), 0o755))
	require.NoError(t, os.Symlink(outside, filepath.Join(f.root, "acme", "site", "link")))

	_, err := f.svc.Write(ctx, acme, "link/pwn.md", drive.Author{}, []byte("x"))
	require.ErrorIs(t, err, drive.ErrInvalidPath)
	assert.NoFileExists(t, filepath.Join(outside, "pwn.md"))
	_, _, err = f.svc.Read(ctx, acme, "link/secret.md")
	require.ErrorIs(t, err, drive.ErrInvalidPath)
	require.ErrorIs(t, f.svc.Delete(ctx, acme, "link/secret.md", drive.Author{}), drive.ErrInvalidPath)
	_, err = f.svc.Move(ctx, acme, "link/secret.md", "stolen.md", drive.Author{})
	require.ErrorIs(t, err, drive.ErrInvalidPath)
	_, err = f.svc.UpsertBlocks(ctx, acme, "link/x.canvas.json", drive.Author{}, "", "", drive.Block{ID: "a", Kind: drive.BlockKindMarkdown})
	require.ErrorIs(t, err, drive.ErrInvalidPath)
	assert.FileExists(t, filepath.Join(outside, "secret.md"))
}

func TestAFileLinkInsideASpaceIsRefused(t *testing.T) {
	f := newFixture(t, nil)
	ctx := t.Context()
	secret := filepath.Join(t.TempDir(), "secret.md")
	require.NoError(t, os.WriteFile(secret, []byte("secret"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(f.root, "acme", "site"), 0o755))
	require.NoError(t, os.Symlink(secret, filepath.Join(f.root, "acme", "site", "leak.md")))

	_, _, err := f.svc.Read(ctx, acme, "leak.md")
	require.ErrorIs(t, err, drive.ErrInvalidPath)
	_, err = f.svc.Write(ctx, acme, "leak.md", drive.Author{}, []byte("x"))
	require.ErrorIs(t, err, drive.ErrInvalidPath)
	_, err = f.svc.Locate(acme, "leak.md")
	require.ErrorIs(t, err, drive.ErrInvalidPath)
	data, err := os.ReadFile(secret)
	require.NoError(t, err)
	assert.Equal(t, "secret", string(data))

	root, err := f.svc.Tree(ctx, acme)
	require.NoError(t, err)
	assert.Empty(t, root.Children, "the scan still hides links")
}

func TestASpaceWhoseDirectoryIsALinkWorks(t *testing.T) {
	f := newFixture(t, nil)
	ctx := t.Context()
	elsewhere := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(f.root, "acme"), 0o755))
	require.NoError(t, os.Symlink(elsewhere, filepath.Join(f.root, "acme", "site")))

	_, err := f.svc.Write(ctx, acme, "plans/a.md", drive.Author{}, []byte("# A\n"))
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(elsewhere, "plans", "a.md"))
	_, data, err := f.svc.Read(ctx, acme, "plans/a.md")
	require.NoError(t, err)
	assert.Equal(t, "# A\n", string(data))
}

// Spaces.

func TestSpacesListsRepositoriesWorkspacesAndShared(t *testing.T) {
	f := newFixture(t, nil)
	for _, dir := range []string{"zed/editor", "acme/site", "acme/docs", "workspaces/hive", "workspaces/.hidden", "shared/plans", ".git", "loose-file-owner", "Shared/x"} {
		require.NoError(t, os.MkdirAll(filepath.Join(f.root, filepath.FromSlash(dir)), 0o755))
	}
	require.NoError(t, os.WriteFile(filepath.Join(f.root, "acme", "README.md"), []byte("x"), 0o644))

	spaces, err := f.svc.Spaces(t.Context())
	require.NoError(t, err)
	keys := make([]string, 0, len(spaces))
	for _, space := range spaces {
		keys = append(keys, space.Key)
	}
	assert.Equal(t, []string{"acme/docs", "acme/site", "zed/editor", "workspaces/hive", "shared"}, keys)
	assert.Equal(t, drive.SpaceKindRepository, spaces[0].Kind)
	assert.Equal(t, drive.SpaceKindWorkspace, spaces[3].Kind)
	assert.Equal(t, drive.Shared, spaces[4])

	empty := newFixture(t, nil)
	spaces, err = empty.svc.Spaces(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []drive.Space{drive.Shared}, spaces, "shared is listed before its directory exists")
}

func TestSpaceDirConfinesKeysToTheRoot(t *testing.T) {
	f := newFixture(t, nil)
	dir, err := f.svc.SpaceDir(acme)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(f.root, "acme", "site"), dir)
	dir, err = f.svc.SpaceDir(drive.Shared)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(f.root, "shared"), dir)
	dir, err = f.svc.SpaceDir(ws)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(f.root, "workspaces", "ws"), dir)
	assert.Equal(t, f.root, f.svc.Root())

	for _, key := range []string{"", "..", "../x", "a/../b", "a/b/c", "/etc/passwd", "workspaces/../x", "workspaces", "shared/x"} {
		_, err := f.svc.SpaceDir(drive.Space{Key: key})
		require.ErrorIs(t, err, drive.ErrInvalidSpace, "key %q", key)
	}
}

func TestSpaceForRemote(t *testing.T) {
	f := newFixture(t, nil)
	space, ok := f.svc.SpaceForRemote("git@github.com:acme/site.git")
	require.True(t, ok)
	assert.Equal(t, acme, space)
	space, ok = f.svc.SpaceForRemote("https://github.com/acme/site")
	require.True(t, ok)
	assert.Equal(t, acme, space)
	_, ok = f.svc.SpaceForRemote("")
	assert.False(t, ok)
	_, ok = f.svc.SpaceForRemote("https://github.com/shared/site")
	assert.False(t, ok, "a reserved owner is no repository space")
	space, ok = SpaceForRemote("git@github.com:acme/site.git")
	require.True(t, ok)
	assert.Equal(t, acme, space)
}

func TestSpaceForRemoteTakesTheOnDiskSpelling(t *testing.T) {
	f := newFixture(t, stubGit{remote: "git@github.com:ACME/Site.git"})
	skipIfCaseSensitive(t, f.root)
	require.NoError(t, os.MkdirAll(filepath.Join(f.root, "acme", "site"), 0o755))

	space, ok := f.svc.SpaceForRemote("git@github.com:ACME/Site.git")
	require.True(t, ok)
	assert.Equal(t, acme, space)
	space, err := f.svc.ResolveSpace(t.Context(), "", false)
	require.NoError(t, err)
	assert.Equal(t, acme, space)
	space, err = f.svc.ResolveSpace(t.Context(), "Acme/SITE", false)
	require.NoError(t, err)
	assert.Equal(t, acme, space)

	space, ok = f.svc.SpaceForRemote("git@github.com:Zed/Editor.git")
	require.True(t, ok)
	assert.Equal(t, "Zed/Editor", space.Key, "a space with no directory keeps the remote's spelling")
}

func TestWriteInAnotherCaseKeepsTheNameOnDisk(t *testing.T) {
	f := newFixture(t, nil)
	ctx := t.Context()
	skipIfCaseSensitive(t, f.root)
	f.write(t, acme, "plans/notes.md", "old")
	_, err := f.svc.Tree(ctx, acme)
	require.NoError(t, err)

	entry, err := f.svc.Write(ctx, acme, "Plans/Notes.md", drive.Author{}, []byte("new"))
	require.NoError(t, err)
	assert.Equal(t, "plans/notes.md", entry.Path)
	root, err := f.svc.Tree(ctx, acme)
	require.NoError(t, err)
	assert.Equal(t, []string{"plans"}, paths(root.Children))
	assert.Equal(t, []string{"plans/notes.md"}, paths(root.Children[0].Children))
	assert.Equal(t, []events.DriveChangedPayload{{Space: acme.Key, Paths: []string{"plans/notes.md"}}}, f.changes())
}

func TestResolveSpaceAndDir(t *testing.T) {
	ctx := t.Context()
	f := newFixture(t, stubGit{remote: "git@github.com:acme/site.git"})

	space, err := f.svc.ResolveSpace(ctx, "", true)
	require.NoError(t, err)
	assert.Equal(t, drive.Shared, space)
	space, err = f.svc.ResolveSpace(ctx, "zed/editor", false)
	require.NoError(t, err)
	assert.Equal(t, "zed/editor", space.Key)
	space, err = f.svc.ResolveSpace(ctx, "", false)
	require.NoError(t, err)
	assert.Equal(t, acme, space)
	_, err = f.svc.ResolveSpace(ctx, "no-slash", false)
	require.Error(t, err)
	_, err = f.svc.ResolveSpace(ctx, "shared/x", false)
	require.ErrorIs(t, err, drive.ErrInvalidSpace)

	dir, err := f.svc.SpaceDir(acme)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(f.root, "acme", "site"), dir)
	dir, err = f.svc.SpaceDir(drive.Shared)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(f.root, "shared"), dir)

	noRemote := newFixture(t, stubGit{err: errors.New("not a git repository")})
	_, err = noRemote.svc.ResolveSpace(ctx, "", false)
	require.ErrorContains(t, err, "detect remote")
	badRemote := newFixture(t, stubGit{remote: "nonsense"})
	_, err = badRemote.svc.ResolveSpace(ctx, "", false)
	require.ErrorContains(t, err, "could not extract owner/repo")
}

// Search.

func TestSearchMatchesPathAndTitleAcrossSpaces(t *testing.T) {
	f := newFixture(t, nil)
	ctx := t.Context()
	f.write(t, acme, "plans/2026-10-11-drive.md", "---\ntitle: Hive Drive\n---\n")
	f.write(t, acme, "research/notes.md", "---\ntitle: Drive research\n---\n")
	f.write(t, acme, "canvases/perf.canvas.json", `{"title":"Perf drive","blocks":[]}`)
	f.write(t, ws, "drive-ideas.md", "x")
	f.write(t, drive.Shared, "misc/other.md", "x")

	hits, err := f.svc.Search(ctx, "DRIVE", 0)
	require.NoError(t, err)
	got := make([]string, 0, len(hits))
	for _, hit := range hits {
		got = append(got, hit.Space.Key+":"+hit.Entry.Path)
		assert.Nil(t, hit.Entry.Children)
	}
	assert.Equal(t, []string{
		"acme/site:canvases/perf.canvas.json",
		"acme/site:plans/2026-10-11-drive.md",
		"acme/site:research/notes.md",
		"workspaces/ws:drive-ideas.md",
	}, got, "spaces in listing order, paths sorted inside a space")

	hits, err = f.svc.Search(ctx, "drive", 2)
	require.NoError(t, err)
	assert.Len(t, hits, 2)

	hits, err = f.svc.Search(ctx, "plans", 0)
	require.NoError(t, err)
	require.Len(t, hits, 2, "a folder matches by path too")
	assert.Equal(t, drive.KindDir, hits[0].Entry.Kind)

	hits, err = f.svc.Search(ctx, "   ", 0)
	require.NoError(t, err)
	assert.Empty(t, hits)
}

func TestSearchMatchesEveryWordAndTreatsHyphensAsSpaces(t *testing.T) {
	f := newFixture(t, nil)
	ctx := t.Context()
	f.write(t, acme, "design-docs/2026-06-12-session-lifecycle.md", "x")
	f.write(t, acme, "design-docs/session_notes.md", "---\ntitle: Lifecycle of a Chat\n---\n")
	f.write(t, acme, "design-docs/other.md", "x")

	search := func(query string) []string {
		t.Helper()
		hits, err := f.svc.Search(ctx, query, 0)
		require.NoError(t, err)
		got := []string{}
		for _, hit := range hits {
			got = append(got, hit.Entry.Path)
		}
		return got
	}
	assert.Equal(t, []string{"design-docs/2026-06-12-session-lifecycle.md", "design-docs/session_notes.md"}, search("session lifecycle"), "a word may match the path or the title")
	assert.Equal(t, search("session lifecycle"), search("session-lifecycle"), "a hyphen in the query splits words too")
	assert.Equal(t, []string{"design-docs/2026-06-12-session-lifecycle.md"}, search("design docs 2026"))
	assert.Empty(t, search("session nothing"))
}

// Reconfigure.

func TestReconfigureDropsTheSnapshotOnlyWhenTheRootMoves(t *testing.T) {
	f := newFixture(t, nil)
	ctx := t.Context()
	f.write(t, acme, "plans/a.md", "x")
	root, err := f.svc.Tree(ctx, acme)
	require.NoError(t, err)
	require.Len(t, root.Children, 1)

	same := &config.Config{Context: config.ContextConfig{BaseDir: f.root, SymlinkName: ".ctx"}}
	f.svc.Reconfigure(same, stubGit{})
	f.write(t, acme, "plans/b.md", "x")
	root, err = f.svc.Tree(ctx, acme)
	require.NoError(t, err)
	assert.Equal(t, []string{"plans/a.md"}, paths(root.Children[0].Children), "the same root keeps the snapshot")

	moved := &config.Config{Context: config.ContextConfig{BaseDir: t.TempDir()}}
	f.svc.Reconfigure(moved, stubGit{})
	assert.Equal(t, moved.ContextDir(), f.svc.Root())
	root, err = f.svc.Tree(ctx, acme)
	require.NoError(t, err)
	assert.Empty(t, root.Children, "a moved root starts over")
}

func TestReconfigureToANewRootPublishesEveryScannedSpace(t *testing.T) {
	f := newFixture(t, nil)
	ctx := t.Context()
	for _, space := range []drive.Space{acme, ws} {
		_, err := f.svc.Tree(ctx, space)
		require.NoError(t, err)
	}
	f.svc.Reconfigure(&config.Config{Context: config.ContextConfig{BaseDir: f.root, SymlinkName: ".ctx"}}, stubGit{})
	assert.Empty(t, f.changes(), "the same root changes nothing")

	f.svc.Reconfigure(&config.Config{Context: config.ContextConfig{BaseDir: t.TempDir()}}, stubGit{})
	assert.Equal(t, []events.DriveChangedPayload{{Space: acme.Key}, {Space: ws.Key}}, f.changes())
}

// Legacy import.

func TestImportLegacyCanvasesMovesRewritesAndRemovesTheEmptySource(t *testing.T) {
	f := newFixture(t, nil)
	ctx := t.Context()
	from := filepath.Join(t.TempDir(), "canvases")
	require.NoError(t, os.MkdirAll(from, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(from, "plan.json"), []byte(`{"workspace":"hive","name":"plan","title":"Legacy","session":42,"createdAt":1,"updatedAt":2,"blocks":[{"id":"a","kind":"markdown","body":"x","createdAt":1,"updatedAt":1}]}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(from, "report.canvas.json"), []byte(`{"blocks":[]}`), 0o644))
	f.write(t, ws, "canvases/taken.canvas.json", `{"title":"Already here","blocks":[]}`)
	require.NoError(t, os.WriteFile(filepath.Join(from, "taken.json"), []byte(`{"blocks":[]}`), 0o644))

	count, err := f.svc.ImportLegacyCanvases(ctx, from, ws)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	c, ok, err := f.svc.LoadCanvas(ctx, ws, "canvases/plan.canvas.json")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "Legacy", c.Title)
	assert.Equal(t, "42", c.Session)
	assert.Len(t, c.Blocks, 1)
	data, err := os.ReadFile(filepath.Join(f.root, "workspaces", "ws", "canvases", "plan.canvas.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), `"space": "workspaces/ws"`)
	assert.Contains(t, string(data), `"path": "canvases/plan.canvas.json"`)
	assert.NotContains(t, string(data), `"workspace"`)
	assert.FileExists(t, filepath.Join(f.root, "workspaces", "ws", "canvases", "report.canvas.json"), "a .canvas.json source keeps its stem")

	c, _, err = f.svc.LoadCanvas(ctx, ws, "canvases/taken.canvas.json")
	require.NoError(t, err)
	assert.Equal(t, "Already here", c.Title, "an existing target is kept")
	assert.FileExists(t, filepath.Join(from, "taken.json"), "a skipped source stays")
	assert.NoFileExists(t, filepath.Join(from, "plan.json"))

	count, err = f.svc.ImportLegacyCanvases(ctx, from, ws)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "a second run finds nothing to move")

	changes := f.changes()
	require.Len(t, changes, 1)
	assert.Equal(t, ws.Key, changes[0].Space)
	assert.ElementsMatch(t, []string{"canvases/plan.canvas.json", "canvases/report.canvas.json"}, changes[0].Paths)
}

func TestImportLegacyCanvasesRemovesAnEmptiedSourceAndLeavesTheRest(t *testing.T) {
	f := newFixture(t, nil)
	ctx := t.Context()
	from := filepath.Join(t.TempDir(), "canvases")
	require.NoError(t, os.MkdirAll(from, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(from, "a.json"), []byte(`{"blocks":[]}`), 0o644))

	count, err := f.svc.ImportLegacyCanvases(ctx, from, drive.Shared)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.NoDirExists(t, from)

	kept := filepath.Join(t.TempDir(), "canvases")
	require.NoError(t, os.MkdirAll(kept, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(kept, "settings.json"), []byte(`{"theme":"dark"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(kept, "b.json"), []byte(`{"blocks":[]}`), 0o644))
	count, err = f.svc.ImportLegacyCanvases(ctx, kept, drive.Shared)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.FileExists(t, filepath.Join(kept, "settings.json"), "a json that is not a canvas stays, and so does its directory")

	count, err = f.svc.ImportLegacyCanvases(ctx, filepath.Join(t.TempDir(), "missing"), drive.Shared)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	root, err := f.svc.Tree(ctx, drive.Shared)
	require.NoError(t, err)
	canvases, ok := drive.Find(root, "canvases")
	require.True(t, ok)
	assert.Equal(t, []string{"canvases/a.canvas.json", "canvases/b.canvas.json"}, paths(canvases.Children))
}

// Context directory operations.

func TestInitCreatesTheStandardFoldersOnce(t *testing.T) {
	f := newFixture(t, nil)
	dir, err := f.svc.SpaceDir(acme)
	require.NoError(t, err)

	created, err := f.svc.Init(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"research", "plans", "references"}, created)
	created, err = f.svc.Init(dir)
	require.NoError(t, err)
	assert.Empty(t, created)

	root, err := f.svc.Tree(t.Context(), acme)
	require.NoError(t, err)
	assert.Equal(t, []string{"plans", "references", "research"}, paths(root.Children))
	changes := f.changes()
	require.Len(t, changes, 1)
	assert.Equal(t, events.DriveChangedPayload{Space: "acme/site"}, changes[0], "no paths means the whole space")
}

func TestCreateSymlink(t *testing.T) {
	f := newFixture(t, nil)
	t.Chdir(t.TempDir())
	dir, err := f.svc.SpaceDir(acme)
	require.NoError(t, err)

	existed, err := f.svc.CreateSymlink(dir)
	require.NoError(t, err)
	assert.False(t, existed)
	target, err := os.Readlink(".hive")
	require.NoError(t, err)
	assert.Equal(t, dir, target)

	existed, err = f.svc.CreateSymlink(dir)
	require.NoError(t, err)
	assert.True(t, existed)

	_, err = f.svc.CreateSymlink(filepath.Join(f.root, "shared"))
	require.ErrorContains(t, err, "points to")
	require.NoError(t, os.Remove(".hive"))
	require.NoError(t, os.Mkdir(".hive", 0o755))
	_, err = f.svc.CreateSymlink(dir)
	require.ErrorContains(t, err, "not a symlink")
}

func TestPruneRemovesOldEntriesAndKeepsCanvases(t *testing.T) {
	f := newFixture(t, nil)
	dir, err := f.svc.SpaceDir(acme)
	require.NoError(t, err)
	old := time.Now().Add(-48 * time.Hour)
	for _, name := range []string{"plans", "notes.md", drive.CanvasesDir} {
		path := filepath.Join(dir, name)
		if filepath.Ext(name) == "" {
			require.NoError(t, os.MkdirAll(path, 0o755))
		} else {
			require.NoError(t, os.MkdirAll(dir, 0o755))
			require.NoError(t, os.WriteFile(path, []byte("x"), 0o644))
		}
		require.NoError(t, os.Chtimes(path, old, old))
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "fresh.md"), []byte("x"), 0o644))
	_, err = f.svc.Tree(t.Context(), acme)
	require.NoError(t, err)

	removed, err := f.svc.Prune(dir, 24*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 2, removed)
	assert.NoDirExists(t, filepath.Join(dir, "plans"))
	assert.NoFileExists(t, filepath.Join(dir, "notes.md"))
	assert.FileExists(t, filepath.Join(dir, "fresh.md"))
	assert.DirExists(t, filepath.Join(dir, drive.CanvasesDir), "canvases are durable output, not stale context")

	root, err := f.svc.Tree(t.Context(), acme)
	require.NoError(t, err)
	assert.Equal(t, []string{"canvases", "fresh.md"}, paths(root.Children), "a prune drops the snapshot")
	assert.Len(t, f.changes(), 1)

	removed, err = f.svc.Prune(filepath.Join(f.root, "nowhere"), time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 0, removed)
	assert.Len(t, f.changes(), 1, "nothing removed, nothing published")
}

func TestPruneKeepsAFolderHoldingACanvasAtAnyDepth(t *testing.T) {
	f := newFixture(t, nil)
	dir, err := f.svc.SpaceDir(acme)
	require.NoError(t, err)
	f.write(t, acme, "reports/2026-10/perf.canvas.json", `{"blocks":[]}`)
	f.write(t, acme, "reports/2026-10/notes.md", "x")
	f.write(t, acme, "old/notes.md", "x")
	f.write(t, acme, "root.canvas.json", `{"blocks":[]}`)
	old := time.Now().Add(-48 * time.Hour)
	for _, name := range []string{"reports", "old", "root.canvas.json"} {
		require.NoError(t, os.Chtimes(filepath.Join(dir, name), old, old))
	}

	removed, err := f.svc.Prune(dir, 24*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 1, removed)
	assert.NoDirExists(t, filepath.Join(dir, "old"))
	assert.FileExists(t, filepath.Join(dir, "reports", "2026-10", "perf.canvas.json"))
	assert.FileExists(t, filepath.Join(dir, "reports", "2026-10", "notes.md"), "a folder holding a canvas is kept whole")
	assert.FileExists(t, filepath.Join(dir, "root.canvas.json"))
}

func TestEveryMutationPublishesOnceWithItsAuthor(t *testing.T) {
	f := newFixture(t, nil)
	ctx := t.Context()
	author := drive.Author{Session: "chat-1", HiveSession: "hive-1"}
	steps := []struct {
		name string
		run  func() error
	}{
		{"write", func() error { _, err := f.svc.Write(ctx, acme, "plans/a.md", author, []byte("x")); return err }},
		{"create folder", func() error { _, err := f.svc.CreateFolder(ctx, acme, "reports", author); return err }},
		{"move", func() error { _, err := f.svc.Move(ctx, acme, "plans/a.md", "reports/a.md", author); return err }},
		{"upsert", func() error {
			_, err := f.svc.UpsertBlocks(ctx, acme, plan, author, "", "", drive.Block{ID: "a", Kind: drive.BlockKindMarkdown, Body: "x"})
			return err
		}},
		{"remove block", func() error { _, _, err := f.svc.RemoveBlock(ctx, acme, plan, "a", author); return err }},
		{"set front matter", func() error {
			_, err := f.svc.SetCanvasFrontmatter(ctx, acme, plan, author, drive.Frontmatter{"k": "v"})
			return err
		}},
		{"clear", func() error { _, err := f.svc.ClearCanvas(ctx, acme, plan, author); return err }},
		{"delete", func() error { return f.svc.Delete(ctx, acme, "reports/a.md", author) }},
	}
	for i, step := range steps {
		require.NoError(t, step.run(), step.name)
		changes := f.changes()
		require.Len(t, changes, i+1, "%s publishes exactly once", step.name)
		assert.Equal(t, "acme/site", changes[i].Space, step.name)
		assert.NotEmpty(t, changes[i].Paths, step.name)
		assert.Equal(t, author, changes[i].Author, step.name)
	}
}
