//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
		for _, name := range names {
			assert.Contains(c, string(out), name)
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
