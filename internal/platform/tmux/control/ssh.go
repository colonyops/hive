package tmuxcc

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/colonyops/hive/internal/domain/server"
	"github.com/colonyops/hive/internal/platform/sshdial"
	tmuxbin "github.com/colonyops/hive/internal/platform/tmux/bin"
)

const sshWaitDelay = 2 * time.Second

// SSHRunner runs a server's tmux over ssh. Control mode reads and writes
// only pipes, so ssh runs without a pty.
type SSHRunner struct {
	Host server.Host
	// ControlDir holds the sockets ssh shares one connection through, so a
	// one-shot command does not pay for a new handshake. Empty means each
	// command opens its own connection.
	ControlDir string
}

// Command ignores env for the remote tmux, which gets the environment of the
// server's login. env is what ssh itself runs with, for SSH_AUTH_SOCK.
func (r SSHRunner) Command(ctx context.Context, binary string, env []string, args ...string) *exec.Cmd {
	if binary == "" {
		binary = defaultBinary
	}
	remote := make([]string, 0, len(args)+1)
	remote = append(remote, sshdial.RemotePath(binary))
	for _, arg := range args {
		remote = append(remote, sshdial.Quote(arg))
	}
	cmd := exec.CommandContext(ctx, "ssh", append(r.options(), r.Host.Name, "--", strings.Join(remote, " "))...) //nolint:gosec // Host.Validate restricts the host and every remote word is quoted
	cmd.Dir = tmuxbin.WorkDir()
	cmd.Env = withoutTmuxClient(env)
	// The background connection of ControlPersist can keep this command's
	// output pipes open, and Wait would then outlive the context.
	cmd.WaitDelay = sshWaitDelay
	return cmd
}

func (r SSHRunner) options() []string {
	opts := sshdial.Options(r.Host)
	if r.ControlDir == "" {
		return opts
	}
	// ControlPersist forks the shared connection into the background, so
	// killing one attach's ssh leaves the others and the next command on it.
	return append(opts,
		"-o", "ControlMaster=auto",
		"-o", "ControlPath="+filepath.Join(r.ControlDir, "%C"),
		"-o", "ControlPersist=60",
	)
}
