//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorktreeExplicitFlag verifies that passing --clone-strategy worktree
// produces a worktree layout and records the strategy in the DB.
func TestWorktreeExplicitFlag(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "wt-explicit")

	out, err := h.Run("new", "--remote", repo, "--clone-strategy", "worktree", "wt-explicit")
	require.NoError(t, err, "hive new --clone-strategy worktree: %s", out)
	assert.Contains(t, out, "Session created")

	sessionPath, err := parseCreatedSessionPath(out)
	require.NoError(t, err, "parse session path from output: %s", out)

	assertWorktreeLayout(t, sessionPath)

	row := readSessionRowByName(t, h, "wt-explicit")
	assert.Equal(t, "worktree", row.CloneStrategy)
	assert.Equal(t, "active", row.State)
}

// TestFullCloneExplicitFlag verifies that --clone-strategy full produces a
// full-clone layout and is persisted correctly.
func TestFullCloneExplicitFlag(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "full-explicit")

	out, err := h.Run("new", "--remote", repo, "--clone-strategy", "full", "full-explicit")
	require.NoError(t, err, "hive new --clone-strategy full: %s", out)

	sessionPath, err := parseCreatedSessionPath(out)
	require.NoError(t, err)

	assertFullCloneLayout(t, sessionPath)

	row := readSessionRowByName(t, h, "full-explicit")
	assert.Equal(t, "full", row.CloneStrategy)
}

// TestWorktreeConfigDefault verifies that setting clone_strategy: worktree in
// config is used when no CLI flag is provided.
func TestWorktreeConfigDefault(t *testing.T) {
	h := NewHarness(t).WithConfig(`
version: "0.2.4"
git_path: git
clone_strategy: worktree
agents:
  default: testbash
  testbash:
    command: bash
rules:
  - spawn:
      - "tmux new-session -d -s {{ .Name | shq }} -c {{ .Path | shq }}"
    batch_spawn:
      - "tmux new-session -d -s {{ .Name | shq }} -c {{ .Path | shq }}"
`)
	repo := createBareRepo(t, "wt-config-default")

	out, err := h.Run("new", "--remote", repo, "wt-config-default")
	require.NoError(t, err, "hive new with worktree config default: %s", out)

	sessionPath, err := parseCreatedSessionPath(out)
	require.NoError(t, err)

	assertWorktreeLayout(t, sessionPath)

	row := readSessionRowByName(t, h, "wt-config-default")
	assert.Equal(t, "worktree", row.CloneStrategy)
}

// TestWorktreeCLIOverridesConfig verifies that a CLI --clone-strategy flag
// takes precedence over the config default.
func TestWorktreeCLIOverridesConfig(t *testing.T) {
	h := NewHarness(t).WithConfig(`
version: "0.2.4"
git_path: git
clone_strategy: worktree
agents:
  default: testbash
  testbash:
    command: bash
rules:
  - spawn:
      - "tmux new-session -d -s {{ .Name | shq }} -c {{ .Path | shq }}"
    batch_spawn:
      - "tmux new-session -d -s {{ .Name | shq }} -c {{ .Path | shq }}"
`)
	repo := createBareRepo(t, "cli-override")

	// Config says worktree, but CLI says full — CLI must win.
	out, err := h.Run("new", "--remote", repo, "--clone-strategy", "full", "cli-override")
	require.NoError(t, err, "hive new CLI overrides config: %s", out)

	sessionPath, err := parseCreatedSessionPath(out)
	require.NoError(t, err)

	assertFullCloneLayout(t, sessionPath)

	row := readSessionRowByName(t, h, "cli-override")
	assert.Equal(t, "full", row.CloneStrategy)
}

// TestWorktreeRuleOverride verifies that a rule-based clone_strategy override
// is applied when the remote matches the rule pattern.
func TestWorktreeRuleOverride(t *testing.T) {
	repo := createBareRepo(t, "rule-wt")

	h := NewHarness(t).WithConfig(`
version: "0.2.4"
git_path: git
clone_strategy: full
agents:
  default: testbash
  testbash:
    command: bash
rules:
  - clone_strategy: worktree
    spawn:
      - "tmux new-session -d -s {{ .Name | shq }} -c {{ .Path | shq }}"
    batch_spawn:
      - "tmux new-session -d -s {{ .Name | shq }} -c {{ .Path | shq }}"
`)

	out, err := h.Run("new", "--remote", repo, "rule-wt")
	require.NoError(t, err, "hive new with rule worktree override: %s", out)

	sessionPath, err := parseCreatedSessionPath(out)
	require.NoError(t, err)

	assertWorktreeLayout(t, sessionPath)

	row := readSessionRowByName(t, h, "rule-wt")
	assert.Equal(t, "worktree", row.CloneStrategy)
}

