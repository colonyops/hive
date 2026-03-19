//go:build integration

package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceListEmpty(t *testing.T) {
	h := NewHarness(t)
	out, err := h.Run("workspace", "list")
	require.NoError(t, err, "workspace list should succeed: %s", out)
	assert.Contains(t, out, "No workspaces configured")
}

func TestWorkspaceListJSON(t *testing.T) {
	h := NewHarness(t)

	// Create a workspace dir containing a git repo
	wsDir := filepath.Join(h.DataDir(), "workspaces")
	repoDir := filepath.Join(wsDir, "test-repo")
	require.NoError(t, os.MkdirAll(repoDir, 0o755))

	// Init git repo and set remote origin
	runGit(t, repoDir, "init")
	runGit(t, repoDir, "remote", "add", "origin", "git@github.com:test/repo.git")

	h.WithConfig(fmt.Sprintf("workspaces:\n  - %s\n", wsDir))

	results, err := h.RunJSONLines("workspace", "list", "--json")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "test-repo", results[0]["name"])
	assert.Equal(t, "git@github.com:test/repo.git", results[0]["remote"])
	assert.Equal(t, repoDir, results[0]["path"])
}

func TestWorkspaceListTable(t *testing.T) {
	h := NewHarness(t)

	wsDir := filepath.Join(h.DataDir(), "workspaces")
	repoDir := filepath.Join(wsDir, "test-repo")
	require.NoError(t, os.MkdirAll(repoDir, 0o755))

	runGit(t, repoDir, "init")
	runGit(t, repoDir, "remote", "add", "origin", "git@github.com:test/repo.git")

	h.WithConfig(fmt.Sprintf("workspaces:\n  - %s\n", wsDir))

	out, err := h.RunStdout("workspace", "list")
	require.NoError(t, err)
	assert.Contains(t, out, "test-repo")
	assert.Contains(t, out, "git@github.com:test/repo.git")
}

// runGit runs a git command in the given directory.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, out)
}
