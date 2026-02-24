//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCtxInit(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "ctx-repo")

	// Clone the repo to get a working directory with an origin remote
	workDir := filepath.Join(t.TempDir(), "work")
	run(t, "git", "clone", repo, workDir)

	out, err := h.RunInDir(workDir, "ctx", "init")
	require.NoError(t, err, "ctx init: %s", out)
	assert.Contains(t, out, "Created symlink")

	// Verify .hive is a symlink
	linkPath := filepath.Join(workDir, ".hive")
	info, err := os.Lstat(linkPath)
	require.NoError(t, err, "stat .hive symlink")
	assert.True(t, info.Mode()&os.ModeSymlink != 0, ".hive should be a symlink")
}

func TestCtxInitIdempotent(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "ctx-idem-repo")

	workDir := filepath.Join(t.TempDir(), "work")
	run(t, "git", "clone", repo, workDir)

	// First init
	out1, err := h.RunInDir(workDir, "ctx", "init")
	require.NoError(t, err, "first ctx init: %s", out1)
	assert.Contains(t, out1, "Created symlink")

	// Second init should report already exists
	out2, err := h.RunInDir(workDir, "ctx", "init")
	require.NoError(t, err, "second ctx init: %s", out2)
	assert.Contains(t, out2, "already exists")
}

func TestCtxLs(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "ctx-ls-repo")

	workDir := filepath.Join(t.TempDir(), "work")
	run(t, "git", "clone", repo, workDir)

	// Init to create the context dir
	_, err := h.RunInDir(workDir, "ctx", "init")
	require.NoError(t, err)

	// Resolve the symlink target and create a test file
	target, err := os.Readlink(filepath.Join(workDir, ".hive"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(target, "test.txt"), []byte("hello"), 0o644))

	// ls should show the file
	out, err := h.RunInDir(workDir, "ctx", "ls")
	require.NoError(t, err, "ctx ls: %s", out)
	assert.Contains(t, out, "test.txt")
}

func TestCtxPrune(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "ctx-prune-repo")

	workDir := filepath.Join(t.TempDir(), "work")
	run(t, "git", "clone", repo, workDir)

	_, err := h.RunInDir(workDir, "ctx", "init")
	require.NoError(t, err)

	// Prune with nothing old should report no files
	out, err := h.RunInDir(workDir, "ctx", "prune", "--older-than", "1d")
	require.NoError(t, err, "ctx prune: %s", out)
	assert.Contains(t, out, "No files older than")
}
