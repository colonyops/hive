//go:build remote

package remote

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

var (
	stateDir  string
	sshConfig string
	project   string
	hiveBin   string
)

func TestMain(m *testing.M) {
	stateDir = os.Getenv("HIVE_REMOTE_STATE")
	project = os.Getenv("HIVE_REMOTE_PROJECT")
	if stateDir == "" || project == "" {
		fmt.Fprintln(os.Stderr, "skipping remote tests: run them with test/remote/run.sh")
		os.Exit(0)
	}
	sshConfig = filepath.Join(stateDir, "ssh_config")

	binDir, err := os.MkdirTemp("", "hive-remote-client-*")
	if err != nil {
		panic(err)
	}
	hiveBin = filepath.Join(binDir, "hive")
	build := exec.Command("go", "build", "-o", hiveBin, "../..")
	build.Stdout, build.Stderr = os.Stderr, os.Stderr
	if err := build.Run(); err != nil {
		panic(fmt.Sprintf("build the client hive: %v", err))
	}

	code := m.Run()
	_ = os.RemoveAll(binDir)
	os.Exit(code)
}
