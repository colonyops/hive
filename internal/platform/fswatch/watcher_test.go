package fswatch

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testDebounce = 20 * time.Millisecond
	waitTimeout  = 10 * time.Second
	quietPeriod  = 15 * testDebounce
)

func newWatcher(t *testing.T, root string, opts ...Option) *Watcher {
	t.Helper()
	opts = append([]Option{WithDebounce(testDebounce)}, opts...)
	w, err := New(zerolog.Nop(), root, opts...)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, w.Close()) })
	return w
}

// waitFor receives Changes until every dir has appeared in one, or fails at
// the deadline.
func waitFor(t *testing.T, w *Watcher, dirs ...string) {
	t.Helper()
	missing := map[string]struct{}{}
	for _, dir := range dirs {
		missing[dir] = struct{}{}
	}
	deadline := time.After(waitTimeout)
	for len(missing) > 0 {
		select {
		case change, ok := <-w.Changes():
			require.True(t, ok, "changes closed while waiting for %v", dirs)
			for _, dir := range change.Dirs {
				delete(missing, dir)
			}
		case <-deadline:
			t.Fatalf("no change named %v within %s", missing, waitTimeout)
		}
	}
}

// assertNeverNamed fails if a Change names dir during the quiet period.
// Changes naming other directories are ignored, because a backend may
// deliver a late event for an earlier operation.
func assertNeverNamed(t *testing.T, w *Watcher, dir string) {
	t.Helper()
	timeout := time.After(quietPeriod)
	for {
		select {
		case change, ok := <-w.Changes():
			require.True(t, ok, "changes closed")
			assert.NotContains(t, change.Dirs, dir)
		case <-timeout:
			return
		}
	}
}

func assertQuiet(t *testing.T, w *Watcher, period time.Duration) {
	t.Helper()
	select {
	case change, ok := <-w.Changes():
		require.True(t, ok, "changes closed")
		t.Fatalf("unexpected change: %v", change.Dirs)
	case <-time.After(period):
	}
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(time.Now().String()), 0o600))
}

func TestFileOperationsNameTheParent(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	w := newWatcher(t, root)

	path := filepath.Join(root, "a.md")
	writeFile(t, path)
	waitFor(t, w, root)

	writeFile(t, path)
	waitFor(t, w, root)

	renamed := filepath.Join(root, "b.md")
	require.NoError(t, os.Rename(path, renamed))
	waitFor(t, w, root)

	require.NoError(t, os.Remove(renamed))
	waitFor(t, w, root)
}

func TestRenameAcrossDirectoriesNamesBoth(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	require.NoError(t, os.Mkdir(sub, 0o755))
	path := filepath.Join(root, "a.md")
	writeFile(t, path)
	w := newWatcher(t, root)

	require.NoError(t, os.Rename(path, filepath.Join(sub, "a.md")))
	waitFor(t, w, root, sub)
}

func TestCreatedDirectoryIsWalkedAndWatched(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	w := newWatcher(t, root)

	sub := filepath.Join(root, "sub")
	deep := filepath.Join(sub, "deep")
	require.NoError(t, os.MkdirAll(deep, 0o755))
	writeFile(t, filepath.Join(deep, "early.md"))
	waitFor(t, w, root, sub)

	writeFile(t, filepath.Join(deep, "late.md"))
	waitFor(t, w, deep)
}

func TestRemovedDirectoryDropsItsWatches(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	deep := filepath.Join(sub, "deep")
	require.NoError(t, os.MkdirAll(deep, 0o755))
	w := newWatcher(t, root)
	require.Contains(t, w.fsw.WatchList(), deep)

	require.NoError(t, os.RemoveAll(sub))
	waitFor(t, w, root)

	assert.NotContains(t, w.fsw.WatchList(), sub)
	assert.NotContains(t, w.fsw.WatchList(), deep)
}

func TestRenamedDirectoryMovesItsWatches(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	deep := filepath.Join(sub, "deep")
	require.NoError(t, os.MkdirAll(deep, 0o755))
	w := newWatcher(t, root)

	other := filepath.Join(root, "other")
	require.NoError(t, os.Rename(sub, other))
	waitFor(t, w, root, other)

	otherDeep := filepath.Join(other, "deep")
	writeFile(t, filepath.Join(otherDeep, "a.md"))
	waitFor(t, w, otherDeep)

	list := w.fsw.WatchList()
	assert.NotContains(t, list, sub)
	assert.NotContains(t, list, deep)
	assert.Contains(t, list, other)
	assert.Contains(t, list, otherDeep)
}

