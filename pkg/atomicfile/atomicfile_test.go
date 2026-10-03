package atomicfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteCreatesTheFileWithPerm(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")

	require.NoError(t, Write(path, []byte("a: 1\n"), 0o640))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "a: 1\n", string(got))
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o640), info.Mode().Perm())
}

func TestWriteReplacesAndLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	require.NoError(t, os.WriteFile(path, []byte("old"), 0o644))

	require.NoError(t, Write(path, []byte("new"), 0o600))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "new", string(got))
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "perm applies to a replaced file too")

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "state.json", entries[0].Name())
}

func TestWriteFailureKeepsTheOldFileAndCleansUp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target")
	require.NoError(t, os.Mkdir(path, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(path, "child"), nil, 0o644))

	require.Error(t, Write(path, []byte("data"), 0o600), "a rename over a non-empty directory fails")

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "the temp file is removed")
	assert.True(t, entries[0].IsDir())
}

func TestWriteNeedsTheParentDirectory(t *testing.T) {
	err := Write(filepath.Join(t.TempDir(), "missing", "file"), nil, 0o600)
	require.ErrorIs(t, err, os.ErrNotExist)
}
