// Package serverattach opens the tmux session of a session on a server in a
// terminal on this machine, over `ssh -t <host> -- tmux attach-session`.
// The daemon does not carry terminal traffic, so ssh does.
package serverattach

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/colonyops/hive/internal/domain/server"
	"github.com/colonyops/hive/internal/platform/sshdial"
	"github.com/colonyops/hive/pkg/tmpl"
)

// Command is the argv that attaches a terminal to tmuxSession on h.
func Command(h server.Host, tmuxSession string) []string {
	args := append([]string{"ssh"}, sshdial.TerminalOptions(h)...)
	return append(args, h.Name, "--", "tmux", "attach-session", "-t", sshdial.Quote("="+tmuxSession))
}

// Run attaches this process's terminal and returns when the user detaches.
// A failed ssh reports its last stderr line, which names the cause.
func Run(ctx context.Context, h server.Host, tmuxSession string) error {
	argv := Command(h, tmuxSession)
	var last lastLine
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // argv is built from the config's ssh target
	cmd.Stdin, cmd.Stdout = os.Stdin, os.Stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, &last)
	if err := cmd.Run(); err != nil {
		return explain(h, err, last.String())
	}
	return nil
}

// OpenWindow opens the attach in a new window of the tmux this process runs
// in, so the caller keeps its own window. The window stays open after a
// failed ssh, so the user can read why.
func OpenWindow(ctx context.Context, h server.Host, tmuxSession, windowName string) error {
	quoted := make([]string, 0, len(Command(h, tmuxSession)))
	for _, arg := range Command(h, tmuxSession) {
		quoted = append(quoted, tmpl.ShellQuote(arg))
	}
	script := strings.Join(quoted, " ") + ` || { printf '\nssh exited with status %s. Press enter to close.' "$?"; read -r _; }`
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "tmux", "new-window", "-n", windowName, script) //nolint:gosec // script quotes every argument
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("open a tmux window for %s: %w: %s", windowName, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func explain(h server.Host, err error, stderr string) error {
	if stderr == "" {
		return fmt.Errorf("ssh %s: %w", h.Name, err)
	}
	return fmt.Errorf("ssh %s: %w: %s", h.Name, err, stderr)
}

// lastLine keeps the last non-empty line written to it.
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
		if line := strings.TrimSpace(string(l.partial[:i])); line != "" {
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
