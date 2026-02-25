//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var createdSessionPathPattern = regexp.MustCompile(`(?ms)Session created\s+(/\S+)`)

// parseCreatedSessionPath extracts the session path from `hive new` combined output.
func parseCreatedSessionPath(t *testing.T, out string) string {
	t.Helper()
	m := createdSessionPathPattern.FindStringSubmatch(out)
	require.GreaterOrEqual(t, len(m), 2, "session path not found in output:\n%s", out)
	return m[1]
}

// assertWorktreeLayout verifies sessionPath has a .git file (gitdir) not a .git directory.
func assertWorktreeLayout(t *testing.T, sessionPath string) {
	t.Helper()
	gitPath := filepath.Join(sessionPath, ".git")
	info, err := os.Stat(gitPath)
	require.NoError(t, err, ".git must exist at %s", gitPath)
	require.False(t, info.IsDir(), ".git must be a file (gitdir), not a directory, at %s", gitPath)

	content, err := os.ReadFile(gitPath)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(string(content), "gitdir:"), ".git file must start with 'gitdir:'")
}

// assertFullCloneLayout verifies sessionPath has a .git directory (full clone).
func assertFullCloneLayout(t *testing.T, sessionPath string) {
	t.Helper()
	gitPath := filepath.Join(sessionPath, ".git")
	info, err := os.Stat(gitPath)
	require.NoError(t, err, ".git must exist at %s", gitPath)
	require.True(t, info.IsDir(), ".git must be a directory for full clone at %s", gitPath)
}

// worktreeBareDir reads the .git file in sessionPath and returns the bare root.
// Format: "gitdir: /path/to/.bare/.../worktrees/<branch>"
func worktreeBareDir(t *testing.T, sessionPath string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(sessionPath, ".git"))
	require.NoError(t, err)
	line := strings.TrimSpace(string(content))
	_, gitdir, ok := strings.Cut(line, "gitdir: ")
	require.True(t, ok, "expected 'gitdir: ' prefix in .git file, got: %s", line)
	// Walk up two levels: worktrees/<branch> -> bare root
	return filepath.Dir(filepath.Dir(gitdir))
}

// createBareRepo creates a local bare git repository with a seeded initial commit.
// Returns the path to the bare repo.
func createBareRepo(t *testing.T, name string) string {
	t.Helper()

	dir := t.TempDir()
	bareDir := filepath.Join(dir, name+".git")

	// Create bare repo
	run(t, "git", "init", "--bare", bareDir)

	// Create a temp working copy to seed the initial commit
	workDir := filepath.Join(dir, "work")
	run(t, "git", "clone", bareDir, workDir)

	readme := filepath.Join(workDir, "README.md")
	if err := os.WriteFile(readme, []byte("# "+name+"\n"), 0o644); err != nil {
		t.Fatalf("writing readme: %v", err)
	}

	runInDir(t, workDir, "git", "add", ".")
	runInDir(t, workDir, "git", "-c", "user.name=Test", "-c", "user.email=test@test.com", "commit", "-m", "initial")
	runInDir(t, workDir, "git", "push", "origin", "HEAD")

	return bareDir
}

// parseJSON parses a JSON string into a map.
func parseJSON(data string) (map[string]any, error) {
	var result map[string]any
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}
	return result, nil
}

// parseJSONLines parses newline-delimited JSON into a slice of maps.
func parseJSONLines(data string) ([]map[string]any, error) {
	var results []map[string]any
	decoder := json.NewDecoder(strings.NewReader(data))
	for decoder.More() {
		var obj map[string]any
		if err := decoder.Decode(&obj); err != nil {
			return nil, fmt.Errorf("decoding JSON line: %w", err)
		}
		results = append(results, obj)
	}
	return results, nil
}

// assertTmuxSessionExists waits for all named tmux sessions to appear.
func assertTmuxSessionExists(t *testing.T, names ...string) {
	t.Helper()
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").CombinedOutput()
		assert.NoError(c, err, "tmux list-sessions: %s", out)
		sessions := strings.Split(strings.TrimSpace(string(out)), "\n")
		for _, name := range names {
			assert.Contains(c, sessions, name)
		}
	}, 5*time.Second, 200*time.Millisecond)
}

// assertTmuxHasWindows waits for the named tmux session to have at least one window.
func assertTmuxHasWindows(t *testing.T, session string) {
	t.Helper()
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		out, err := exec.Command("tmux", "list-windows", "-t", session, "-F", "#{window_name}").CombinedOutput()
		assert.NoError(c, err, "tmux list-windows: %s", out)
		assert.NotEmpty(c, strings.TrimSpace(string(out)), "no windows found")
	}, 5*time.Second, 200*time.Millisecond)
}

// assertTmuxWindowNames waits for the named tmux session to have exactly the expected window names in order.
func assertTmuxWindowNames(t *testing.T, session string, wantNames []string) {
	t.Helper()
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		out, err := exec.Command("tmux", "list-windows", "-t", session, "-F", "#{window_name}").CombinedOutput()
		assert.NoError(c, err, "tmux list-windows: %s", out)
		got := strings.Split(strings.TrimSpace(string(out)), "\n")
		assert.Equal(c, wantNames, got)
	}, 5*time.Second, 200*time.Millisecond)
}

// cleanupTmuxSession registers a t.Cleanup to kill a named tmux session.
func cleanupTmuxSession(t *testing.T, name string) {
	t.Helper()
	t.Cleanup(func() {
		_ = exec.Command("tmux", "kill-session", "-t", name).Run()
	})
}

func run(t *testing.T, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command %s %v failed: %v\noutput: %s", name, args, err, out)
	}
	return string(out)
}

func runInDir(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command %s %v in %s failed: %v\noutput: %s", name, args, dir, err, out)
	}
	return string(out)
}
