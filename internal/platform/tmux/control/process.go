package tmuxcc

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	tmuxbin "github.com/colonyops/hive/internal/platform/tmux/bin"
	"github.com/colonyops/hive/pkg/executil"
)

// process is the seam that makes the reader/exit/join logic testable without
// real tmux: os/exec satisfies it in production, in-memory pipes in tests.
type process interface {
	Start(ctx context.Context) (stdin io.Writer, stdout io.Reader, err error)
	Wait() error
	Kill() error
}

// defaultBinary is the fallback when no caller resolved one: what $PATH says.
// This package holds no discovery policy — locating tmux is the composition
// root's job (ADR tmux-discovery).
const defaultBinary = "tmux"

// Runner builds the tmux commands a Manager runs. LocalRunner runs this
// machine's tmux; SSHRunner runs a server's tmux over ssh.
type Runner interface {
	// Command returns tmux with args, not started. env is the environment of
	// the local process.
	Command(ctx context.Context, binary string, env []string, args ...string) *exec.Cmd
}

// LocalRunner runs tmux on this machine, on the server $TMUX names when this
// process is itself inside tmux.
type LocalRunner struct{}

// Command passes the $TMUX socket explicitly: hive's session-creating commands
// run with $TMUX intact, so when this process is inside tmux the sessions live
// on the server $TMUX names, which outranks TMUX_TMPDIR for those commands but
// not for a scrubbed client. Outside tmux both resolve identically.
func (LocalRunner) Command(ctx context.Context, binary string, env []string, args ...string) *exec.Cmd {
	if binary == "" {
		binary = defaultBinary
	}
	if socket := socketFromTMUX(os.Getenv("TMUX")); socket != "" {
		args = append([]string{"-S", socket}, args...)
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = tmuxbin.WorkDir()
	cmd.Env = withoutTmuxClient(env)
	return cmd
}

type execProcess struct {
	slug   string
	binary string
	env    []string
	runner Runner

	mu     sync.Mutex
	cmd    *exec.Cmd
	stderr *executil.HeadWriter

	waitOnce sync.Once
	waitErr  error
}

func newExecProcess(opts Options) process {
	return &execProcess{
		slug:   opts.Slug,
		binary: opts.Binary,
		env:    opts.Environ,
		runner: opts.Runner,
		stderr: &executil.HeadWriter{Max: 4 << 10},
	}
}

// Start does not bind the process to ctx's cancellation: the control client
// outlives the attach request that spawned it, so it is killed explicitly on
// teardown.
func (p *execProcess) Start(ctx context.Context) (io.Writer, io.Reader, error) {
	cmd := p.runner.Command(context.WithoutCancel(ctx), p.binary, p.env, "-C", "attach", "-t", "="+p.slug)
	cmd.Stderr = p.stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("tmuxcc: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("tmuxcc: stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("tmuxcc: start tmux: %w", err)
	}

	p.mu.Lock()
	p.cmd = cmd
	p.mu.Unlock()
	return stdin, stdout, nil
}

// Wait is callable more than once: the attach path reports why a child died
// before the handshake, and teardown reaps it again.
func (p *execProcess) Wait() error {
	p.waitOnce.Do(func() {
		p.mu.Lock()
		cmd := p.cmd
		p.mu.Unlock()
		if cmd == nil {
			return
		}
		p.waitErr = cmd.Wait()
		if p.waitErr != nil {
			if msg := strings.TrimSpace(p.stderr.String()); msg != "" {
				p.waitErr = fmt.Errorf("%w: %s", p.waitErr, msg)
			}
		}
	})
	return p.waitErr
}

func (p *execProcess) Kill() error {
	p.mu.Lock()
	cmd := p.cmd
	p.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}

// runTmux runs a one-shot tmux command against the server the control clients
// attach to, returning stdout as lines and folding tmux's own complaint into the
// error, since that is all a failed has-session or rename-session reports.
//
// env is what the command client runs with, and a new-session's pane inherits
// it: a tmux pane takes its environment from the client that created it, so
// this is the only place the app can put anything on an agent's PATH.
func runTmux(ctx context.Context, runner Runner, binary string, env []string, args ...string) ([]string, error) {
	out, err := outputTmux(ctx, runner, binary, env, args...)
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimRight(string(out), "\n")
	if trimmed == "" {
		return nil, nil
	}
	return strings.Split(trimmed, "\n"), nil
}

// inputTmux runs a one-shot tmux command with content on its stdin. It exists
// for load-buffer, the one command this package runs that carries arbitrary
// bytes rather than arguments: passing pasted text as an argument would publish
// it in the process table, where anything running as this user can read it.
func inputTmux(ctx context.Context, runner Runner, binary string, env []string, stdin io.Reader, args ...string) error {
	cmd := runner.Command(ctx, binary, env, args...)
	cmd.Stdin = stdin
	stderr := &executil.HeadWriter{Max: 4 << 10}
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
		return err
	}
	return nil
}

func outputTmux(ctx context.Context, runner Runner, binary string, env []string, args ...string) ([]byte, error) {
	cmd := runner.Command(ctx, binary, env, args...)
	stderr := &executil.HeadWriter{Max: 4 << 10}
	cmd.Stderr = stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("%w: %s", err, msg)
		}
		return nil, err
	}
	return out, nil
}

// socketFromTMUX extracts the server socket path from a $TMUX value
// ("<socket>,<pid>,<session>"). Empty when unset or malformed.
func socketFromTMUX(v string) string {
	socket, _, found := strings.Cut(v, ",")
	if !found {
		return ""
	}
	return socket
}

// RunnerEnviron adapts Desktop's resolved environment for one-shot tmux
// commands without exposing the parent client's tmux variables to child panes.
func RunnerEnviron(environ func(context.Context) []string) func(context.Context) []string {
	return func(ctx context.Context) []string {
		if environ == nil {
			return withoutTmuxClient(nil)
		}
		return withoutTmuxClient(environ(ctx))
	}
}

// RunnerArgs keeps one-shot commands on an inherited custom tmux socket after
// RunnerEnviron removes the parent client's tmux variables.
func RunnerArgs(args []string) []string {
	socket := socketFromTMUX(os.Getenv("TMUX"))
	if socket == "" {
		return args
	}
	return append([]string{"-S", socket}, args...)
}

// withoutTmuxClient drops base's tmux client variables so the command runs as
// an independent client rather than nesting. A nil base is this process's own
// environment, which is what a caller outside the app's composition root gets.
func withoutTmuxClient(base []string) []string {
	return executil.WithoutEnv(base, "TMUX", "TMUX_PANE")
}
