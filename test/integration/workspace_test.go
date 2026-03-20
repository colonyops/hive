//go:build integration

package integration

import (
	"fmt"
	"os"
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

	wsDir, repoDir := createWorkspaceRepo(t, "test-repo", "git@github.com:test/repo.git")
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

	wsDir, _ := createWorkspaceRepo(t, "test-repo", "git@github.com:test/repo.git")
	h.WithConfig(fmt.Sprintf("workspaces:\n  - %s\n", wsDir))

	out, err := h.RunStdout("workspace", "list")
	require.NoError(t, err)
	assert.Contains(t, out, "test-repo")
	assert.Contains(t, out, "git@github.com:test/repo.git")
}

// createWorkspaceRepo creates a workspace directory containing a git repo with the given name and remote.
// Returns (workspaceDir, repoDir).
func createWorkspaceRepo(t *testing.T, name, remote string) (string, string) {
	t.Helper()
	wsDir := filepath.Join(t.TempDir(), "workspaces")
	repoDir := filepath.Join(wsDir, name)
	require.NoError(t, os.MkdirAll(repoDir, 0o755))
	runInDir(t, repoDir, "git", "init")
	runInDir(t, repoDir, "git", "remote", "add", "origin", remote)
	return wsDir, repoDir
}
