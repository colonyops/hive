package serverattach

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/server"
	"github.com/colonyops/hive/internal/platform/sshdial"
)

func TestCommandAttachesWithTheConnectionsSSHOptionsAndATerminal(t *testing.T) {
	argv := Command(server.Host{Name: "box", ConfigFile: "/cfg/ssh config"}, "it's mine")

	assert.Equal(t, []string{
		"ssh", "-F", "/cfg/ssh config", "-t",
		"-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "ConnectTimeout=10",
		"-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=3",
		"box", "--", "sh", "-c", sshdial.Quote(attachScript("tmux", "it's mine")),
	}, argv)
}

func TestAttachScriptFallsBackWhenTheServerDoesNotKnowTheTerminal(t *testing.T) {
	bin := t.TempDir()
	writeScript := func(name, body string) {
		require.NoError(t, os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body+"\n"), 0o700)) //nolint:gosec // test executable
	}
	writeScript("infocmp", `[ "$1" = known-term ]`)
	writeScript("tmux", `echo "$TERM $*"`)

	for _, tc := range []struct{ term, want string }{
		{"known-term", "known-term attach-session -t =it's mine\n"},
		{"xterm-ghostty", "xterm-256color attach-session -t =it's mine\n"},
	} {
		t.Run(tc.term, func(t *testing.T) {
			cmd := exec.Command("/bin/sh", "-c", attachScript("tmux", "it's mine"))
			cmd.Env = []string{"PATH=" + bin + ":/usr/bin:/bin", "TERM=" + tc.term}
			out, err := cmd.Output()
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(out))
		})
	}
}

func TestCommandUsesTheTmuxPathOfTheServer(t *testing.T) {
	argv := Command(server.Host{Name: "box", Tmux: "~/bin/tmux"}, "s")

	assert.Equal(t, sshdial.Quote(attachScript(`"$HOME"/'bin/tmux'`, "s")), argv[len(argv)-1])
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
