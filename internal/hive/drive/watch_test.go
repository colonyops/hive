package drive

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/hive/events"
	"github.com/colonyops/hive/internal/platform/fswatch"
)

const (
	testDebounce = 20 * time.Millisecond
	waitTimeout  = 10 * time.Second
)

// watching is a fixture whose watcher runs with a short debounce and whose
// acme space, holding an empty plans folder, is already scanned.
func watching(t *testing.T) fixture {
	t.Helper()
	f := newFixture(t, nil)
	f.svc.debounce = testDebounce
	f.svc.rootPoll = testDebounce
	require.NoError(t, os.MkdirAll(filepath.Join(f.root, "acme", "site", "plans"), 0o755))
	_, err := f.svc.Tree(t.Context(), acme)
	require.NoError(t, err)
	require.NoError(t, f.svc.Watch(t.Context()))
	t.Cleanup(func() { require.NoError(t, f.svc.Close()) })
	return f
}

// waitClosed fails unless w's channel closes before the deadline, which
// proves its pump has stopped.
func waitClosed(t *testing.T, w *watcher) {
	t.Helper()
	deadline := time.After(waitTimeout)
	for {
		select {
		case _, ok := <-w.fs.Changes():
			if !ok {
				return
			}
		case <-deadline:
			t.Fatalf("watcher channel not closed within %s", waitTimeout)
		}
	}
}

// waitForPath polls Tree until path is present, or absent when present is
// false.
func waitForPath(t *testing.T, f fixture, space drive.Space, path string, present bool) {
	t.Helper()
	require.Eventuallyf(t, func() bool {
		root, err := f.svc.Tree(context.Background(), space)
		if err != nil {
			return false
		}
		_, ok := drive.Find(root, path)
		return ok == present
	}, waitTimeout, 5*time.Millisecond, "%s/%s present=%v", space.Key, path, present)
}

func TestWatchIndexesADocumentWrittenBehindTheService(t *testing.T) {
	f := watching(t)

	f.write(t, acme, "plans/a.md", "---\ntitle: A\n---\n# A\n")
	waitForPath(t, f, acme, "plans/a.md", true)

	root, err := f.svc.Tree(t.Context(), acme)
	require.NoError(t, err)
	entry, _ := drive.Find(root, "plans/a.md")
	assert.Equal(t, drive.KindMarkdown, entry.Kind)
	assert.Equal(t, "A", entry.Title)
	assert.Equal(t, []events.DriveChangedPayload{{Space: acme.Key, Paths: []string{"plans/a.md"}}}, f.changes())
}

func TestWatchWalksANewFolder(t *testing.T) {
	f := watching(t)

	f.write(t, acme, "reports/2026/q1.md", "# Q1\n")
	waitForPath(t, f, acme, "reports/2026/q1.md", true)

	root, err := f.svc.Tree(t.Context(), acme)
	require.NoError(t, err)
	folder, ok := drive.Find(root, "reports/2026")
	require.True(t, ok)
	assert.Equal(t, drive.KindDir, folder.Kind)
	changes := f.changes()
	require.NotEmpty(t, changes)
	for _, change := range changes {
		assert.Equal(t, acme.Key, change.Space)
	}
}

func TestWatchRemovesADeletedDocument(t *testing.T) {
	f := watching(t)
	f.write(t, acme, "plans/a.md", "# A\n")
	waitForPath(t, f, acme, "plans/a.md", true)

	require.NoError(t, os.Remove(filepath.Join(f.root, "acme", "site", "plans", "a.md")))
	waitForPath(t, f, acme, "plans/a.md", false)

	assert.Equal(t, []events.DriveChangedPayload{
		{Space: acme.Key, Paths: []string{"plans/a.md"}},
		{Space: acme.Key, Paths: []string{"plans/a.md"}},
	}, f.changes())
}

func TestWatchDropsASpaceWhoseDirectoryVanished(t *testing.T) {
	f := watching(t)
	f.write(t, acme, "plans/a.md", "# A\n")
	waitForPath(t, f, acme, "plans/a.md", true)

	require.NoError(t, os.RemoveAll(filepath.Join(f.root, "acme", "site")))
	waitForPath(t, f, acme, "plans", false)

	changes := f.changes()
	require.NotEmpty(t, changes)
	assert.Equal(t, events.DriveChangedPayload{Space: acme.Key}, changes[len(changes)-1], "the whole space changed")
}

