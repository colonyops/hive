package dirwatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testDebounce = 100 * time.Millisecond

func isYAML(path string) bool { return strings.HasSuffix(path, ".yaml") }

func startWatcher(t *testing.T, dir string, opts ...Option) (*Watcher, <-chan struct{}) {
	t.Helper()
	changed := make(chan struct{}, 16)
	opts = append([]Option{WithDebounce(testDebounce), WithComponent("test-watcher")}, opts...)
	w, err := New(dir, isYAML, func() { changed <- struct{}{} }, zerolog.Nop(), opts...)
	require.NoError(t, err)
	w.Start()
	t.Cleanup(w.Close)
	return w, changed
}

func waitForChange(t *testing.T, changed <-chan struct{}) {
	t.Helper()
	select {
	case <-changed:
	case <-time.After(5 * time.Second):
		t.Fatal("watcher did not fire")
	}
}

func assertNoChange(t *testing.T, changed <-chan struct{}) {
	t.Helper()
	select {
	case <-changed:
		t.Fatal("watcher fired unexpectedly")
	case <-time.After(4 * testDebounce):
	}
}

func TestWatcherFiresOnEachOperation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "a.yaml")
	_, changed := startWatcher(t, dir)

	// Create: the file does not exist when the watch starts.
	require.NoError(t, os.WriteFile(path, []byte("v: 1\n"), 0o600))
	waitForChange(t, changed)

	// Write to an existing file.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	require.NoError(t, err)
	_, err = f.WriteString("w: 2\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	waitForChange(t, changed)

	// Rename over: the atomic replace editors and the app use.
	tmp := filepath.Join(dir, "a.tmp")
	require.NoError(t, os.WriteFile(tmp, []byte("v: 3\n"), 0o600))
	require.NoError(t, os.Rename(tmp, path))
	waitForChange(t, changed)

	require.NoError(t, os.Remove(path))
	waitForChange(t, changed)
}

func TestWatcherIgnoresUnmatchedFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	_, changed := startWatcher(t, dir)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o600))
	assertNoChange(t, changed)
}

func TestWatcherCreatesMissingDir(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "hive", "desktop")
	_, changed := startWatcher(t, dir)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("v: 1\n"), 0o600))
	waitForChange(t, changed)
}

func TestWatcherDebounceResetsOnEachEvent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "a.yaml")
	_, changed := startWatcher(t, dir)

	// The burst outlasts one debounce period, so a callback here means the
	// timer was not reset by the later events.
	for i := range 6 {
		require.NoError(t, os.WriteFile(path, []byte{byte('0' + i)}, 0o600))
		time.Sleep(testDebounce / 3)
	}

	waitForChange(t, changed)
	assertNoChange(t, changed)
}

func TestWatcherIgnoresSubdirectoriesByDefault(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sub := filepath.Join(dir, "ws")
	require.NoError(t, os.Mkdir(sub, 0o700))
	_, changed := startWatcher(t, dir)

	require.NoError(t, os.WriteFile(filepath.Join(sub, "a.yaml"), []byte("v: 1\n"), 0o600))
	assertNoChange(t, changed)
}

func TestWatcherWithSubdirectories(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	existing := filepath.Join(dir, "existing")
	require.NoError(t, os.Mkdir(existing, 0o700))
	_, changed := startWatcher(t, dir, WithSubdirectories())

	require.NoError(t, os.WriteFile(filepath.Join(existing, "a.yaml"), []byte("v: 1\n"), 0o600))
	waitForChange(t, changed)

	// A new subdirectory changes the watch set, which calls back by itself.
	added := filepath.Join(dir, "added")
	require.NoError(t, os.Mkdir(added, 0o700))
	waitForChange(t, changed)

	require.NoError(t, os.WriteFile(filepath.Join(added, "a.yaml"), []byte("v: 1\n"), 0o600))
	waitForChange(t, changed)

	// Files two levels down stay out of reach.
	deep := filepath.Join(added, "deep")
	require.NoError(t, os.Mkdir(deep, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(deep, "a.yaml"), []byte("v: 1\n"), 0o600))
	assertNoChange(t, changed)
}

func TestWatcherCloseStopsDeliveryAndIsIdempotent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	w, changed := startWatcher(t, dir)

	w.Close()
	w.Close()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("v: 1\n"), 0o600))
	assertNoChange(t, changed)
}

func TestWatcherCloseWithoutStart(t *testing.T) {
	t.Parallel()

	w, err := New(t.TempDir(), isYAML, func() {}, zerolog.Nop())
	require.NoError(t, err)
	w.Close()
	w.Close()
}

func TestNewErrorNamesComponentAndDir(t *testing.T) {
	t.Parallel()

	file := filepath.Join(t.TempDir(), "occupied")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	_, err := New(file, isYAML, func() {}, zerolog.Nop(), WithComponent("test-watcher"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "test-watcher")
	assert.Contains(t, err.Error(), file)
}