// TestWorktreeRuleNoMatchFallsBackToGlobal verifies that a non-matching rule
// does not override the global clone_strategy.
func TestWorktreeRuleNoMatchFallsBackToGlobal(t *testing.T) {
	repo := createBareRepo(t, "rule-no-match")

	h := NewHarness(t).WithConfig(`
version: "0.2.4"
git_path: git
clone_strategy: full
agents:
  default: testbash
  testbash:
    command: bash
rules:
  - pattern: "git@github.com:.*"
    clone_strategy: worktree
    spawn:
      - "tmux new-session -d -s {{ .Name | shq }} -c {{ .Path | shq }}"
    batch_spawn:
      - "tmux new-session -d -s {{ .Name | shq }} -c {{ .Path | shq }}"
  - spawn:
      - "tmux new-session -d -s {{ .Name | shq }} -c {{ .Path | shq }}"
    batch_spawn:
      - "tmux new-session -d -s {{ .Name | shq }} -c {{ .Path | shq }}"
`)

	// Local repo path won't match "git@github.com:.*", so rule is skipped.
	out, err := h.Run("new", "--remote", repo, "rule-no-match")
	require.NoError(t, err, "hive new rule no-match: %s", out)

	sessionPath, err := parseCreatedSessionPath(out)
	require.NoError(t, err)

	assertFullCloneLayout(t, sessionPath)

	row := readSessionRowByName(t, h, "rule-no-match")
	assert.Equal(t, "full", row.CloneStrategy)
}

// TestInvalidCloneStrategyRejected verifies that an invalid --clone-strategy
// value causes a non-zero exit.
func TestInvalidCloneStrategyRejected(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "invalid-strategy")

	_, err := h.Run("new", "--remote", repo, "--clone-strategy", "bogus", "invalid-strategy")
	require.Error(t, err, "hive new with invalid clone strategy should fail")
}

// TestWorktreePathConsistency verifies that the session path reported by the CLI
// matches the path persisted in the database.
func TestWorktreePathConsistency(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "path-consistency")

	out, err := h.Run("new", "--remote", repo, "--clone-strategy", "worktree", "path-consistency")
	require.NoError(t, err, "hive new: %s", out)

	cliPath, err := parseCreatedSessionPath(out)
	require.NoError(t, err)

	row := readSessionRowByName(t, h, "path-consistency")
	assert.Equal(t, cliPath, row.Path, "DB path must match CLI-reported path")
}

// TestWorktreeBareRootCreated verifies that the bare-clone root directory
// (<dataDir>/repos/.bare/<owner>/<repo>/) is created for worktree sessions.
func TestWorktreeBareRootCreated(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "bare-root")

	out, err := h.Run("new", "--remote", repo, "--clone-strategy", "worktree", "bare-root")
	require.NoError(t, err, "hive new: %s", out)

	// Derive the expected bare dir path from the .git file in the worktree.
	sessionPath, err := parseCreatedSessionPath(out)
	require.NoError(t, err)
	bareDir := worktreeBareDir(t, sessionPath)

	info, err := os.Stat(bareDir)
	require.NoError(t, err, "bare clone directory must exist at %s", bareDir)
	assert.True(t, info.IsDir(), "bare clone path must be a directory")

	// Verify it is actually a bare repo by checking for HEAD file.
	headPath := filepath.Join(bareDir, "HEAD")
	_, err = os.Stat(headPath)
	assert.NoError(t, err, "bare clone must contain HEAD file")
}

// TestWorktreeRecycleReuse verifies that a recycled worktree session is reused
// when a new session with the same remote and worktree strategy is created.
func TestWorktreeRecycleReuse(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "recycle-reuse")

	// Step 1: Create the first session.
	out, err := h.Run("new", "--remote", repo, "--clone-strategy", "worktree", "wt-recycle-alpha")
	require.NoError(t, err, "create wt-recycle-alpha: %s", out)

	pathOne, err := parseCreatedSessionPath(out)
	require.NoError(t, err)

	rowOne := readSessionRowByName(t, h, "wt-recycle-alpha")
	require.Equal(t, "active", rowOne.State)
	require.Equal(t, "worktree", rowOne.CloneStrategy)

	// Step 2: Simulate recycling by removing the worktree from git and updating DB state.
	bareDir := worktreeBareDir(t, pathOne)
	run(t, "git", "-C", bareDir, "worktree", "remove", "--force", pathOne)
	updateSessionState(t, h, rowOne.ID, "recycled")

	// Step 3: Create a second session — it should reuse the recycled path.
	out2, err := h.Run("new", "--remote", repo, "--clone-strategy", "worktree", "wt-recycle-beta")
	require.NoError(t, err, "create wt-recycle-beta (reuse): %s", out2)

	pathTwo, err := parseCreatedSessionPath(out2)
	require.NoError(t, err)

	assert.Equal(t, pathOne, pathTwo, "reused session must have same path as recycled session")

	rowTwo := readSessionRowByName(t, h, "wt-recycle-beta")
	assert.Equal(t, "active", rowTwo.State)
	assert.Equal(t, "worktree", rowTwo.CloneStrategy)
	assert.Equal(t, rowOne.ID, rowTwo.ID, "reused session must keep the same DB ID")
}

// worktreeBareDir resolves the bare clone directory from a worktree session path
// by reading the .git file and following the gitdir pointer.
func worktreeBareDir(t *testing.T, sessionPath string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(sessionPath, ".git"))
	require.NoError(t, err, "reading .git file")

	// .git file format: "gitdir: /path/to/.bare/<owner>/<repo>/worktrees/<name>\n"
	line := strings.TrimSpace(string(data))
	prefix := "gitdir: "
	require.True(t, strings.HasPrefix(line, prefix), ".git file should start with 'gitdir: ', got: %q", line)
	gitdirPath := strings.TrimPrefix(line, prefix)

	// Walk up from worktrees/<name> to the bare root:
	// .bare/<owner>/<repo>/worktrees/<name> → .bare/<owner>/<repo>/
	return filepath.Clean(filepath.Join(gitdirPath, "..", ".."))
}
