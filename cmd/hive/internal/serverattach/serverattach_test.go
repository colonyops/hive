package serverattach

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/server"
)

func TestCommandAttachesWithTheConnectionsSSHOptionsAndATerminal(t *testing.T) {
	argv := Command(server.Host{Name: "box", ConfigFile: "/cfg/ssh config"}, Target("it's mine", ""))

	assert.Equal(t, []string{
		"ssh", "-F", "/cfg/ssh config", "-t",
		"-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "ConnectTimeout=10",
		"-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=3",
		"box", "--", "tmux", "attach-session", "-t", `'=it'\''s mine'`,
	}, argv)
}

func TestCommandUsesTheTmuxPathOfTheServer(t *testing.T) {
	argv := Command(server.Host{Name: "box", Tmux: "~/bin/tmux"}, Target("s", ""))

	assert.Contains(t, argv, `"$HOME"/'bin/tmux'`)
	assert.NotContains(t, argv, "tmux")
}

func TestTargetNamesTheWindowOrPane(t *testing.T) {
	assert.Equal(t, "=s", Target("s", ""))
	assert.Equal(t, "=s:tests", Target("s", "tests"))
	assert.Equal(t, "%7", Target("s", "%7"))
}

func TestPrepareReportsTheLastLineOfSSH(t *testing.T) {
	cmd, report := Prepare(t.Context(), server.Host{Name: "box"}, Target("s", ""))
	_, _ = fmt.Fprint(cmd.Stderr, "ssh: connect to host box port 22: Connection refused\n")

	require.NoError(t, report(nil))
	assert.EqualError(t, report(errors.New("exit status 255")), "ssh box: exit status 255: ssh: connect to host box port 22: Connection refused")
}

func TestWindowScriptCarriesTheSSHAgentOfThisProcess(t *testing.T) {
	h := server.Host{Name: "box"}

	assert.True(t, strings.HasPrefix(windowScript(h, "s", "/tmp/my agent"), `SSH_AUTH_SOCK='/tmp/my agent' 'ssh' `))
	assert.True(t, strings.HasPrefix(windowScript(h, "s", ""), `'ssh' `))
}

func TestLastLineIgnoresTheNoteSSHPrintsAtTheEnd(t *testing.T) {
	var l lastLine
	_, _ = fmt.Fprint(&l, "Connection to box closed.\n")
	assert.Empty(t, l.String())

	l = lastLine{}
	_, _ = fmt.Fprint(&l, "Connection to box closed by remote host.\nConnection to box closed.\n")
	assert.Equal(t, "Connection to box closed by remote host.", l.String())
}

func TestLastLineKeepsTheLastNonEmptyLine(t *testing.T) {
	var l lastLine
	_, _ = fmt.Fprint(&l, "Warning: added host\nssh: connect to host box port 22: ")
	_, _ = fmt.Fprint(&l, "Connection refused\n\n")

	assert.Equal(t, "ssh: connect to host box port 22: Connection refused", l.String())
}
