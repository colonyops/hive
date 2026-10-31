package serverattach

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/colonyops/hive/internal/domain/server"
)

func TestCommandAttachesWithTheConnectionsSSHOptionsAndATerminal(t *testing.T) {
	argv := Command(server.Host{Name: "box", ConfigFile: "/cfg/ssh config"}, "it's mine")

	assert.Equal(t, []string{
		"ssh", "-F", "/cfg/ssh config", "-t",
		"-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "ConnectTimeout=10",
		"-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=3",
		"box", "--", "tmux", "attach-session", "-t", `'=it'\''s mine'`,
	}, argv)
}

func TestCommandUsesTheTmuxPathOfTheServer(t *testing.T) {
	argv := Command(server.Host{Name: "box", Tmux: "~/bin/tmux"}, "s")

	assert.Contains(t, argv, `"$HOME"/'bin/tmux'`)
	assert.NotContains(t, argv, "tmux")
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
