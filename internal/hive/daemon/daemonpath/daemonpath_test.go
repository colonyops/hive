package daemonpath

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSocketPathUsesTheDataDirWhenItFits(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "dp")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(real, "hived.sock"), SocketPath(dir))
	assert.Equal(t, filepath.Join(dir, "hived.lock"), LockPath(dir))
}

func TestSocketPathFallsBackForALongDataDir(t *testing.T) {
	long := "/" + strings.Repeat("d", 120)
	path := SocketPath(long)

	assert.Equal(t, fallbackSocketDir(), filepath.Dir(path))
	assert.LessOrEqual(t, len(path), maxSocketPath)
	assert.Equal(t, path, SocketPath(long), "every client computes the same path")
	assert.NotEqual(t, path, SocketPath(long+"x"), "each data dir gets its own socket")
}

func otherOwner(t *testing.T) {
	t.Helper()
	original := fileOwner
	t.Cleanup(func() { fileOwner = original })
	fileOwner = func(os.FileInfo) (int, bool) { return os.Getuid() + 1, true }
}

func TestPrepareSocketDirRejectsAFallbackDirOwnedByAnotherUser(t *testing.T) {
	otherOwner(t)

	err := PrepareSocketDir(filepath.Join(fallbackSocketDir(), "x.sock"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "belongs to another user")
}

func TestPrepareSocketDirMakesThePrivateFallbackDir(t *testing.T) {
	require.NoError(t, PrepareSocketDir(filepath.Join(fallbackSocketDir(), "x.sock")))
	info, err := os.Stat(fallbackSocketDir())
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
}

func TestCheckSocketDirRejectsAFallbackDirOwnedByAnotherUser(t *testing.T) {
	require.NoError(t, PrepareSocketDir(filepath.Join(fallbackSocketDir(), "x.sock")))
	otherOwner(t)

	err := CheckSocketDir(filepath.Join(fallbackSocketDir(), "x.sock"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "belongs to another user")
}

func TestCheckSocketDirAcceptsOurFallbackDirAndAnyDataDir(t *testing.T) {
	require.NoError(t, PrepareSocketDir(filepath.Join(fallbackSocketDir(), "x.sock")))
	assert.NoError(t, CheckSocketDir(filepath.Join(fallbackSocketDir(), "x.sock")))

	otherOwner(t)
	assert.NoError(t, CheckSocketDir(filepath.Join(t.TempDir(), "hived.sock")), "only the shared fallback dir is checked")
}

func TestSocketPathIsTheSameThroughASymlink(t *testing.T) {
	root := t.TempDir()
	long := filepath.Join(root, strings.Repeat("d", 110))
	require.NoError(t, os.MkdirAll(long, 0o755))
	link := filepath.Join(root, "link")
	require.NoError(t, os.Symlink(long, link))

	assert.Equal(t, SocketPath(long), SocketPath(link), "a long data dir and a symlink to it")
	assert.Equal(t, SocketPath(filepath.Join(long, "new")), SocketPath(filepath.Join(link, "new")), "a data dir that does not exist yet")

	short := filepath.Join(root, "s")
	require.NoError(t, os.Symlink(link, short))
	assert.Equal(t, SocketPath(long), SocketPath(short), "a chain of symlinks")
}
