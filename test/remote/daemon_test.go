//go:build remote

package remote

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type probeAgent struct {
	Profile string `json:"profile"`
	Command string `json:"command"`
	Path    string `json:"path"`
}

type probeHealth struct {
	Hello struct {
		Protocol int `json:"protocol"`
	} `json:"hello"`
	Health struct {
		Supervised bool         `json:"supervised"`
		Agents     []probeAgent `json:"agents"`
	} `json:"health"`
}

func TestServersProbe_ReportsAMissingAgentAndAnUnsupervisedDaemon(t *testing.T) {
	probe := DecodeJSON[probeHealth](t, NewClient(t).Hive("servers", "probe", "--json", serverA.Name).Ok(t))

	assert.Positive(t, probe.Hello.Protocol)
	assert.False(t, probe.Health.Supervised, "dial-stdio started the fixture daemon, not hive daemon install")
	agents := map[string]probeAgent{}
	for _, agent := range probe.Health.Agents {
		agents[agent.Profile] = agent
	}
	assert.NotEmpty(t, agents["testbash"].Path)
	require.Contains(t, agents, "missing")
	assert.Empty(t, agents["missing"].Path)

	text := NewClient(t).Hive("servers", "probe", serverA.Name).Ok(t)
	assert.Contains(t, text.Stdout, "missing: no-such-agent is not on the daemon's PATH")
}

// The daemon that dial-stdio starts resolves PATH through the login shell,
// so an agent gets the PATH a session created on the server would get, not
// the bare PATH of an ssh command.
func TestSessionAPI_StartsTheAgentWithTheServerLoginPath(t *testing.T) {
	const loginOnly = "/home/hive/.local/bin"
	bare := serverA.Shell(t, `printf %s "$PATH"`).Ok(t).Stdout
	require.NotContains(t, strings.Split(bare, ":"), loginOnly, "the fixture must keep this directory off the bare ssh PATH")

	created := serverA.CreateSession(t, "login-path")
	pane := serverA.Shell(t, "tr '\\0' '\\n' < /proc/$(tmux display -p -t "+shellQuote(created.Name)+" '#{pane_pid}')/environ | sed -n 's/^PATH=//p'").Ok(t).Stdout

	assert.Contains(t, strings.Split(strings.TrimSpace(pane), ":"), loginOnly)
}
