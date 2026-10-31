// Package serverattach opens the tmux session of a session on a server in a
// terminal on this machine, over `ssh -t <host> -- tmux attach-session`.
// The daemon does not carry terminal traffic, so ssh does.
package serverattach

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"

	"github.com/colonyops/hive/internal/domain/server"
	"github.com/colonyops/hive/internal/platform/sshdial"
	"github.com/colonyops/hive/pkg/tmpl"
)

// ErrSessionCompleted is a session whose commands all finished, so it left no
// terminal to attach to.
var ErrSessionCompleted = errors.New("the session's commands finished, so it has no terminal to attach to")

// Target is the tmux target of window in tmuxSession. An empty window is the
// session's current window. A pane id stands alone because tmux does not
// resolve it after a session name, and it is unique on its server.
func Target(tmuxSession, window string) string {
	switch {
	case window == "":
		return "=" + tmuxSession
	case strings.HasPrefix(window, "%"):
		return window
	default:
		return "=" + tmuxSession + ":" + window
	}
}

// Command is the argv that attaches a terminal to target on h.
func Command(h server.Host, target string) []string {
	tmux := "tmux"
	if h.Tmux != "" {
		tmux = sshdial.RemotePath(h.Tmux)
	}
	args := append([]string{"ssh"}, sshdial.TerminalOptions(h)...)
	return append(args, h.Name, "--", tmux, "attach-session", "-t", sshdial.Quote(target))
}

// Run attaches this process's terminal and returns when the user detaches.
// A failed connection reports ssh's last stderr line, which names the cause. An
// error from tmux on the server reaches the user over the terminal instead.
func Run(ctx context.Context, h server.Host, target string) error {
	cmd, report := Prepare(ctx, h, target)
	cmd.Stdin, cmd.Stdout = os.Stdin, os.Stdout
	return report(cmd.Run())
}

// Prepare builds the attach for a caller that runs it, which sets its stdin
// and stdout. report turns the command's result into the error to show, with
// ssh's last stderr line, because the terminal may not keep that line on
// screen after ssh ends.
func Prepare(ctx context.Context, h server.Host, target string) (cmd *exec.Cmd, report func(error) error) {
	argv := Command(h, target)
	last := &lastLine{}
	cmd = exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // argv is built from the config's ssh target
	cmd.Stderr = io.MultiWriter(os.Stderr, last)
	return cmd, func(err error) error {
		if err == nil {
			return nil
		}
		return explain(h, err, last.String())
	}
}

// OpenWindow opens the attach in a new window of the tmux this process runs
// in, so the caller keeps its own window. The window stays open after a
// failed ssh, so the user can read why.
func OpenWindow(ctx context.Context, h server.Host, target, windowName string) error {
	args := []string{"new-window", "-n", windowName}
	// The window starts in the tmux server's directory, where a relative
	// ssh_config path would not resolve.
	if cwd, err := os.Getwd(); err == nil {
		args = append(args, "-c", cwd)
	}
	// sh, because tmux otherwise runs the script with the user's default-shell,
	// which may not be POSIX.
	args = append(args, "--", "sh", "-c", windowScript(h, target, os.Getenv("SSH_AUTH_SOCK")))
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "tmux", args...) //nolint:gosec // windowScript quotes every argument
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("open a tmux window for %s: %w: %s", windowName, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// windowScript carries authSock because the window gets the tmux server's
// environment, which can name a different ssh agent than the one that just
// reached the server, and BatchMode cannot ask for a password instead.
func windowScript(h server.Host, target, authSock string) string {
	var script strings.Builder
	if authSock != "" {
		script.WriteString("SSH_AUTH_SOCK=" + tmpl.ShellQuote(authSock) + " ")
	}
	for _, arg := range Command(h, target) {
		script.WriteString(tmpl.ShellQuote(arg) + " ")
	}
	script.WriteString(`|| { printf '\nssh exited with status %s. Press enter to close.' "$?"; read -r _; }`)
	return script.String()
}

func explain(h server.Host, err error, stderr string) error {
	if stderr == "" {
		return fmt.Errorf("ssh %s: %w", h.Name, err)
	}
	return fmt.Errorf("ssh %s: %w: %s", h.Name, err, stderr)
}

// sshClosedNote is what ssh prints whenever a session with a terminal ends. It
// names no cause, and on a failed attach it would read as a network fault.
var sshClosedNote = regexp.MustCompile(`^(Shared c|C)onnection to \S+ closed\.$`)

// lastLine keeps the last non-empty line written to it, other than
// sshClosedNote.
type lastLine struct {
	mu      sync.Mutex
	partial []byte
	last    string
}

func (l *lastLine) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.partial = append(l.partial, p...)
	for {
		i := bytes.IndexByte(l.partial, '\n')
		if i < 0 {
			break
		}
		if line := strings.TrimSpace(string(l.partial[:i])); line != "" && !sshClosedNote.MatchString(line) {
			l.last = line
		}
		l.partial = l.partial[i+1:]
	}
	return len(p), nil
}

func (l *lastLine) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if line := strings.TrimSpace(string(l.partial)); line != "" {
		return line
	}
	return l.last
}
