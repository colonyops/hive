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

// worktreeConfig returns inline YAML config that sets a global clone strategy.
func worktreeConfig(cloneStrategy string) string {
	return fmt.Sprintf(`data_dir: ""
spawn: []
clone_strategy: %s
`, cloneStrategy)
}

func TestWorktreeExplicitFlag(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "wt-explicit-repo")

	out, err := h.Run("new", "--clone-strategy", "worktree", "--remote", repo, "wt-explicit")
	require.NoError(t, err, "hive new --clone-strategy worktree: %s", out)
	assert.Contains(t, out, "Session created")

	path := parseCreatedSessionPath(t, out)
	assertWorktreeLayout(t, path)

	bareDir := worktreeBareDir(t, path)
	_, err = os.Stat(filepath.Join(bareDir, "HEAD"))
	require.NoError(t, err, "bare root must contain HEAD at %s", bareDir)
}

func TestFullCloneExplicitFlag(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "full-explicit-repo")

	out, err := h.Run("new", "--clone-strategy", "full", "--remote", repo, "full-explicit")
	require.NoError(t, err, "hive new --clone-strategy full: %s", out)
	assert.Contains(t, out, "Session created")

	path := parseCreatedSessionPath(t, out)
	assertFullCloneLayout(t, path)
}

func TestWorktreeConfigDefault(t *testing.T) {
	h := NewHarness(t).WithConfig(worktreeConfig("worktree"))
	repo := createBareRepo(t, "wt-config-repo")

	out, err := h.Run("new", "--remote", repo, "wt-config")
	require.NoError(t, err, "hive new with worktree config: %s", out)

	path := parseCreatedSessionPath(t, out)
	assertWorktreeLayout(t, path)
}

func TestWorktreeCLIOverridesConfig(t *testing.T) {
	h := NewHarness(t).WithConfig(worktreeConfig("worktree"))
	repo := createBareRepo(t, "wt-override-repo")

	// Config says worktree but CLI flag says full — full should win
	out, err := h.Run("new", "--clone-strategy", "full", "--remote", repo, "override-to-full")
	require.NoError(t, err, "hive new --clone-strategy full overriding config: %s", out)

	path := parseCreatedSessionPath(t, out)
	assertFullCloneLayout(t, path)
}

func TestWorktreeRuleOverride(t *testing.T) {
	repo := createBareRepo(t, "wt-rule-repo")

	// Global default is full, but a rule matching everything sets worktree
	cfg := fmt.Sprintf(`data_dir: ""
spawn: []
clone_strategy: full
rules:
  - clone_strategy: worktree
`)
	h := NewHarness(t).WithConfig(cfg)

	out, err := h.Run("new", "--remote", repo, "rule-wt")
	require.NoError(t, err, "hive new with rule override: %s", out)

	path := parseCreatedSessionPath(t, out)
	assertWorktreeLayout(t, path)
}

func TestWorktreeRuleNoMatchFallsBackToGlobal(t *testing.T) {
	repo := createBareRepo(t, "wt-nomatch-repo")

	// Rule only matches GitHub remotes — local bare repo won't match
	cfg := `data_dir: ""
spawn: []
clone_strategy: full
rules:
  - pattern: "git@github.com:.*"
    clone_strategy: worktree
`
	h := NewHarness(t).WithConfig(cfg)

	out, err := h.Run("new", "--remote", repo, "no-match")
	require.NoError(t, err, "hive new with non-matching rule: %s", out)

	path := parseCreatedSessionPath(t, out)
	assertFullCloneLayout(t, path)
}

func TestInvalidCloneStrategyRejected(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "invalid-strat-repo")

	_, err := h.Run("new", "--clone-strategy", "bogus", "--remote", repo, "bad-strat")
	require.Error(t, err, "hive new --clone-strategy bogus should fail")
}

func TestWorktreePathConsistency(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "wt-path-repo")

	out, err := h.Run("new", "--clone-strategy", "worktree", "--remote", repo, "path-check")
	require.NoError(t, err, "hive new: %s", out)

	pathFromOutput := parseCreatedSessionPath(t, out)

	// Verify path from output matches what's on disk
	_, err = os.Stat(pathFromOutput)
	require.NoError(t, err, "session path from output must exist on disk: %s", pathFromOutput)
	assertWorktreeLayout(t, pathFromOutput)
}

func TestWorktreeBareRootCreated(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "bare-root-repo")

	out, err := h.Run("new", "--clone-strategy", "worktree", "--remote", repo, "bare-root-check")
	require.NoError(t, err, "hive new: %s", out)

	path := parseCreatedSessionPath(t, out)
	assertWorktreeLayout(t, path)

	bareDir := worktreeBareDir(t, path)
	require.NoError(t, err, "reading gitdir")

	headPath := filepath.Join(bareDir, "HEAD")
	_, err = os.Stat(headPath)
	require.NoError(t, err, "bare root must contain HEAD at %s", headPath)
}

func TestWorktreeTwoSessionsSameBare(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "shared-bare-repo")

	out1, err := h.Run("new", "--clone-strategy", "worktree", "--remote", repo, "wt-one")
	require.NoError(t, err, "first session: %s", out1)

	out2, err := h.Run("new", "--clone-strategy", "worktree", "--remote", repo, "wt-two")
	require.NoError(t, err, "second session: %s", out2)

	path1 := parseCreatedSessionPath(t, out1)
	path2 := parseCreatedSessionPath(t, out2)

	// Both are worktrees
	assertWorktreeLayout(t, path1)
	assertWorktreeLayout(t, path2)

	// They should share the same bare root
	bare1 := worktreeBareDir(t, path1)
	bare2 := worktreeBareDir(t, path2)
	assert.Equal(t, bare1, bare2, "both worktree sessions should use the same bare clone")
}
