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
)

const (
	testDebounce = 20 * time.Millisecond
	// quietPeriod is how long a negative test waits for a change that must
	// not arrive: several debounce windows.
	quietPeriod = 10 * testDebounce
	waitTimeout = 10 * time.Second
)

// watching is a fixture whose watcher runs with a short debounce and whose
// acme space, holding an empty plans folder, is already scanned.
func watching(t *testing.T) fixture {
	t.Helper()
	f := newFixture(t, nil)
	f.svc.debounce = testDebounce
	require.NoError(t, os.MkdirAll(filepath.Join(f.root, "acme", "site", "plans"), 0o755))
	_, err := f.svc.Tree(t.Context(), acme)
	require.NoError(t, err)
	require.NoError(t, f.svc.Watch(t.Context()))
	t.Cleanup(func() { require.NoError(t, f.svc.Close()) })
	return f
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

	_, err := f.svc.Write(t.Context(), acme, "plans/a.md", []byte("# A\n"))
	require.NoError(t, err)
	f.write(t, acme, "plans/b.md", "# B\n")
	waitForPath(t, f, acme, "plans/b.md", true)

	assert.Equal(t, []events.DriveChangedPayload{
		{Space: acme.Key, Paths: []string{"plans/a.md"}},
		{Space: acme.Key, Paths: []string{"plans/b.md"}},
	}, f.changes())
}

func TestWatchIgnoresASpaceNoOneHasRead(t *testing.T) {
	f := watching(t)

	f.write(t, ws, "notes.md", "# N\n")
	time.Sleep(quietPeriod)
	assert.Empty(t, f.changes())

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

func TestCloseStopsTheWatcher(t *testing.T) {
	f := watching(t)
	require.NoError(t, f.svc.Close())
	require.NoError(t, f.svc.Close(), "idempotent")

	f.write(t, acme, "plans/a.md", "# A\n")
	time.Sleep(quietPeriod)
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

	cancel()
	f.svc.watchers.Wait()
	f.write(t, acme, "plans/a.md", "# A\n")
	time.Sleep(quietPeriod)
	assert.Empty(t, f.changes())
	require.NoError(t, f.svc.Close())
}

func TestReconfigureMovesTheWatcherToTheNewRoot(t *testing.T) {
	f := watching(t)
	newRoot := t.TempDir()
	f.svc.Reconfigure(&config.Config{Context: config.ContextConfig{BaseDir: newRoot, SymlinkName: ".hive"}}, nil)
	require.Equal(t, newRoot, f.svc.Root())

	_, err := f.svc.Tree(t.Context(), acme)
	require.NoError(t, err)
	moved := fixture{svc: f.svc, root: newRoot, bus: f.bus}
	moved.write(t, acme, "plans/new.md", "# New\n")
	waitForPath(t, moved, acme, "plans/new.md", true)

	f.write(t, acme, "plans/old.md", "# Old\n")
	time.Sleep(quietPeriod)
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
