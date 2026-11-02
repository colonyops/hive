// Package tmuxtest gives tests that drive a real tmux server one of their own,
// so they can run on a machine whose tmux server a developer is working in.
//
// Every isolation layer here matters, because the failure it guards against is
// destroying that server, and rerunning the test cannot recover it. A test that
// uses tmux must call Private before anything reads the environment.
package tmuxtest

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/colonyops/hive/pkg/executil"
)

var versionPattern = regexp.MustCompile(`(\d+)\.(\d+)`)

// Run runs a package's tests with no route to a tmux server outside the package,
// for TestMain: os.Exit(tmuxtest.Run(m)). Private isolates a test that wants a
// server; this catches the test that reaches tmux without asking for one, such
// as a has-session deep inside a service it only meant to exercise.
func Run(m *testing.M) int {
	dir, err := os.MkdirTemp("", "hvtmux")
	if err != nil {
		fmt.Fprintf(os.Stderr, "tmuxtest: scratch dir: %v\n", err)
		return 1
	}
	defer func() { _ = os.RemoveAll(dir) }()

	if err := os.Setenv("TMUX_TMPDIR", dir); err != nil {
		fmt.Fprintf(os.Stderr, "tmuxtest: set TMUX_TMPDIR: %v\n", err)
		return 1
	}
	for _, name := range []string{"TMUX", "TMUX_PANE"} {
		if err := os.Unsetenv(name); err != nil {
			fmt.Fprintf(os.Stderr, "tmuxtest: unset %s: %v\n", name, err)
			return 1
		}
	}
	return m.Run()
}

// Require skips unless this machine has a tmux new enough to test against.
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
}

// Server is a tmux server for one test, reached only through its socket. It
// changes nothing in the process environment, so a parallel test can have one.
type Server struct {
	Socket string
	home   string
}

// New starts no process yet: the first command through the server's Command or
// Run brings it up.
func New(t *testing.T) *Server {
	t.Helper()
	Require(t)

	// See Private on why this is not t.TempDir.
	dir, err := os.MkdirTemp("", "hvtmux") //nolint:usetesting // socket path length
	if err != nil {
		t.Fatalf("tmuxtest: scratch dir: %v", err)
	}
	// tmux canonicalizes its socket path, and on macOS the temp dir is a symlink.
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("tmuxtest: scratch dir: %v", err)
	}
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatalf("tmuxtest: home dir: %v", err)
	}

	s := &Server{Socket: filepath.Join(dir, "s"), home: home}
	t.Cleanup(func() {
		_ = s.Command(context.Background(), "", nil, "kill-server").Run()
		_ = os.RemoveAll(dir)
	})
	return s
}

// Command runs tmux against this server, whatever env says. It satisfies the
// terminal code's Runner, so a Manager given this server stays on it.
//
// HOME, XDG_CONFIG_HOME, and ZDOTDIR move for the same reasons Private moves
// them, and -f /dev/null covers a tmux.conf outside HOME.
func (s *Server) Command(ctx context.Context, binary string, env []string, args ...string) *exec.Cmd {
	if binary == "" {
		binary = "tmux"
	}
	cmd := exec.CommandContext(ctx, binary, append([]string{"-f", "/dev/null", "-S", s.Socket}, args...)...)
	cmd.Dir = s.home
	cmd.Env = append(
		executil.WithoutEnv(env, "TMUX", "TMUX_PANE", "TMUX_TMPDIR", "HOME", "XDG_CONFIG_HOME", "ZDOTDIR"),
		"HOME="+s.home, "XDG_CONFIG_HOME="+filepath.Join(s.home, ".config"), "ZDOTDIR="+s.home,
	)
	return cmd
}

// Run is Command for a fixture's own commands, past any test context so cleanup
// can use it.
func (s *Server) Run(args ...string) error {
	return s.Command(context.Background(), "", nil, args...).Run()
}

// Output is Run returning stdout.
func (s *Server) Output(args ...string) (string, error) {
	out, err := s.Command(context.Background(), "", nil, args...).Output()
	return string(out), err
}

// Private reserves a tmux server for one test and returns its socket path.
//
// It rewrites the process environment, so it cannot run in a parallel test.
// Prefer New: use this only for code that resolves its server from the
// environment and takes no Runner.
//
//   - TMUX and TMUX_PANE go, because $TMUX outranks TMUX_TMPDIR and the terminal
//     code deliberately follows it to the server that launched the process.
//   - TMUX_TMPDIR names a directory that exists for the whole test, because tmux
//     silently falls back to /tmp, a developer's default server, when it cannot
//     create its socket directory inside TMUX_TMPDIR.
//   - HOME, XDG_CONFIG_HOME, and ZDOTDIR move, so the private server loads no
//     tmux.conf and a shell probe sources no rc file that names another server.
//
// Commands the test issues directly must also pass the returned path as -S: tmux
// resolves -S ahead of $TMUX and TMUX_TMPDIR, so a kill-server pinned that way
// cannot reach another server even if the environment leaks. The two agree
// because this is exactly the path tmux derives from TMUX_TMPDIR.
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
	// tmux canonicalizes its socket path, and on macOS the temp dir is a symlink.
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("tmuxtest: scratch dir: %v", err)
	}
	t.Setenv("TMUX_TMPDIR", dir)
	unsetenv(t, "TMUX")
	unsetenv(t, "TMUX_PANE")

	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatalf("tmuxtest: home dir: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("ZDOTDIR", home)

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
		kill.Env = executil.WithoutEnv(nil, "TMUX", "TMUX_PANE")
		_ = kill.Run()
		_ = os.RemoveAll(dir)
	})
	return socket
}

// unsetenv removes a variable for the test, where an empty value would still
// reach code that checks whether the variable is set.
func unsetenv(t *testing.T, name string) {
	t.Helper()
	t.Setenv(name, "")
	if err := os.Unsetenv(name); err != nil {
		t.Fatalf("tmuxtest: unset %s: %v", name, err)
	}
}
