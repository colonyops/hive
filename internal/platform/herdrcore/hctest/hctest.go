// Package hctest starts herdr-core cores for tests, each in scratch state
// that no real core or Herdr app shares.
package hctest

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/colonyops/hive/internal/platform/herdrcore"
)

// Start starts an isolated core and returns its socket. It skips the test
// when hc is not on PATH and stops the core at cleanup.
func Start(t testing.TB) string {
	t.Helper()
	hc, err := exec.LookPath("hc")
	if err != nil {
		t.Skip("hc (herdr-core) is not on PATH")
	}
	// Unix socket paths are short (104 bytes on macOS); t.TempDir is too deep.
	dir, err := os.MkdirTemp("/tmp", "hc")
	if err != nil {
		t.Fatal(err)
	}
	env := []string{
		"HOME=" + dir, "HC_SESSION=t", "XDG_STATE_HOME=" + dir + "/s", "XDG_CONFIG_HOME=" + dir + "/c",
		"XDG_RUNTIME_DIR=", "PATH=/usr/bin:/bin", "SHELL=/bin/sh", "TERM=xterm-256color",
	}
	run := func(args ...string) {
		cmd := exec.Command(hc, args...)
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("hc %v: %v\n%s", args, err, out)
		}
	}
	run("server", "start")
	t.Cleanup(func() {
		run("server", "stop")
		_ = os.RemoveAll(dir)
	})
	socket, err := herdrcore.SocketPath(func(k string) string {
		for _, kv := range env {
			if v, ok := strings.CutPrefix(kv, k+"="); ok {
				return v
			}
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	return socket
}
