package tmuxtest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/pkg/executil"
)

// decoyServer stands in for the server a developer works in. The test reaches
// it only through -S, the one selector the environment cannot redirect.
func decoyServer(t *testing.T) (socket string, sessions func() string) {
	t.Helper()
	Require(t)
	dir, err := os.MkdirTemp("", "hvdecoy") //nolint:usetesting // socket path length
	require.NoError(t, err)
	socket = filepath.Join(dir, "decoy")

	run := func(args ...string) (string, error) {
		cmd := exec.Command("tmux", append([]string{"-S", socket, "-f", "/dev/null"}, args...)...) //nolint:noctx // cleanup runs past the test context
		cmd.Env = executil.WithoutEnv(nil, "TMUX", "TMUX_PANE")
		out, err := cmd.Output()
		return strings.TrimSpace(string(out)), err
	}
	t.Cleanup(func() {
		_, _ = run("kill-server")
		_ = os.RemoveAll(dir)
	})

	_, err = run("new-session", "-d", "-s", "work", "sleep", "300")
	require.NoError(t, err)
	return socket, func() string {
		listed, err := run("list-sessions", "-F", "#{session_name}")
		require.NoError(t, err, "the decoy server must survive")
		return listed
	}
}

func TestPrivateKeepsAServerTheEnvironmentNamesOutOfReach(t *testing.T) {
	socket, decoySessions := decoyServer(t)
	t.Setenv("TMUX", fmt.Sprintf("%s,1,0", socket))
	t.Setenv("TMUX_PANE", "%0")

	t.Run("bare commands land on the private server", func(t *testing.T) {
		private := Private(t)

		require.Empty(t, os.Getenv("TMUX"))
		for _, args := range [][]string{
			{"-f", "/dev/null", "new-session", "-d", "-s", "private", "sleep", "300"},
			{"kill-session", "-t", "work"},
			{"kill-server"},
		} {
			// The kill-session is expected to fail: no such session here.
			_ = exec.CommandContext(t.Context(), "tmux", args...).Run()
		}

		_, err := os.Stat(private)
		require.NoError(t, err, "the private server created its socket")
	})

	require.Equal(t, "work", decoySessions())
}

func TestPrivateResolvesTheSocketItReturns(t *testing.T) {
	Require(t)
	socket := Private(t)

	require.NoError(t, exec.CommandContext(t.Context(), "tmux", "-f", "/dev/null", "new-session", "-d", "-s", "probe", "sleep", "300").Run())
	out, err := exec.CommandContext(t.Context(), "tmux", "display-message", "-p", "-t", "probe", "#{socket_path}").Output()

	require.NoError(t, err)
	require.Equal(t, socket, strings.TrimSpace(string(out)))
}

func TestPrivateMovesHomeSoTheServerSkipsTheDevelopersConfig(t *testing.T) {
	Require(t)
	developerHome := os.Getenv("HOME")
	socket := Private(t)

	require.NotEqual(t, developerHome, os.Getenv("HOME"))
	require.Contains(t, os.Getenv("HOME"), filepath.Dir(filepath.Dir(socket)))
}

func TestServerStaysOnItsOwnSocketWhateverTheEnvironmentNames(t *testing.T) {
	socket, decoySessions := decoyServer(t)
	t.Setenv("TMUX", fmt.Sprintf("%s,1,0", socket))
	t.Setenv("TMUX_TMPDIR", filepath.Dir(socket))

	server := New(t)
	require.NoError(t, server.Run("new-session", "-d", "-s", "private", "sleep", "300"))
	got, err := server.Output("list-sessions", "-F", "#{session_name}")
	require.NoError(t, err)
	require.Equal(t, "private", strings.TrimSpace(got))
	require.NoError(t, server.Run("kill-server"))

	require.Equal(t, "work", decoySessions())
}
