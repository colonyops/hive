package repocontext

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/config"
)

func TestPruneRemovesOldEntriesAndKeepsCanvases(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().Add(-48 * time.Hour)
	for _, name := range []string{"plans", "notes.md", CanvasesDirName} {
		path := filepath.Join(dir, name)
		if filepath.Ext(name) == "" {
			require.NoError(t, os.MkdirAll(path, 0o755))
		} else {
			require.NoError(t, os.WriteFile(path, []byte("x"), 0o644))
		}
		require.NoError(t, os.Chtimes(path, old, old))
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "fresh.md"), []byte("x"), 0o644))

	svc := NewService(zerolog.Nop(), &config.Config{}, nil)
	removed, err := svc.Prune(dir, 24*time.Hour)
	require.NoError(t, err)

	assert.Equal(t, 2, removed)
	assert.NoDirExists(t, filepath.Join(dir, "plans"))
	assert.NoFileExists(t, filepath.Join(dir, "notes.md"))
	assert.FileExists(t, filepath.Join(dir, "fresh.md"))
	assert.DirExists(t, filepath.Join(dir, CanvasesDirName), "canvases are durable output, not stale context")
}
