//go:build integration

package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// Harness wraps hive binary invocations with isolated environment per test.
type Harness struct {
	t         *testing.T
	dataDir   string
	homeDir   string
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

// DataDir returns the isolated data directory path.
func (h *Harness) DataDir() string { return h.dataDir }

// HomeDir returns the isolated home directory path.
func (h *Harness) HomeDir() string { return h.homeDir }

func (h *Harness) command(args ...string) *exec.Cmd {
	cmd := exec.Command(hiveBin, args...)
	cmd.Env = append(os.Environ(),
		"HIVE_DATA_DIR="+h.dataDir,
		"HOME="+h.homeDir,
		"HIVE_CONFIG="+h.configPath,
		"HIVE_LOG_LEVEL=debug",
		"NO_COLOR=1",
	)
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