// The service's own write files the entry and publishes before the watcher
// sees the file, so the rescan finds nothing new. A later write behind the
// service's back proves the watcher ran.
func TestWatchDoesNotRepublishTheServicesOwnWrite(t *testing.T) {
	f := watching(t)

	_, err := f.svc.Write(t.Context(), acme, "plans/a.md", drive.Author{}, []byte("# A\n"))
	require.NoError(t, err)
	f.write(t, acme, "plans/b.md", "# B\n")
	waitForPath(t, f, acme, "plans/b.md", true)

	assert.Equal(t, []events.DriveChangedPayload{
		{Space: acme.Key, Paths: []string{"plans/a.md"}},
		{Space: acme.Key, Paths: []string{"plans/b.md"}},
	}, f.changes())
}

// The acme write comes after the ws write, so once the watcher has filed it,
// it has handled the ws write too.
func TestWatchIgnoresASpaceNoOneHasRead(t *testing.T) {
	f := watching(t)

	f.write(t, ws, "notes.md", "# N\n")
	f.write(t, acme, "plans/control.md", "# C\n")
	waitForPath(t, f, acme, "plans/control.md", true)
	assert.Equal(t, []events.DriveChangedPayload{{Space: acme.Key, Paths: []string{"plans/control.md"}}}, f.changes())

	root, err := f.svc.Tree(t.Context(), ws)
	require.NoError(t, err)
	_, ok := drive.Find(root, "notes.md")
	assert.True(t, ok, "scanned on first use")
}

func TestWatchTwiceIsANoop(t *testing.T) {
	f := watching(t)
	first := f.svc.watch
	require.NoError(t, f.svc.Watch(t.Context()))
	assert.Same(t, first, f.svc.watch)
}

func TestWatchCreatesTheRoot(t *testing.T) {
	f := newFixture(t, nil)
	root := filepath.Join(f.root, "fresh", "context")
	f.svc.cfg = &config.Config{Context: config.ContextConfig{BaseDir: root, SymlinkName: ".hive"}}

	require.NoError(t, f.svc.Watch(t.Context()))
	t.Cleanup(func() { require.NoError(t, f.svc.Close()) })
	assert.DirExists(t, root)
}

// Close waits for the goroutine that applies changes, so once it returns and
// the watcher's channel is closed, nothing can file a later write.
func TestCloseStopsTheWatcher(t *testing.T) {
	f := watching(t)
	w := f.svc.watch
	require.NoError(t, f.svc.Close())
	require.NoError(t, f.svc.Close(), "idempotent")
	waitClosed(t, w)

	f.write(t, acme, "plans/a.md", "# A\n")
	assert.Empty(t, f.changes())
	root, err := f.svc.Tree(t.Context(), acme)
	require.NoError(t, err)
	_, ok := drive.Find(root, "plans/a.md")
	assert.False(t, ok, "the snapshot is no longer kept current")
}

func TestCloseWithoutWatchIsANoop(t *testing.T) {
	f := newFixture(t, nil)
	require.NoError(t, f.svc.Close())
}

func TestWatchEndsWithItsContext(t *testing.T) {
	f := newFixture(t, nil)
	f.svc.debounce = testDebounce
	require.NoError(t, os.MkdirAll(filepath.Join(f.root, "acme", "site", "plans"), 0o755))
	_, err := f.svc.Tree(t.Context(), acme)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	require.NoError(t, f.svc.Watch(ctx))
	w := f.svc.watch

	cancel()
	f.svc.watchers.Wait()
	waitClosed(t, w)
	f.write(t, acme, "plans/a.md", "# A\n")
	assert.Empty(t, f.changes())
	require.NoError(t, f.svc.Close())
}

