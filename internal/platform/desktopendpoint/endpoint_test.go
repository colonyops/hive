package desktopendpoint

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteReadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := Endpoint{URL: "http://127.0.0.1:4321", PID: 99, StartedAt: time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)}

	require.NoError(t, Write(dir, want))

	got, err := Read(dir)
	require.NoError(t, err)
	assert.Equal(t, want.URL, got.URL)
	assert.Equal(t, want.PID, got.PID)
	assert.True(t, want.StartedAt.Equal(got.StartedAt))
}

func TestWriteIsPrivateAndLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Write(dir, Endpoint{URL: "http://127.0.0.1:1"}))
	require.NoError(t, Write(dir, Endpoint{URL: "http://127.0.0.1:2"}))

	info, err := os.Stat(Path(dir))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	entries, err := os.ReadDir(filepath.Dir(Path(dir)))
	require.NoError(t, err)
	require.Len(t, entries, 1)

	got, err := Read(dir)
	require.NoError(t, err)
	assert.Equal(t, "http://127.0.0.1:2", got.URL)
}

func TestReadMissingIsNotRunning(t *testing.T) {
	_, err := Read(t.TempDir())
	require.ErrorIs(t, err, ErrNotRunning)
}

func TestReadRejectsBadContent(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Dir(Path(dir)), 0o700))

	require.NoError(t, os.WriteFile(Path(dir), []byte("{not json"), 0o600))
	_, err := Read(dir)
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrNotRunning)

	require.NoError(t, os.WriteFile(Path(dir), []byte(`{"pid":1}`), 0o600))
	_, err = Read(dir)
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrNotRunning)
}

func TestRemove(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Remove(dir), "missing file")

	require.NoError(t, Write(dir, Endpoint{URL: "http://127.0.0.1:1"}))
	require.NoError(t, Remove(dir))
	_, err := Read(dir)
	require.ErrorIs(t, err, ErrNotRunning)
}