func TestDotDirectoriesAreNotWatched(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	git := filepath.Join(root, ".git")
	require.NoError(t, os.Mkdir(git, 0o755))
	w := newWatcher(t, root)
	assert.NotContains(t, w.fsw.WatchList(), git)

	writeFile(t, filepath.Join(git, "HEAD"))
	assertNeverNamed(t, w, git)

	obsidian := filepath.Join(root, ".obsidian")
	require.NoError(t, os.Mkdir(obsidian, 0o755))
	waitFor(t, w, root)
	assert.NotContains(t, w.fsw.WatchList(), obsidian)

	writeFile(t, filepath.Join(obsidian, "workspace.json"))
	assertNeverNamed(t, w, obsidian)
}

func TestWithSkipDirReplacesTheDefault(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	hidden := filepath.Join(root, ".hidden")
	skipped := filepath.Join(root, "skip")
	require.NoError(t, os.Mkdir(hidden, 0o755))
	require.NoError(t, os.Mkdir(skipped, 0o755))
	w := newWatcher(t, root, WithSkipDir(func(name string) bool { return name == "skip" }))

	assert.Contains(t, w.fsw.WatchList(), hidden)
	assert.NotContains(t, w.fsw.WatchList(), skipped)

	writeFile(t, filepath.Join(hidden, "a.md"))
	waitFor(t, w, hidden)
}

func TestSymlinkedDirectoryIsNotFollowed(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	target := t.TempDir()
	link := filepath.Join(root, "link")
	require.NoError(t, os.Symlink(target, link))
	w := newWatcher(t, root)

	list := w.fsw.WatchList()
	assert.NotContains(t, list, link)
	assert.NotContains(t, list, target)

	writeFile(t, filepath.Join(target, "a.md"))
	assertNeverNamed(t, w, link)
	assertNeverNamed(t, w, target)
}

func TestSymlinkedRootReportsUnderTheLink(t *testing.T) {
	t.Parallel()
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "root")
	require.NoError(t, os.Symlink(target, link))
	w := newWatcher(t, link)

	writeFile(t, filepath.Join(target, "a.md"))
	waitFor(t, w, link)
}

func TestWritesInsideTheWindowYieldOneChange(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "a.md")
	writeFile(t, path)
	// A window wide enough that a loaded runner cannot split two back-to-back
	// writes across it.
	window := 10 * testDebounce
	w := newWatcher(t, root, WithDebounce(window))

	writeFile(t, path)
	writeFile(t, path)
	waitFor(t, w, root)
	assertQuiet(t, w, 3*window)
}

func TestSlowConsumerReceivesOneMergedChange(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	var dirs []string
	for _, name := range []string{"a", "b", "c"} {
		dir := filepath.Join(root, name)
		require.NoError(t, os.Mkdir(dir, 0o755))
		dirs = append(dirs, dir)
	}
	w := newWatcher(t, root)

	for _, dir := range dirs {
		writeFile(t, filepath.Join(dir, "x.md"))
		time.Sleep(5 * testDebounce)
	}

	select {
	case change, ok := <-w.Changes():
		require.True(t, ok)
		assert.Equal(t, dirs, change.Dirs)
	case <-time.After(waitTimeout):
		t.Fatal("no change")
	}
	assertQuiet(t, w, quietPeriod)
}

func TestCloseClosesChanges(t *testing.T) {
	t.Parallel()
	w, err := New(zerolog.Nop(), t.TempDir(), WithDebounce(testDebounce))
	require.NoError(t, err)

	require.NoError(t, w.Close())
	select {
	case _, ok := <-w.Changes():
		assert.False(t, ok, "changes still open after Close")
	case <-time.After(waitTimeout):
		t.Fatal("changes not closed")
	}
	require.NoError(t, w.Close())
}

func TestNewRejectsMissingAndFileRoots(t *testing.T) {
	t.Parallel()
	_, err := New(zerolog.Nop(), filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)

	file := filepath.Join(t.TempDir(), "file")
	writeFile(t, file)
	_, err = New(zerolog.Nop(), file)
	require.Error(t, err)
}