func TestReconfigureMovesTheWatcherToTheNewRoot(t *testing.T) {
	f := watching(t)
	old := f.svc.watch
	newRoot := t.TempDir()
	f.svc.Reconfigure(&config.Config{Context: config.ContextConfig{BaseDir: newRoot, SymlinkName: ".hive"}}, nil)
	require.Equal(t, newRoot, f.svc.Root())
	waitClosed(t, old)

	_, err := f.svc.Tree(t.Context(), acme)
	require.NoError(t, err)
	f.write(t, acme, "plans/old.md", "# Old\n")
	moved := fixture{svc: f.svc, root: newRoot, bus: f.bus}
	moved.write(t, acme, "plans/new.md", "# New\n")
	waitForPath(t, moved, acme, "plans/new.md", true)

	changes := f.changes()
	require.NotEmpty(t, changes)
	for _, change := range changes {
		assert.Equal(t, acme.Key, change.Space)
		assert.NotContains(t, change.Paths, "plans/old.md", "the old root is no longer watched")
	}
	root, err := f.svc.Tree(t.Context(), acme)
	require.NoError(t, err)
	_, ok := drive.Find(root, "plans/old.md")
	assert.False(t, ok)
}

func TestWatchLeavesALegacyCanvasWrittenLaterAlone(t *testing.T) {
	f := watching(t)

	f.write(t, acme, "canvases/x.json", `{"blocks":[]}`)
	waitForPath(t, f, acme, "canvases/x.json", true)

	root, err := f.svc.Tree(t.Context(), acme)
	require.NoError(t, err)
	entry, _ := drive.Find(root, "canvases/x.json")
	assert.Equal(t, drive.KindFile, entry.Kind)
	assert.FileExists(t, filepath.Join(f.root, "acme", "site", "canvases", "x.json"))
	assert.NoFileExists(t, filepath.Join(f.root, "acme", "site", "canvases", "x.canvas.json"), "the legacy rename runs only before the first scan")
}

func TestWatchRestartsWhenTheRootComesBack(t *testing.T) {
	f := watching(t)
	first := f.svc.watch

	require.NoError(t, os.RemoveAll(f.root))
	waitClosed(t, first)
	f.write(t, acme, "plans/a.md", "# A\n")
	waitForPath(t, f, acme, "plans/a.md", true)
	assert.NotSame(t, first, f.svc.watch)

	f.write(t, acme, "plans/b.md", "# B\n")
	waitForPath(t, f, acme, "plans/b.md", true)
}

func TestResyncRescansEveryScannedSpace(t *testing.T) {
	f := newFixture(t, nil)
	ctx := t.Context()
	for _, space := range []drive.Space{acme, ws} {
		_, err := f.svc.Tree(ctx, space)
		require.NoError(t, err)
	}
	f.write(t, acme, "plans/a.md", "# A\n")
	f.write(t, ws, "notes.md", "# N\n")

	f.svc.apply(&watcher{cfgRoot: f.svc.Root()}, fswatch.Change{Resync: true})

	assert.Equal(t, []events.DriveChangedPayload{{Space: acme.Key}, {Space: ws.Key}}, f.changes())
	root, err := f.svc.Tree(ctx, acme)
	require.NoError(t, err)
	_, ok := drive.Find(root, "plans/a.md")
	assert.True(t, ok)
	root, err = f.svc.Tree(ctx, ws)
	require.NoError(t, err)
	_, ok = drive.Find(root, "notes.md")
	assert.True(t, ok)
}

func TestSpaceAtResolvesThroughSymlinks(t *testing.T) {
	f := newFixture(t, nil)
	require.NoError(t, os.MkdirAll(filepath.Join(f.root, "acme", "site", "plans"), 0o755))
	checkout := t.TempDir()
	link := filepath.Join(checkout, ".hive")
	require.NoError(t, os.Symlink(filepath.Join(f.root, "acme", "site"), link))

	space, rel, ok := f.svc.SpaceAt(filepath.Join(link, "plans"))
	require.True(t, ok)
	assert.Equal(t, acme, space)
	assert.Equal(t, "plans", rel)

	require.NoError(t, os.MkdirAll(filepath.Join(f.root, "shared"), 0o755))
	space, rel, ok = f.svc.SpaceAt(filepath.Join(f.root, "shared"))
	require.True(t, ok)
	assert.Equal(t, drive.Shared, space)
	assert.Empty(t, rel)

	_, _, ok = f.svc.SpaceAt(f.root)
	assert.False(t, ok, "the root holds spaces, not documents")
	_, _, ok = f.svc.SpaceAt(filepath.Join(f.root, "acme"))
	assert.False(t, ok, "an owner directory holds spaces, not documents")
	_, _, ok = f.svc.SpaceAt(checkout)
	assert.False(t, ok, "outside the root")
}
