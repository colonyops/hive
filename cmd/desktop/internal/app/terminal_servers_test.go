//go:build !server

package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/platform/tmux/control"
	"github.com/colonyops/hive/pkg/executil"
)

// socketRunner stands a second tmux server on this machine in for a remote
// one, so routing is tested without ssh. test/remote covers the ssh runner.
type socketRunner struct{ socket string }

func (r socketRunner) Command(ctx context.Context, _ string, _ []string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "tmux", append([]string{"-S", r.socket}, args...)...)
	cmd.Env = executil.WithoutEnv(nil, "TMUX", "TMUX_PANE")
	return cmd
}

type fakeTerminalServers map[string]*tmuxcc.Manager

func (f fakeTerminalServers) Manager(_ context.Context, name string) (*tmuxcc.Manager, error) {
	if m, ok := f[name]; ok {
		return m, nil
	}
	return nil, Errorf(KindNotFound, "server %q is not under servers: in config.yaml", name)
}

func newRoutedTerminals(t *testing.T) (*TerminalsService, func(args ...string) error, func(args ...string) error) {
	t.Helper()
	local := privateTmux(t)
	// Not t.TempDir: the test name in that path overflows a unix socket path.
	dir, err := os.MkdirTemp("", "hvbox") //nolint:usetesting // socket path length
	require.NoError(t, err)
	socket := filepath.Join(dir, "box")
	t.Cleanup(func() {
		kill := exec.Command("tmux", "-S", socket, "kill-server") //nolint:noctx // cleanup runs past the test context
		kill.Env = executil.WithoutEnv(nil, "TMUX", "TMUX_PANE")
		_ = kill.Run()
		_ = os.RemoveAll(dir)
	})
	remote := func(args ...string) error { return socketRunner{socket}.Command(t.Context(), "", nil, args...).Run() }

	box := tmuxcc.NewManager(t.Context(), tmuxcc.ManagerOptions{Logger: zerolog.Nop(), Runner: socketRunner{socket}})
	t.Cleanup(func() { _ = box.Stop(context.WithoutCancel(t.Context())) })
	terminals := newTestTerminals(t, &spawningStarter{tmux: local})
	terminals.servers = fakeTerminalServers{"box": box}
	return terminals, local, remote
}

func TestTerminals_RouteAServerSlugToThatServersTmux(t *testing.T) {
	terminals, local, remote := newRoutedTerminals(t)
	require.NoError(t, local("-f", "/dev/null", "new-session", "-d", "-s", "hive-here", "-n", "local", "sh"))
	require.NoError(t, remote("-f", "/dev/null", "new-session", "-d", "-s", "hive-there", "-n", "remote", "sh"))

	windows, err := terminals.Attach(t.Context(), "box:hive-there", 100, 30)
	require.NoError(t, err)
	require.Len(t, windows, 1)
	assert.Equal(t, "remote", windows[0].Name)

	_, err = terminals.Attach(t.Context(), "hive-there", 100, 30)
	assert.Equal(t, KindNotFound, KindOf(err), "a bare slug is local")

	events, unsubscribe, err := terminals.Subscribe(t.Context(), "box:hive-there")
	require.NoError(t, err)
	defer unsubscribe()
	assert.NotNil(t, events)

	require.NoError(t, terminals.Detach(t.Context(), "box:hive-there"))
}

func TestTerminals_SweepKeysServerWindowsByTarget(t *testing.T) {
	terminals, local, remote := newRoutedTerminals(t)
	require.NoError(t, local("-f", "/dev/null", "new-session", "-d", "-s", "hive-here", "-n", "local", "sh"))
	require.NoError(t, remote("-f", "/dev/null", "new-session", "-d", "-s", "hive-there", "-n", "remote", "sh"))

	windows, err := terminals.ListAllWindows(t.Context(), []string{"hive-here", "box:hive-there", "box:hive-gone", "offline:hive-x"})
	require.NoError(t, err, "an unknown or offline server must not fail the sweep")
	assert.ElementsMatch(t, []string{"hive-here", "box:hive-there"}, keys(windows))
	assert.Equal(t, "remote", windows["box:hive-there"][0].Name)
}

// hungRunner stands in for a server that accepts a command and never answers.
type hungRunner struct{}

func (hungRunner) Command(ctx context.Context, _ string, _ []string, _ ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "sleep", "100")
}

func TestTerminals_SweepIsNotHeldByAHungServer(t *testing.T) {
	terminals, local, remote := newRoutedTerminals(t)
	require.NoError(t, local("new-session", "-d", "-s", "here"))
	require.NoError(t, remote("new-session", "-d", "-s", "there"))
	hung := tmuxcc.NewManager(t.Context(), tmuxcc.ManagerOptions{Logger: zerolog.Nop(), Runner: hungRunner{}})
	t.Cleanup(func() { _ = hung.Stop(context.WithoutCancel(t.Context())) })
	box, err := terminals.servers.Manager(t.Context(), "box")
	require.NoError(t, err)
	terminals.servers = fakeTerminalServers{"box": box, "hung": hung}
	terminals.serverSweepBudget = 200 * time.Millisecond

	start := time.Now()
	windows, err := terminals.ListAllWindows(t.Context(), []string{"here", "box:there", "hung:away"})
	require.NoError(t, err)

	assert.Less(t, time.Since(start), 10*time.Second)
	assert.Contains(t, windows, "here")
	assert.Contains(t, windows, "box:there")
	assert.NotContains(t, windows, "hung:away")
}

func TestTerminals_RefuseToStartAServersSession(t *testing.T) {
	terminals, _, _ := newRoutedTerminals(t)

	_, err := terminals.Start(t.Context(), "box:hive-there")
	assert.Equal(t, KindUnavailable, KindOf(err))

	_, err = terminals.Attach(t.Context(), "elsewhere:hive-there", 100, 30)
	assert.Equal(t, KindNotFound, KindOf(err))

	_, err = terminals.Attach(t.Context(), "box:", 100, 30)
	assert.Equal(t, KindInvalid, KindOf(err))
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
