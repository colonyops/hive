package feed

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func waitForChange(t *testing.T, changed <-chan struct{}) {
	t.Helper()
	select {
	case <-changed:
	case <-time.After(5 * time.Second):
		t.Fatal("watcher did not fire")
	}
}

func TestConfigWatcherFiresOnWriteAndAtomicReplace(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "profiles.yaml")
	changed := make(chan struct{}, 8)

	watcher, err := NewConfigWatcher(path, func() { changed <- struct{}{} }, zerolog.Nop())
	require.NoError(t, err)
	watcher.Start()
	t.Cleanup(watcher.Close)

	// Plain create+write of the watched file.
	require.NoError(t, os.WriteFile(path, []byte("profiles:\n"), 0o600))
	waitForChange(t, changed)

	// Atomic replace (write tmp, rename over) — how editors and the app
	// itself save.
	tmp := path + ".tmp"
	require.NoError(t, os.WriteFile(tmp, []byte("profiles: []\n"), 0o600))
	require.NoError(t, os.Rename(tmp, path))
	waitForChange(t, changed)
}

func TestConfigWatcherIgnoresOtherFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	changed := make(chan struct{}, 8)

	watcher, err := NewConfigWatcher(filepath.Join(dir, "profiles.yaml"), func() { changed <- struct{}{} }, zerolog.Nop())
	require.NoError(t, err)
	watcher.Start()
	t.Cleanup(watcher.Close)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "unrelated.yaml"), []byte("x: 1\n"), 0o600))
	select {
	case <-changed:
		t.Fatal("watcher fired for an unrelated file")
	case <-time.After(600 * time.Millisecond):
	}
}

func TestConfigWatcherCreatesMissingDir(t *testing.T) {
	t.Parallel()

	// The config dir may not exist on first run; the watcher must create
	// it so the watch is in place before the file is first written.
	path := filepath.Join(t.TempDir(), "hive", "desktop", "profiles.yaml")
	changed := make(chan struct{}, 8)

	watcher, err := NewConfigWatcher(path, func() { changed <- struct{}{} }, zerolog.Nop())
	require.NoError(t, err)
	watcher.Start()
	t.Cleanup(watcher.Close)

	require.NoError(t, os.WriteFile(path, []byte("profiles:\n"), 0o600))
	waitForChange(t, changed)
}
