package serverattach

import (
	"fmt"
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

func TestLastLineKeepsTheLastNonEmptyLine(t *testing.T) {
	var l lastLine
	_, _ = fmt.Fprint(&l, "Warning: added host\nssh: connect to host box port 22: ")
	_, _ = fmt.Fprint(&l, "Connection refused\n\n")

	assert.Equal(t, "ssh: connect to host box port 22: Connection refused", l.String())
}
