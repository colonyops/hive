package agentws

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func waitForWorkspaceChange(t *testing.T, changed <-chan struct{}) {
	t.Helper()
	select {
	case <-changed:
	case <-time.After(5 * time.Second):
		t.Fatal("watcher did not fire")
	}
}

func assertNoWorkspaceChange(t *testing.T, changed <-chan struct{}) {
	t.Helper()
	select {
	case <-changed:
		t.Fatal("watcher fired unexpectedly")
	case <-time.After(600 * time.Millisecond):
	}
}

const minimalManifest = "version: 2\nname: X\nagent: claude\ncommand: claude\n"

func TestWatcher(t *testing.T) {
	t.Run("NewWorkspaceDirectoryIsPickedUp", func(t *testing.T) {
		t.Parallel()

		root := filepath.Join(t.TempDir(), "workspaces")
		changed := make(chan struct{}, 8)
		w, err := NewWatcher(root, func() { changed <- struct{}{} }, zerolog.Nop())
		require.NoError(t, err)
		w.Start()
		t.Cleanup(w.Close)

		wsDir := filepath.Join(root, "homeassistant")
		require.NoError(t, os.MkdirAll(wsDir, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(wsDir, manifestFileName), []byte(minimalManifest), 0o600))
		waitForWorkspaceChange(t, changed)
	})

	t.Run("EditFiresOnceAfterDebounce", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		wsDir := filepath.Join(root, "ws")
		require.NoError(t, os.MkdirAll(wsDir, 0o700))
		manifest := filepath.Join(wsDir, manifestFileName)
		require.NoError(t, os.WriteFile(manifest, []byte(minimalManifest), 0o600))

		changed := make(chan struct{}, 8)
		w, err := NewWatcher(root, func() { changed <- struct{}{} }, zerolog.Nop())
		require.NoError(t, err)
		w.Start()
		t.Cleanup(w.Close)

		require.NoError(t, os.WriteFile(manifest, []byte("version: 2\nname: Y\nagent: claude\ncommand: claude\n"), 0o600))
		waitForWorkspaceChange(t, changed)

		select {
		case <-changed:
			t.Fatal("watcher fired more than once for one edit")
		case <-time.After(400 * time.Millisecond):
		}
	})

	t.Run("AtomicRenameIntoPlaceFires", func(t *testing.T) {
		t.Parallel()

		// The atomic editor save (write tmp, rename over) is the whole reason
		// the design watches directories rather than files — precedent
		// TestActionsWatcherFiresOnWriteAndAtomicReplace.
		root := t.TempDir()
		wsDir := filepath.Join(root, "ws")
		require.NoError(t, os.MkdirAll(wsDir, 0o700))
		manifest := filepath.Join(wsDir, manifestFileName)
		require.NoError(t, os.WriteFile(manifest, []byte(minimalManifest), 0o600))

		changed := make(chan struct{}, 8)
		w, err := NewWatcher(root, func() { changed <- struct{}{} }, zerolog.Nop())
		require.NoError(t, err)
		w.Start()
		t.Cleanup(w.Close)

		tmp := manifest + ".tmp"
		require.NoError(t, os.WriteFile(tmp, []byte("version: 2\nname: Z\nagent: claude\ncommand: claude\n"), 0o600))
		require.NoError(t, os.Rename(tmp, manifest))
		waitForWorkspaceChange(t, changed)
	})

	t.Run("AgentsMDEditFiresNothing", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		wsDir := filepath.Join(root, "ws")
		require.NoError(t, os.MkdirAll(wsDir, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(wsDir, manifestFileName), []byte(minimalManifest), 0o600))

		changed := make(chan struct{}, 8)
		w, err := NewWatcher(root, func() { changed <- struct{}{} }, zerolog.Nop())
		require.NoError(t, err)
		w.Start()
		t.Cleanup(w.Close)

		require.NoError(t, os.WriteFile(filepath.Join(wsDir, "AGENTS.md"), []byte("# Hi\n"), 0o600))
		assertNoWorkspaceChange(t, changed)
	})

	t.Run("WriteUnderDocsFiresNothing", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		wsDir := filepath.Join(root, "ws")
		docsDir := filepath.Join(wsDir, "docs")
		require.NoError(t, os.MkdirAll(docsDir, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(wsDir, manifestFileName), []byte(minimalManifest), 0o600))

		changed := make(chan struct{}, 8)
		w, err := NewWatcher(root, func() { changed <- struct{}{} }, zerolog.Nop())
		require.NoError(t, err)
		w.Start()
		t.Cleanup(w.Close)

		require.NoError(t, os.WriteFile(filepath.Join(docsDir, "notes.md"), []byte("hi"), 0o600))
		assertNoWorkspaceChange(t, changed)
	})

	t.Run("RemovedWorkspaceDropsItsWatch", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		wsDir := filepath.Join(root, "ws")
		require.NoError(t, os.MkdirAll(wsDir, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(wsDir, manifestFileName), []byte(minimalManifest), 0o600))

		changed := make(chan struct{}, 8)
		w, err := NewWatcher(root, func() { changed <- struct{}{} }, zerolog.Nop())
		require.NoError(t, err)
		w.Start()
		t.Cleanup(w.Close)

		require.NoError(t, os.RemoveAll(wsDir))
		waitForWorkspaceChange(t, changed)

		// Recreating the same name proves the old watch was actually dropped
		// (and freshly re-added), not left dangling.
		require.NoError(t, os.MkdirAll(wsDir, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(wsDir, manifestFileName), []byte("version: 2\nname: Recreated\nagent: claude\ncommand: claude\n"), 0o600))
		waitForWorkspaceChange(t, changed)
	})

	t.Run("RenamedWorkspaceDirectoryDropsOldAddsNew", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		oldDir := filepath.Join(root, "old")
		require.NoError(t, os.MkdirAll(oldDir, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(oldDir, manifestFileName), []byte(minimalManifest), 0o600))

		changed := make(chan struct{}, 8)
		w, err := NewWatcher(root, func() { changed <- struct{}{} }, zerolog.Nop())
		require.NoError(t, err)
		w.Start()
		t.Cleanup(w.Close)

		newDir := filepath.Join(root, "new")
		require.NoError(t, os.Rename(oldDir, newDir))
		waitForWorkspaceChange(t, changed)

		// The strongest proof the rename's resync re-pointed the watch
		// (rather than merely dropping the old one): an edit under the new
		// name must still fire.
		require.NoError(t, os.WriteFile(filepath.Join(newDir, manifestFileName), []byte("version: 2\nname: Renamed\nagent: claude\ncommand: claude\n"), 0o600))
		waitForWorkspaceChange(t, changed)
	})

	t.Run("RootNotYetExistingStartsAndPicksItUpWhenItAppears", func(t *testing.T) {
		t.Parallel()

		// precedent: TestActionsWatcherCreatesMissingDir
		root := filepath.Join(t.TempDir(), "hive", "desktop", "workspaces")
		changed := make(chan struct{}, 8)
		w, err := NewWatcher(root, func() { changed <- struct{}{} }, zerolog.Nop())
		require.NoError(t, err)
		w.Start()
		t.Cleanup(w.Close)

		wsDir := filepath.Join(root, "ws")
		require.NoError(t, os.MkdirAll(wsDir, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(wsDir, manifestFileName), []byte(minimalManifest), 0o600))
		waitForWorkspaceChange(t, changed)
	})

	t.Run("RootDeletedWhileRunningDoesNotWedgeOrSpin", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		wsDir := filepath.Join(root, "ws")
		require.NoError(t, os.MkdirAll(wsDir, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(wsDir, manifestFileName), []byte(minimalManifest), 0o600))

		changed := make(chan struct{}, 8)
		w, err := NewWatcher(root, func() { changed <- struct{}{} }, zerolog.Nop())
		require.NoError(t, err)
		w.Start()

		require.NoError(t, os.RemoveAll(root))

		// Give the watcher a moment to observe the removal (best-effort; the
		// assertion that matters is that Close still returns promptly below,
		// proving the run loop never wedged or spun).
		time.Sleep(300 * time.Millisecond)

		done := make(chan struct{})
		go func() {
			w.Close()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("Close did not return after the root was deleted out from under the watcher")
		}
	})
}
