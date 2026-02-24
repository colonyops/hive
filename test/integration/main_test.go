//go:build integration

package integration

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
)

var hiveBin string

func TestMain(m *testing.M) {
	if os.Getenv("HIVE_INTEGRATION") != "1" {
		fmt.Fprintln(os.Stderr, "skipping integration tests: HIVE_INTEGRATION=1 not set (run inside Docker container)")
		os.Exit(0)
	}

	path, err := exec.LookPath("hive")
	if err != nil {
		panic("hive binary not found in PATH; build it first")
	}
	hiveBin = path

	// Isolate tmux server socket to avoid destroying user sessions
	tmuxDir, err := os.MkdirTemp("", "hive-integration-tmux-*")
	if err != nil {
		panic(fmt.Sprintf("creating tmux tmpdir: %v", err))
	}
	os.Setenv("TMUX_TMPDIR", tmuxDir)
	defer os.RemoveAll(tmuxDir)

	// Best-effort cleanup of the isolated tmux server
	_ = exec.Command("tmux", "kill-server").Run()

	code := m.Run()

	// Best-effort cleanup after suite
	_ = exec.Command("tmux", "kill-server").Run()

	os.Exit(code)
}
