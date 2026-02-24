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

// pollFor retries fn until it returns nil or the timeout is reached.
func pollFor(t *testing.T, timeout time.Duration, interval time.Duration, fn func() error) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		lastErr = fn()
		if lastErr == nil {
			return
		}
		time.Sleep(interval)
	}
	t.Fatalf("pollFor timed out after %v: %v", timeout, lastErr)
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
