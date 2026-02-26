//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const commandTimeout = 30 * time.Second

// Harness wraps hive binary invocations with isolated environment per test.
type Harness struct {
	t          *testing.T
	dataDir    string
	homeDir    string
	configPath string
}

// NewHarness creates a new test harness with isolated temp directories.
func NewHarness(t *testing.T) *Harness {
	t.Helper()

	dataDir := t.TempDir()
	homeDir := t.TempDir()

	// Configure git user for the test home
	gitConfigDir := filepath.Join(homeDir, ".gitconfig")
	gitConfig := "[user]\n\tname = Test User\n\temail = test@example.com\n"
	if err := os.WriteFile(gitConfigDir, []byte(gitConfig), 0o644); err != nil {
		t.Fatalf("writing git config: %v", err)
	}

	return &Harness{
		t:          t,
		dataDir:    dataDir,
		homeDir:    homeDir,
		configPath: testdataConfig(),
	}
}

// Run executes hive with the given arguments and returns combined output.
func (h *Harness) Run(args ...string) (string, error) {
	h.t.Helper()
	cmd := h.command(args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// RunInDir executes hive with a specific working directory.
func (h *Harness) RunInDir(dir string, args ...string) (string, error) {
	h.t.Helper()
	cmd := h.command(args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// RunStdout executes hive and returns only stdout (ignoring stderr).
// Use this for commands that produce structured output (JSON) where
// stderr noise (migration logs, etc.) would break parsing.
func (h *Harness) RunStdout(args ...string) (string, error) {
	h.t.Helper()
	cmd := h.command(args...)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			h.t.Logf("stderr from hive %v: %s", args, exitErr.Stderr)
		}
	}
	return string(out), err
}

// RunStdoutInDir executes hive with a specific working directory, returning only stdout.
func (h *Harness) RunStdoutInDir(dir string, args ...string) (string, error) {
	h.t.Helper()
	cmd := h.command(args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			h.t.Logf("stderr from hive %v: %s", args, exitErr.Stderr)
		}
	}
	return string(out), err
}

// RunWithStdin executes hive with stdin input and returns only stdout.
func (h *Harness) RunWithStdin(input string, args ...string) (string, error) {
	h.t.Helper()
	cmd := h.command(args...)
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			h.t.Logf("stderr from hive %v: %s", args, exitErr.Stderr)
		}
	}
	return string(out), err
}

// RunJSONLines executes hive and parses stdout as newline-delimited JSON objects.
// Use for commands that stream JSONL output (ls --json, todo list, msg sub, etc.).
func (h *Harness) RunJSONLines(args ...string) ([]map[string]any, error) {
	h.t.Helper()
	out, err := h.RunStdout(args...)
	if err != nil {
		return nil, err
	}
	return parseJSONLines(strings.TrimSpace(out))
}

// RunJSONLinesInDir executes hive with a specific working directory and parses stdout as JSONL.
func (h *Harness) RunJSONLinesInDir(dir string, args ...string) ([]map[string]any, error) {
	h.t.Helper()
	out, err := h.RunStdoutInDir(dir, args...)
	if err != nil {
		return nil, err
	}
	return parseJSONLines(strings.TrimSpace(out))
}

// RunJSON executes hive and parses stdout as a single JSON object.
// Use for commands that return one structured result (batch, doctor --format json).
func (h *Harness) RunJSON(args ...string) (map[string]any, error) {
	h.t.Helper()
	out, err := h.RunStdout(args...)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		return nil, fmt.Errorf("parsing JSON from hive %v: %w\noutput: %s", args, err, out)
	}
	return result, nil
}

// RunJSONWithStdin executes hive with stdin input and parses stdout as a single JSON object.
func (h *Harness) RunJSONWithStdin(input string, args ...string) (map[string]any, error) {
	h.t.Helper()
	out, err := h.RunWithStdin(input, args...)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		return nil, fmt.Errorf("parsing JSON from hive %v: %w\noutput: %s", args, err, out)
	}
	return result, nil
}

// WithConfig writes custom YAML config to the harness temp dir and updates the config path.
func (h *Harness) WithConfig(yaml string) *Harness {
	h.t.Helper()
	configPath := filepath.Join(h.dataDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(yaml), 0o644); err != nil {
		h.t.Fatalf("writing test config: %v", err)
	}
	h.configPath = configPath
	return h
}

// DataDir returns the isolated data directory path.
func (h *Harness) DataDir() string { return h.dataDir }

// HomeDir returns the isolated home directory path.
func (h *Harness) HomeDir() string { return h.homeDir }

func (h *Harness) command(args ...string) *exec.Cmd {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	h.t.Cleanup(cancel)

	cmd := exec.CommandContext(ctx, hiveBin, args...)
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"TMPDIR=" + os.Getenv("TMPDIR"),
		"TERM=" + os.Getenv("TERM"),
		"HIVE_DATA_DIR=" + h.dataDir,
		"HOME=" + h.homeDir,
		"HIVE_CONFIG=" + h.configPath,
		"HIVE_LOG_LEVEL=debug",
		"HIVE_LOG_FILE=" + filepath.Join(h.dataDir, "hive.log"),
		"NO_COLOR=1",
	}
	// Propagate tmux socket isolation if set
	if tmuxDir := os.Getenv("TMUX_TMPDIR"); tmuxDir != "" {
		cmd.Env = append(cmd.Env, "TMUX_TMPDIR="+tmuxDir)
	}
	return cmd
}

// testdataConfig resolves the path to the test config fixture relative to this source file.
func testdataConfig() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("cannot resolve test source file path")
	}
	return filepath.Join(filepath.Dir(file), "testdata", "config.yaml")
}
