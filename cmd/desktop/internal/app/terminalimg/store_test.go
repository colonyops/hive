package terminalimg

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testPNG(t *testing.T) []byte {
	t.Helper()
	im := image.NewRGBA(image.Rect(0, 0, 3, 2))
	im.Set(1, 1, color.RGBA{R: 255, A: 255})
	var out bytes.Buffer
	require.NoError(t, png.Encode(&out, im))
	return out.Bytes()
}

func unquote(t *testing.T, text string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "/bin/sh", "-c", "set -- "+text+"; printf '%s' \"$1\"")
	cmd.Dir = t.TempDir()
	out, err := cmd.Output()
	require.NoError(t, err)
	return string(out)
}

func TestSavePreservesImagesAndSurvivesRestart(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "image storage")
	raw := testPNG(t)
	store := NewStore(dir)
	pastes, err := store.Save(t.Context(), []io.Reader{bytes.NewReader(raw), bytes.NewReader(raw)})
	require.NoError(t, err)
	require.Len(t, pastes, 2)
	assert.NotEqual(t, pastes[0], pastes[1])
	for _, paste := range pastes {
		path := unquote(t, paste)
		got, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, raw, got)
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
		info, err = os.Stat(filepath.Dir(path))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
		prepared, err := NewStore(dir).Prepare(t.Context(), []string{path})
		require.NoError(t, err)
		resolved, err := filepath.EvalSymlinks(path)
		require.NoError(t, err)
		assert.Equal(t, []string{quotePath(resolved)}, prepared)
	}
}

func TestSaveRollsBackInvalidBatch(t *testing.T) {
	store := NewStore(t.TempDir())
	pastes, err := store.Save(t.Context(), []io.Reader{bytes.NewReader(testPNG(t)), strings.NewReader("not an image")})
	require.ErrorIs(t, err, ErrInvalid)
	assert.Nil(t, pastes)
	entries, err := os.ReadDir(store.dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestPrepareEscapesPathsWithoutExecutingThem(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"space image.png", "quote'\".png", "图.png", "$(touch injected)`touch injected`(1).png", "back\\slash.png"} {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, testPNG(t), 0o600))
		pastes, err := NewStore(t.TempDir()).Prepare(t.Context(), []string{path})
		require.NoError(t, err)
		require.Len(t, pastes, 1)
		resolved, err := filepath.EvalSymlinks(path)
		require.NoError(t, err)
		assert.Equal(t, resolved, unquote(t, pastes[0]))
		assert.NotContains(t, pastes[0], "\n")
	}
}

func TestPrepareRejectsInvalidFiles(t *testing.T) {
	store := NewStore(t.TempDir())
	for _, path := range []string{"relative.png", "/missing.png", t.TempDir(), "/tmp/new\nline.png", "/tmp/escape\x1b.png"} {
		_, err := store.Prepare(t.Context(), []string{path})
		require.ErrorIs(t, err, ErrInvalid)
	}
	_, err := store.Prepare(t.Context(), nil)
	require.ErrorIs(t, err, ErrInvalid)
	_, err = store.Save(t.Context(), make([]io.Reader, MaxFiles+1))
	require.ErrorIs(t, err, ErrInvalid)
}

func TestSaveLimitsAndCancellation(t *testing.T) {
	store := NewStore(t.TempDir())
	_, err := store.Save(t.Context(), []io.Reader{bytes.NewReader(make([]byte, MaxFileBytes+1))})
	require.ErrorIs(t, err, ErrInvalid)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = store.Save(ctx, []io.Reader{bytes.NewReader(testPNG(t))})
	require.ErrorIs(t, err, context.Canceled)
	entries, err := os.ReadDir(store.dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestSaveRejectsOversizedDimensionsBeforeDecoding(t *testing.T) {
	raw := testPNG(t)
	binary.BigEndian.PutUint32(raw[16:20], 100_000)
	binary.BigEndian.PutUint32(raw[20:24], 100_000)
	binary.BigEndian.PutUint32(raw[29:33], crc32.ChecksumIEEE(raw[12:29]))
	store := NewStore(t.TempDir())
	_, err := store.Save(t.Context(), []io.Reader{bytes.NewReader(raw)})
	require.ErrorIs(t, err, ErrInvalid)
	assert.Contains(t, err.Error(), "64 megapixels")
	entries, err := os.ReadDir(store.dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

type failedImageReader struct{}

func (failedImageReader) Read([]byte) (int, error) { return 0, errors.New("source lost") }

func TestSaveCleansUpWhenReadingLaterImageFails(t *testing.T) {
	store := NewStore(t.TempDir())
	pastes, err := store.Save(t.Context(), []io.Reader{bytes.NewReader(testPNG(t)), failedImageReader{}})
	require.ErrorContains(t, err, "source lost")
	assert.Nil(t, pastes)
	entries, err := os.ReadDir(store.dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestConcurrentUploadsRespectQuotaWithoutEviction(t *testing.T) {
	raw := testPNG(t)
	store := NewStore(t.TempDir())
	store.quota = int64(len(raw))
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Go(func() {
			_, err := store.Save(t.Context(), []io.Reader{bytes.NewReader(raw)})
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	successes := 0
	for err := range errs {
		if err == nil {
			successes++
		} else {
			require.ErrorIs(t, err, ErrQuota)
		}
	}
	assert.Equal(t, 1, successes)
	used, err := store.usedBytes()
	require.NoError(t, err)
	assert.Equal(t, int64(len(raw)), used)
}
