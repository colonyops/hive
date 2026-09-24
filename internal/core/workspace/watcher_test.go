package workspace

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWatcherDetectsRepositoryCreation(t *testing.T) {
	root := t.TempDir()
	watcher, err := NewWatcher([]string{root})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, watcher.Close()) })

	changed := make(chan error, 1)
	go func() { changed <- watcher.Wait() }()

	repo := filepath.Join(root, "new-repo")
	require.NoError(t, os.MkdirAll(filepath.Join(repo, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".git", "config"), []byte("[remote \"origin\"]\n"), 0o644))

	select {
	case err := <-changed:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("workspace watcher did not report repository creation")
	}
}

func TestWatcherDetectsGitMetadataCreationInExistingDirectory(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "existing-directory")
	require.NoError(t, os.Mkdir(repo, 0o755))

	watcher, err := NewWatcher([]string{root})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, watcher.Close()) })

	changed := make(chan error, 1)
	go func() { changed <- watcher.Wait() }()
	require.NoError(t, os.Mkdir(filepath.Join(repo, ".git"), 0o755))

	select {
	case err := <-changed:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("workspace watcher did not report .git creation")
	}
}

func TestWatcherDetectsWorkspaceRootRecreation(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	require.NoError(t, os.Mkdir(root, 0o755))

	watcher, err := NewWatcher([]string{root})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, watcher.Close()) })

	changed := make(chan error, 1)
	go func() { changed <- watcher.Wait() }()
	require.NoError(t, os.Remove(root))
	select {
	case err := <-changed:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("workspace watcher did not report root removal")
	}

	go func() { changed <- watcher.Wait() }()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "repo", ".git"), 0o755))
	select {
	case err := <-changed:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("workspace watcher did not report root recreation")
	}
}

func TestWatcherDetectsInitiallyMissingWorkspaceRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	watcher, err := NewWatcher([]string{root})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, watcher.Close()) })

	changed := make(chan error, 1)
	go func() { changed <- watcher.Wait() }()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "repo", ".git"), 0o755))

	select {
	case err := <-changed:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("workspace watcher did not report missing root creation")
	}
}

func TestWatcherDetectsWorkspaceRootWhenParentIsMissing(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing-parent", "workspace")
	watcher, err := NewWatcher([]string{root})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, watcher.Close()) })

	changed := make(chan error, 1)
	go func() { changed <- watcher.Wait() }()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "repo", ".git"), 0o755))

	select {
	case err := <-changed:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("workspace watcher did not report root creation through a missing parent")
	}
}

func TestWatcherCloseUnblocksWait(t *testing.T) {
	watcher, err := NewWatcher([]string{t.TempDir()})
	require.NoError(t, err)

	stopped := make(chan error, 1)
	go func() { stopped <- watcher.Wait() }()
	require.NoError(t, watcher.Close())

	select {
	case err := <-stopped:
		require.ErrorIs(t, err, ErrWatcherClosed)
	case <-time.After(3 * time.Second):
		t.Fatal("closing workspace watcher did not unblock Wait")
	}
}

func TestWatcherIgnoresWorkingTreeChanges(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	require.NoError(t, os.MkdirAll(filepath.Join(repo, ".git"), 0o755))

	watcher, err := NewWatcher([]string{root})
	require.NoError(t, err)

	changed := make(chan error, 1)
	go func() { changed <- watcher.Wait() }()
	require.NoError(t, os.WriteFile(filepath.Join(repo, "README.md"), []byte("test"), 0o644))

	select {
	case err := <-changed:
		require.NoError(t, err)
		t.Fatal("workspace watcher reported an unrelated working tree change")
	case <-time.After(2 * watcherDebounce):
	}

	require.NoError(t, watcher.Close())
	select {
	case err := <-changed:
		require.ErrorIs(t, err, ErrWatcherClosed)
	case <-time.After(3 * time.Second):
		t.Fatal("closing workspace watcher did not unblock Wait")
	}
}
