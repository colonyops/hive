package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSocketPathUsesTheDataDirWhenItFits(t *testing.T) {
	assert.Equal(t, "/home/u/.local/share/hive/hived.sock", SocketPath("/home/u/.local/share/hive"))
	assert.Equal(t, "/home/u/.local/share/hive/hived.lock", LockPath("/home/u/.local/share/hive"))
}

func TestSocketPathFallsBackForALongDataDir(t *testing.T) {
	long := "/" + strings.Repeat("d", 120)
	path := SocketPath(long)

	assert.Equal(t, fallbackSocketDir(), filepath.Dir(path))
	assert.LessOrEqual(t, len(path), maxSocketPath)
	assert.Equal(t, path, SocketPath(long), "every client computes the same path")
	assert.NotEqual(t, path, SocketPath(long+"x"), "each data dir gets its own socket")
}

func TestPrepareSocketDirRejectsAFallbackDirOwnedByAnotherUser(t *testing.T) {
	original := fileOwner
	t.Cleanup(func() { fileOwner = original })
	fileOwner = func(os.FileInfo) (int, bool) { return os.Getuid() + 1, true }

	err := prepareSocketDir(filepath.Join(fallbackSocketDir(), "x.sock"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "belongs to another user")
}

func TestPrepareSocketDirMakesThePrivateFallbackDir(t *testing.T) {
	require.NoError(t, prepareSocketDir(filepath.Join(fallbackSocketDir(), "x.sock")))
	info, err := os.Stat(fallbackSocketDir())
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
}
