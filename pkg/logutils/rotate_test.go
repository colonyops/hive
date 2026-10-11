package logutils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readLog(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

// openChecking checks on every write instead of every few minutes.
func openChecking(t *testing.T, path string, maxSize int64, backups int) *rotatingFile {
	t.Helper()
	r, err := openRotating(path, maxSize, backups)
	require.NoError(t, err)
	r.checkInterval = 0
	return r
}

func TestNewRootRotatesAnOversizedFileOnOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hive.log")
	require.NoError(t, os.WriteFile(path, []byte(strings.Repeat("old\n", 300)), 0o644))

	logger, closeLog, err := NewRoot(Options{Level: zerolog.InfoLevel, File: path, MaxFileSize: 1 << 10})
	require.NoError(t, err)
	logger.Info().Msg("fresh")
	closeLog()

	assert.Equal(t, strings.Repeat("old\n", 300), readLog(t, path+".1"))
	assert.Contains(t, readLog(t, path), "fresh")
	assert.NotContains(t, readLog(t, path), "old")
}

func TestRotatingFileRotatesWhileWritingAndKeepsBackups(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hive.log")
	r := openChecking(t, path, 1<<10, 2)

	line := strings.Repeat("x", 99) + "\n"
	for range 100 {
		_, err := r.Write([]byte(line))
		require.NoError(t, err)
	}
	require.NoError(t, r.Close())

	for _, p := range []string{path, path + ".1", path + ".2"} {
		info, err := os.Stat(p)
		require.NoError(t, err, p)
		assert.LessOrEqual(t, info.Size(), int64(1<<10+len(line)), p)
	}
	assert.NoFileExists(t, path+".3")
}

func TestRotatingFileNegativeBackupsTruncates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hive.log")
	require.NoError(t, os.WriteFile(path, []byte(strings.Repeat("old\n", 300)), 0o644))

	r, err := openRotating(path, 1<<10, -1)
	require.NoError(t, err)
	require.NoError(t, r.Close())

	assert.Empty(t, readLog(t, path))
	assert.NoFileExists(t, path+".1")
}

func TestRotatingFileFollowsARotationByAnotherWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hive.log")
	desktop := openChecking(t, path, 1<<10, 3)
	cli := openChecking(t, path, 1<<10, 3)

	_, err := cli.Write([]byte(strings.Repeat("c", 2<<10) + "\n"))
	require.NoError(t, err)
	_, err = desktop.Write([]byte(strings.Repeat("d", 200) + "\n"))
	require.NoError(t, err)
	_, err = desktop.Write([]byte("after rotation\n"))
	require.NoError(t, err)
	require.NoError(t, desktop.Close())
	require.NoError(t, cli.Close())

	assert.Contains(t, readLog(t, path), "after rotation")
	assert.NotContains(t, readLog(t, path+".1"), "after rotation")
}

func TestRotatingFileLosesNoLinesAcrossConcurrentWriters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hive.log")
	const writers, lines = 4, 500

	var wg sync.WaitGroup
	for w := range writers {
		r := openChecking(t, path, 4<<10, 1000)
		wg.Go(func() {
			defer func() { _ = r.Close() }()
			for i := range lines {
				_, _ = fmt.Fprintf(r, "writer %d line %d\n", w, i)
			}
		})
	}
	wg.Wait()

	matches, err := filepath.Glob(path + "*")
	require.NoError(t, err)
	total := 0
	for _, m := range matches {
		if strings.HasSuffix(m, ".lock") {
			continue
		}
		total += strings.Count(readLog(t, m), "\n")
	}
	assert.Equal(t, writers*lines, total)
}

func TestRotatingFileWaitsForTheIntervalBeforeChecking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hive.log")
	r, err := openRotating(path, 1<<10, 3)
	require.NoError(t, err)

	_, err = r.Write([]byte(strings.Repeat("x", 4<<10) + "\n"))
	require.NoError(t, err)
	assert.NoFileExists(t, path+".1")

	r.lastChecked = r.lastChecked.Add(-rotateCheckInterval)
	_, err = r.Write([]byte("next\n"))
	require.NoError(t, err)
	require.NoError(t, r.Close())

	assert.FileExists(t, path+".1")
}
