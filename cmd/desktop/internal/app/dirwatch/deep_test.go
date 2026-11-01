package dirwatch

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllSubdirectoriesSeesAnEditAtAnyDepthAndSkipsWhatItIsTold(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "radio", "pages", "js")
	skipped := filepath.Join(root, "radio", "node_modules", "dep")
	require.NoError(t, os.MkdirAll(deep, 0o755))
	require.NoError(t, os.MkdirAll(skipped, 0o755))

	changes := make(chan struct{}, 8)
	watcher, err := New(root, func(path string) bool { return filepath.Ext(path) == ".js" }, func() { changes <- struct{}{} }, zerolog.Nop(),
		WithDebounce(10*time.Millisecond),
		WithAllSubdirectories(func(dir string) bool { return dir == "node_modules" }),
	)
	require.NoError(t, err)
	watcher.Start()
	defer watcher.Close()

	require.NoError(t, os.WriteFile(filepath.Join(skipped, "a.js"), []byte("x"), 0o600))
	select {
	case <-changes:
		t.Fatal("a skipped folder reported a change")
	case <-time.After(200 * time.Millisecond):
	}

	require.NoError(t, os.WriteFile(filepath.Join(deep, "app.js"), []byte("x"), 0o600))
	select {
	case <-changes:
	case <-time.After(3 * time.Second):
		t.Fatal("an edit two folders down reported nothing")
	}

	made := filepath.Join(deep, "made-later")
	require.NoError(t, os.Mkdir(made, 0o755))
	assert.Eventually(t, func() bool { return len(changes) > 0 }, 3*time.Second, 10*time.Millisecond)
	for len(changes) > 0 {
		<-changes
	}
	require.NoError(t, os.WriteFile(filepath.Join(made, "late.js"), []byte("x"), 0o600))
	select {
	case <-changes:
	case <-time.After(3 * time.Second):
		t.Fatal("an edit in a folder made after the start reported nothing")
	}
}
