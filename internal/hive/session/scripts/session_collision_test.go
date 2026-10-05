package scripts

import (
	"os/exec"
	"testing"

	"github.com/colonyops/hive/internal/platform/tmuxtest"
	"github.com/stretchr/testify/require"
)

func TestHiveTmuxCreatesAnExactSessionBesideALongerName(t *testing.T) {
	socket := tmuxtest.Private(t)
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
	t.Setenv("HIVE_AGENT_COMMAND", "sleep")
	t.Setenv("HIVE_AGENT_FLAGS", "300")
	t.Setenv("HIVE_AGENT_WINDOW", "agent")
	tmux := func(args ...string) string {
		t.Helper()
		out, err := exec.CommandContext(t.Context(), "tmux", append([]string{"-S", socket, "-f", "/dev/null"}, args...)...).CombinedOutput()
		require.NoErrorf(t, err, "tmux %v: %s", args, out)
		return string(out)
	}
	tmux("new-session", "-d", "-s", "pbac-through-lbac-plumbing", "sleep 300")
	for range 2 {
		out, err := exec.CommandContext(t.Context(), "bash", "bin/hive-tmux", "-b", "pbac", t.TempDir()).CombinedOutput()
		require.NoErrorf(t, err, "hive-tmux: %s", out)
		require.Equal(t, "agent\nshell\n", tmux("list-windows", "-t", "=pbac", "-F", "#{window_name}"))
	}
	require.Equal(t, "pbac\npbac-through-lbac-plumbing\n", tmux("list-sessions", "-F", "#{session_name}"))
}
