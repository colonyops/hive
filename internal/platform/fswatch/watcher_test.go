package fswatch

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testDebounce = 20 * time.Millisecond
	waitTimeout  = 10 * time.Second
)

// withSources feeds the pump from events and errs instead of fsnotify, so a
// test decides which events are lost.
func withSources(events <-chan fsnotify.Event, errs <-chan error) Option {
	return func(o *options) {
		o.events = events
		o.errors = errs
	}
}

func withBudget(n int) Option {
	return func(o *options) { o.budget = n }
}

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

// assertNeverNamed writes a file into control, a watched directory, and
// fails if any Change up to the one naming control names dir. The pump
// handles events in order, so once control is named every earlier event has
// been seen.
func assertNeverNamed(t *testing.T, w *Watcher, dir, control string) {
	t.Helper()
	writeFile(t, filepath.Join(control, "control-"+time.Now().Format("150405.000000000")))
	deadline := time.After(waitTimeout)
	for {
		select {
		case change, ok := <-w.Changes():
			require.True(t, ok, "changes closed")
			assert.NotContains(t, change.Dirs, dir)
			if slices.Contains(change.Dirs, control) {
				return
			}
		case <-deadline:
			t.Fatalf("no change named the control %s within %s", control, waitTimeout)
		}
	}
}

// nextChange is the next Change, or fails at the deadline.
func nextChange(t *testing.T, w *Watcher) Change {
	t.Helper()
	select {
	case change, ok := <-w.Changes():
		require.True(t, ok, "changes closed")
		return change
	case <-time.After(waitTimeout):
		t.Fatalf("no change within %s", waitTimeout)
		return Change{}
	}
}

// waitClosed fails unless Changes closes before the deadline.
func waitClosed(t *testing.T, w *Watcher) {
	t.Helper()
	deadline := time.After(waitTimeout)
	for {
		select {
		case _, ok := <-w.Changes():
			if !ok {
				return
			}
		case <-deadline:
			t.Fatalf("changes not closed within %s", waitTimeout)
		}
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
	assertNeverNamed(t, w, git, root)

	obsidian := filepath.Join(root, ".obsidian")
	require.NoError(t, os.Mkdir(obsidian, 0o755))
	waitFor(t, w, root)
	assert.NotContains(t, w.fsw.WatchList(), obsidian)

	writeFile(t, filepath.Join(obsidian, "workspace.json"))
	assertNeverNamed(t, w, obsidian, root)
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
	assertNeverNamed(t, w, link, root)
	writeFile(t, filepath.Join(target, "b.md"))
	assertNeverNamed(t, w, target, root)
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
	other := filepath.Join(root, "other")
	require.NoError(t, os.Mkdir(other, 0o755))
	path := filepath.Join(root, "a.md")
	writeFile(t, path)
	// A window wide enough that a loaded runner cannot split two back-to-back
	// writes across it.
	window := 10 * testDebounce
	w := newWatcher(t, root, WithDebounce(window))

	writeFile(t, path)
	writeFile(t, path)
	waitFor(t, w, root)

	writeFile(t, filepath.Join(other, "control.md"))
	assert.Equal(t, []string{other}, nextChange(t, w).Dirs, "no second change for root came before the control")
}

// The consumer reads nothing until every write is done, so windows that
// close in between merge. The sleeps only make separate windows likely; the
// assertion holds however the writes fall into windows.
func TestSlowConsumerReceivesMergedChanges(t *testing.T) {
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

	seen := map[string]struct{}{}
	received := 0
	for len(seen) < len(dirs) {
		change := nextChange(t, w)
		received++
		for _, dir := range change.Dirs {
			seen[dir] = struct{}{}
		}
	}
	assert.Less(t, received, len(dirs), "windows that closed while the consumer was busy merged")
}

func TestOverflowResyncsEveryWatchedDirectory(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	require.NoError(t, os.Mkdir(sub, 0o755))
	events := make(chan fsnotify.Event)
	errs := make(chan error)
	w := newWatcher(t, root, withSources(events, errs))

	gap := filepath.Join(root, "gap")
	deep := filepath.Join(gap, "deep")
	require.NoError(t, os.MkdirAll(deep, 0o755))
	require.NoError(t, os.RemoveAll(sub))
	errs <- fsnotify.ErrEventOverflow

	change := nextChange(t, w)
	assert.True(t, change.Resync)
	assert.Equal(t, []string{root, gap, deep}, change.Dirs, "every watched directory, the ones made during the gap included")
	assert.Contains(t, w.fsw.WatchList(), deep)
	assert.NotContains(t, w.fsw.WatchList(), sub)
}

func TestRemovedRootClosesChanges(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "root")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "sub"), 0o755))
	w := newWatcher(t, root)

	require.NoError(t, os.RemoveAll(root))
	waitClosed(t, w)
}

func TestCloseClosesChanges(t *testing.T) {
	t.Parallel()
	w, err := New(zerolog.Nop(), t.TempDir(), WithDebounce(testDebounce))
	require.NoError(t, err)

	require.NoError(t, w.Close())
	waitClosed(t, w)
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

func TestDirectoriesPastTheBudgetAreNotWatched(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	require.NoError(t, os.Mkdir(sub, 0o755))
	writeFile(t, filepath.Join(root, "a.md"))
	w := newWatcher(t, root, withBudget(1))

	list := w.fsw.WatchList()
	assert.Contains(t, list, root)
	assert.NotContains(t, list, sub)
	writeFile(t, filepath.Join(sub, "b.md"))
	assertNeverNamed(t, w, sub, root)
}

func TestOverflowPastTheBudgetStillResyncs(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "sub"), 0o755))
	events := make(chan fsnotify.Event)
	errs := make(chan error)
	w := newWatcher(t, root, withSources(events, errs), withBudget(1))

	errs <- fsnotify.ErrEventOverflow

	change := nextChange(t, w)
	assert.True(t, change.Resync)
	assert.Equal(t, []string{root}, change.Dirs)
}
