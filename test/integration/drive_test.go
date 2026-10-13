//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// driveWorkDir clones a repository, runs "drive init" in it, and writes one
// markdown plan and one text file into its space.
func driveWorkDir(t *testing.T, h *Harness, name string) string {
	t.Helper()
	repo := createBareRepo(t, name)
	workDir := filepath.Join(t.TempDir(), "work")
	run(t, "git", "clone", repo, workDir)

	out, err := h.RunInDir(workDir, "drive", "init")
	require.NoError(t, err, "drive init: %s", out)

	target, err := os.Readlink(filepath.Join(workDir, ".hive"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(target, "plans", "ship.md"), []byte("# Ship\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(target, "notes.txt"), []byte("hello"), 0o644))
	return workDir
}

func TestDriveLs(t *testing.T) {
	h := NewHarness(t)
	workDir := driveWorkDir(t, h, "drive-ls-repo")

	entries, err := h.RunJSONLinesInDir(workDir, "drive", "ls", "--json")
	require.NoError(t, err)
	kinds := map[string]string{}
	for _, entry := range entries {
		path, ok := entry["path"].(string)
		require.True(t, ok, "entry without a path: %v", entry)
		kind, ok := entry["kind"].(string)
		require.True(t, ok, "entry without a kind: %v", entry)
		kinds[path] = kind
		assert.NotContains(t, entry, "children", "the lines are the tree, so no entry nests")
	}
	assert.Equal(t, "markdown", kinds["plans/ship.md"])
	assert.Equal(t, "file", kinds["notes.txt"])
	assert.Equal(t, "dir", kinds["plans"])
}

func TestDriveLsAll(t *testing.T) {
	h := NewHarness(t)
	workDir := driveWorkDir(t, h, "drive-ls-all-repo")
	target, err := os.Readlink(filepath.Join(workDir, ".hive"))
	require.NoError(t, err)
	// The JSON lines are entries, so the shared space shows only once it holds one.
	shared := filepath.Join(filepath.Dir(filepath.Dir(target)), "shared")
	require.NoError(t, os.MkdirAll(shared, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(shared, "team.md"), []byte("# Team\n"), 0o644))

	lines, err := h.RunJSONLinesInDir(workDir, "drive", "ls", "--all", "--json")
	require.NoError(t, err)
	require.NotEmpty(t, lines)
	for _, line := range lines {
		assert.Contains(t, line, "space")
		assert.Contains(t, line, "entry")
	}

	keys := map[string]bool{}
	for _, line := range lines {
		space, _ := line["space"].(map[string]any)
		key, _ := space["key"].(string)
		keys[key] = true
	}
	assert.True(t, keys["shared"], "--all lists the shared space: %v", keys)
}

func TestDriveSpaces(t *testing.T) {
	h := NewHarness(t)
	workDir := driveWorkDir(t, h, "drive-spaces-repo")

	spaces, err := h.RunJSONLinesInDir(workDir, "drive", "spaces", "--json")
	require.NoError(t, err)
	kinds := map[string]int{}
	for _, space := range spaces {
		kind, ok := space["kind"].(string)
		require.True(t, ok, "space without a kind: %v", space)
		kinds[kind]++
		assert.NotEmpty(t, space["key"])
	}
	assert.Equal(t, 1, kinds["repository"])
	assert.Equal(t, 1, kinds["shared"])
}

func TestCtxAliasStillWorks(t *testing.T) {
	h := NewHarness(t)
	workDir := driveWorkDir(t, h, "drive-alias-repo")

	viaDrive, err := h.RunStdoutInDir(workDir, "drive", "ls")
	require.NoError(t, err)
	viaCtx, err := h.RunStdoutInDir(workDir, "ctx", "ls")
	require.NoError(t, err)
	assert.Equal(t, viaDrive, viaCtx)
}
