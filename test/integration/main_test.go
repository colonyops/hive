//go:build integration

package integration

import (
	"os"
	"os/exec"
	"testing"
)

var hiveBin string

func TestMain(m *testing.M) {
	path, err := exec.LookPath("hive")
	if err != nil {
		panic("hive binary not found in PATH; build it first")
	}
	hiveBin = path

	// Best-effort cleanup of any leftover tmux server
	_ = exec.Command("tmux", "kill-server").Run()

	code := m.Run()

	// Best-effort cleanup after suite
	_ = exec.Command("tmux", "kill-server").Run()

	os.Exit(code)
}
