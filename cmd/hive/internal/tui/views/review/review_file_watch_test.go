package review

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileWatcherReportsAChangeToTheFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.md")
	writeFile(t, path)
	doc := Document{Path: path, RelPath: "notes.md", Type: TypeOther, Content: "# doc\n"}

	w, err := WatchFile(zerolog.Nop(), doc)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, w.Close()) })

	msgs := make(chan any, 1)
	go func() { msgs <- w.Next()() }()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "other.md"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(path, []byte("# changed\n"), 0o644))

	select {
	case msg := <-msgs:
		changed, ok := msg.(FileChangedMsg)
		require.True(t, ok, "got %T", msg)
		assert.Equal(t, path, changed.Document.Path)
		assert.Empty(t, changed.Document.Content, "the view reads the new content")
	case <-time.After(5 * time.Second):
		t.Fatal("no change reported")
	}
}

func TestFileWatcherNextEndsOnClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.md")
	writeFile(t, path)
	w, err := WatchFile(zerolog.Nop(), Document{Path: path})
	require.NoError(t, err)

	msgs := make(chan any, 1)
	go func() { msgs <- w.Next()() }()
	require.NoError(t, w.Close())

	select {
	case msg := <-msgs:
		assert.Nil(t, msg)
	case <-time.After(5 * time.Second):
		t.Fatal("Next did not return after Close")
	}
}

func TestNilFileWatcherWaitsForNothing(t *testing.T) {
	var w *FileWatcher
	assert.Nil(t, w.Next())
	assert.NoError(t, w.Close())
}
