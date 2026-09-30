package agentws

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeWorkspace(t *testing.T, root, dir, name string) string {
	t.Helper()
	wsDir := filepath.Join(root, dir)
	require.NoError(t, os.MkdirAll(wsDir, 0o700))
	path := filepath.Join(wsDir, manifestFileName)
	content := fmt.Sprintf("version: 2\nname: %s\nagent: claude\ncommand: claude\n", name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestStoreKeepsLastGoodPerWorkspace(t *testing.T) {
	t.Parallel()

	t.Run("PerWorkspaceIsolation", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeWorkspace(t, root, "a", "Alpha")
		pathB := writeWorkspace(t, root, "b", "Bravo")

		s := NewStore(root)
		require.NoError(t, s.Reload())

		a, ok := s.Status("a")
		require.True(t, ok)
		assert.Equal(t, "Alpha", a.Workspace.Name)
		b, ok := s.Status("b")
		require.True(t, ok)
		assert.Equal(t, "Bravo", b.Workspace.Name)

		// Break B, edit A, then reload.
		require.NoError(t, os.WriteFile(pathB, []byte("version: 1\nfoo: bar\n"), 0o600))
		writeWorkspace(t, root, "a", "Alpha Prime")
		require.NoError(t, s.Reload())

		aAfter, ok := s.Status("a")
		require.True(t, ok)
		assert.Equal(t, "Alpha Prime", aAfter.Workspace.Name, "A must reload to its new bytes")

		bStatus, ok := s.Status("b")
		require.True(t, ok)
		assert.False(t, bStatus.Valid)
		require.Error(t, bStatus.Err)
		assert.Equal(t, "Bravo", bStatus.Workspace.Name, "B must keep its last-good content")

		assert.Len(t, s.Statuses(), 2, "the list must not blank when one workspace breaks")
	})

	t.Run("BrokenLibraryLeavesWorkspacesLoaded", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeWorkspace(t, root, "a", "Alpha")
		libPath := filepath.Join(root, libraryFileName)
		require.NoError(t, os.WriteFile(libPath, []byte("version: 1\nservers: {}\n"), 0o600))

		s := NewStore(root)
		require.NoError(t, s.Reload())
		require.True(t, s.Library().Valid)

		require.NoError(t, os.WriteFile(libPath, []byte("version: 1\nfoo: bar\n"), 0o600))
		require.NoError(t, s.Reload())

		lib := s.Library()
		assert.False(t, lib.Valid)
		require.Error(t, lib.Err)

		_, ok := s.Status("a")
		assert.True(t, ok, "workspaces must stay loaded when only the library breaks")
	})
}

func TestStoreIgnoresDirectoriesWithoutAManifest(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "empty"), 0o700))
	writeWorkspace(t, root, "a", "Alpha")

	s := NewStore(root)
	require.NoError(t, s.Reload())

	_, ok := s.Status("empty")
	assert.False(t, ok)
	assert.Len(t, s.Statuses(), 1)
}

func TestStoreReportsMalformedEntries(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "stray.txt"), []byte("hello"), 0o600))

	weirdDir := filepath.Join(root, "weird")
	require.NoError(t, os.MkdirAll(filepath.Join(weirdDir, manifestFileName), 0o700))

	s := NewStore(root)
	require.NoError(t, s.Reload())

	_, ok := s.Status("stray.txt")
	assert.False(t, ok, "a regular file at root must not be read as a workspace")

	weird, ok := s.Status("weird")
	require.True(t, ok, "a directory-shaped manifest must still surface as a workspace entry")
	assert.False(t, weird.Valid)
	require.Error(t, weird.Err)
}

func TestStoreReloadOnMissingRootIsEmptyNotError(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "does-not-exist")
	s := NewStore(root)
	require.NoError(t, s.Reload())
	assert.Empty(t, s.Statuses())
	assert.True(t, s.Library().Valid)
}
