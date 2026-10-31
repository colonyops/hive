package tmuxcc

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/colonyops/hive/internal/domain/server"
)

func TestSSHRunner_QuotesTheRemoteCommand(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-501/default,1,0")
	r := SSHRunner{Host: server.Host{Name: "ada@box", ConfigFile: "/cfg"}}

	cmd := r.Command(t.Context(), "", nil, "display-message", "-p", "-t", "=it's", "#{pane_current_path}")

	args := cmd.Args
	assert.Equal(t, "ssh", args[0])
	assert.Equal(t, []string{"-F", "/cfg", "-T"}, args[1:4])
	assert.NotContains(t, args, "-S", "a local tmux socket must not reach the server")
	assert.Equal(t, []string{"ada@box", "--", `'tmux' 'display-message' '-p' '-t' '=it'\''s' '#{pane_current_path}'`}, args[len(args)-3:])
}

func TestSSHRunner_ExpandsAHomeRelativeTmuxOnTheServer(t *testing.T) {
	cmd := SSHRunner{Host: server.Host{Name: "box"}}.Command(t.Context(), "~/bin/tmux", nil, "-V")
	assert.Equal(t, `"$HOME"/'bin/tmux' '-V'`, cmd.Args[len(cmd.Args)-1])
}

func TestSSHRunner_SharesAConnectionThroughControlDir(t *testing.T) {
	cmd := SSHRunner{Host: server.Host{Name: "box"}, ControlDir: "/tmp/hive-501/ssh"}.Command(t.Context(), "tmux", nil, "-V")

	assert.Contains(t, cmd.Args, "ControlMaster=auto")
	assert.Contains(t, cmd.Args, "ControlPath=/tmp/hive-501/ssh/%C")
	assert.Contains(t, cmd.Args, "ControlPersist=60")

	plain := SSHRunner{Host: server.Host{Name: "box"}}.Command(t.Context(), "tmux", nil, "-V")
	assert.NotContains(t, plain.Args, "ControlMaster=auto")
}

func TestSSHRunner_DropsTheLocalTmuxClient(t *testing.T) {
	cmd := SSHRunner{Host: server.Host{Name: "box"}}.Command(t.Context(), "tmux", []string{"TMUX=/s,1,0", "TMUX_PANE=%1", "SSH_AUTH_SOCK=/agent"}, "-V")
	assert.Equal(t, []string{"SSH_AUTH_SOCK=/agent"}, cmd.Env)
}
