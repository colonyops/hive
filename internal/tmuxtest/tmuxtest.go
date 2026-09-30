// Package tmuxtest gives tests that drive a real tmux server one of their own.
//
// The isolation is deliberately two-layered, because the failure it guards
// against is destroying the tmux server a developer is working in, and that is
// not recoverable by rerunning the test.
package tmuxtest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var versionPattern = regexp.MustCompile(`(\d+)\.(\d+)`)

// Require skips unless this machine can host a throwaway tmux server safely.
//
// Outside CI it refuses to run at all while any tmux server is reachable. That
// is stricter than it needs to be — Private's socket is separate — but the
// isolation only holds as long as every tmux command in the test honours it,
// and a single command that resolves its server from the environment instead
// would take a developer's session with it. Skipping costs a local test run;
// the alternative costs a working session.
func Require(t *testing.T) {
	t.Helper()

	out, err := exec.CommandContext(t.Context(), "tmux", "-V").Output()
	if err != nil {
		t.Skip("tmux is not installed; the real-tmux tests need one")
	}
	match := versionPattern.FindStringSubmatch(string(out))
	if match == nil {
		t.Skipf("could not read a version from %q", strings.TrimSpace(string(out)))
	}
	major, _ := strconv.Atoi(match[1])
	minor, _ := strconv.Atoi(match[2])
	if major < 3 || (major == 3 && minor < 2) {
		t.Skipf("tmux %d.%d is older than the 3.2 control-mode floor", major, minor)
	}

	if os.Getenv("CI") == "" && serverIsRunning(t) {
		t.Skip("a tmux server is already running on this machine; the real-tmux tests run in CI or Docker so they cannot reach it")
	}
}

// serverIsRunning reports whether the tmux a developer would reach right now
// answers. The environment is left alone on purpose: $TMUX names the server
// when the process is inside one, and the default socket answers when it is
// not, so this sees whichever server is at risk.
func serverIsRunning(t *testing.T) bool {
	t.Helper()
	return exec.CommandContext(t.Context(), "tmux", "list-sessions").Run() == nil
}

// Private reserves a tmux server for one test and returns its socket path.
//
// TMUX_TMPDIR is what points the code under test at that server, since it
// resolves its own socket. Commands the test issues directly must instead pass
// the returned path as -S: tmux resolves -S ahead of both $TMUX and
// TMUX_TMPDIR, so a kill-server pinned that way cannot reach another server
// even if the environment leaks. The two agree because this is exactly the
// path tmux derives from TMUX_TMPDIR.
func Private(t *testing.T) string {
	t.Helper()
	Require(t)

	// t.TempDir() bakes the test name into the path, and a tmux socket is a
	// unix socket bound by the ~104 byte sun_path limit — on macOS the two
	// together overflow it.
	dir, err := os.MkdirTemp("", "hvtmux") //nolint:usetesting // socket path length, see above
	if err != nil {
		t.Fatalf("tmuxtest: scratch dir: %v", err)
	}
	t.Setenv("TMUX_TMPDIR", dir)

	// tmux creates this directory itself when it resolves the socket from
	// TMUX_TMPDIR, but not when handed one with -S, and the fixture's first
	// command may be either.
	socketDir := filepath.Join(dir, fmt.Sprintf("tmux-%d", os.Getuid()))
	if err := os.MkdirAll(socketDir, 0o700); err != nil {
		t.Fatalf("tmuxtest: socket dir: %v", err)
	}

	socket := filepath.Join(socketDir, "default")
	t.Cleanup(func() {
		kill := exec.Command("tmux", "-S", socket, "kill-server") //nolint:noctx // cleanup runs past the test context
		kill.Env = ScrubbedEnv()
		_ = kill.Run()
		_ = os.RemoveAll(dir)
	})
	return socket
}

// ScrubbedEnv drops $TMUX/$TMUX_PANE so a command cannot inherit a server from
// the process running the test, matching what tmuxcc's detachedEnv does for the
// control client.
func ScrubbedEnv() []string {
	env := os.Environ()
	kept := env[:0]
	for _, kv := range env {
		if strings.HasPrefix(kv, "TMUX=") || strings.HasPrefix(kv, "TMUX_PANE=") {
			continue
		}
		kept = append(kept, kv)
	}
	return kept
}
