//go:build !server

package app

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/server"
	"github.com/colonyops/hive/internal/hive/servers"
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

	require.NoError(t, terminals.Resize(t.Context(), "box:hive-there", 90, 20), "a call on the attached client routes too")
	require.NoError(t, terminals.Detach(t.Context(), "box:hive-there"))

	killed, err := terminals.Kill(t.Context(), "box:hive-there")
	require.NoError(t, err)
	assert.True(t, killed)
	require.Error(t, remote("has-session", "-t", "=hive-there"))
	require.NoError(t, local("has-session", "-t", "=hive-here"), "killing a server's session leaves the local one")
}

// offlineRunner stands in for a server ssh cannot reach after the probe passed:
// ssh itself exits 255.
type offlineRunner struct{}

func (offlineRunner) Command(ctx context.Context, _ string, _ []string, args ...string) *exec.Cmd {
	if args[0] == "-V" {
		return exec.CommandContext(ctx, "echo", "tmux 3.4")
	}
	return exec.CommandContext(ctx, "sh", "-c", "exit 255")
}

// The frontend stops attaching again on not_found, so the two answers must not
// blur: only the first means the session is gone.
func TestTerminals_AttachTellsAGoneSessionFromAnOfflineServer(t *testing.T) {
	terminals, _, remote := newRoutedTerminals(t)
	require.NoError(t, remote("-f", "/dev/null", "new-session", "-d", "-s", "hive-there", "sh"))
	offline := tmuxcc.NewManager(t.Context(), tmuxcc.ManagerOptions{Logger: zerolog.Nop(), Runner: offlineRunner{}})
	t.Cleanup(func() { _ = offline.Stop(context.WithoutCancel(t.Context())) })
	box, err := terminals.servers.Manager(t.Context(), "box")
	require.NoError(t, err)
	terminals.servers = fakeTerminalServers{"box": box, "offline": offline}

	_, err = terminals.Attach(t.Context(), "box:hive-gone", 100, 30)
	assert.Equal(t, KindNotFound, KindOf(err))

	_, err = terminals.Attach(t.Context(), "offline:hive-there", 100, 30)
	assert.Equal(t, KindInternal, KindOf(err))
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
	// Long enough for the healthy server to answer, far below the hung command.
	terminals.serverSweepBudget = 2 * time.Second

	start := time.Now()
	windows, err := terminals.ListAllWindows(t.Context(), []string{"here", "box:there", "hung:away"})
	require.NoError(t, err)

	assert.Less(t, time.Since(start), 10*time.Second)
	assert.Contains(t, windows, "here")
	assert.Contains(t, windows, "box:there")
	assert.NotContains(t, windows, "hung:away")
}

// barrierServers answers only once every server has been asked, so a sweep that
// asks them one at a time runs the first out of its budget.
type barrierServers struct {
	fakeTerminalServers
	arrivals sync.WaitGroup
	all      chan struct{}
}

func (b *barrierServers) Manager(ctx context.Context, name string) (*tmuxcc.Manager, error) {
	b.arrivals.Done()
	select {
	case <-b.all:
		return b.fakeTerminalServers.Manager(ctx, name)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestTerminals_SweepAsksServersInParallel(t *testing.T) {
	terminals, _, remote := newRoutedTerminals(t)
	require.NoError(t, remote("new-session", "-d", "-s", "there"))
	box, err := terminals.servers.Manager(t.Context(), "box")
	require.NoError(t, err)
	barrier := &barrierServers{fakeTerminalServers: fakeTerminalServers{"box": box, "also": box}, all: make(chan struct{})}
	barrier.arrivals.Add(2)
	go func() {
		barrier.arrivals.Wait()
		close(barrier.all)
	}()
	terminals.servers = barrier
	terminals.serverSweepBudget = 2 * time.Second

	windows, err := terminals.ListAllWindows(t.Context(), []string{"box:there", "also:there"})

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"box:there", "also:there"}, keys(windows))
}

// newRecordedServerTerminals gives managers that run no server and a setter for
// the fleet a reload would swap in.
func newRecordedServerTerminals(t *testing.T) (*serverTerminals, func(...config.ServerConfig)) {
	t.Helper()
	var fleet atomic.Pointer[servers.Fleet]
	set := func(configs ...config.ServerConfig) { fleet.Store(servers.New(zerolog.Nop(), nil, configs, "test")) }
	set()
	terminals := newServerTerminals(t.Context(), zerolog.Nop(), fleet.Load, nil, "")
	terminals.newManager = func(string, server.Host) *tmuxcc.Manager {
		return tmuxcc.NewManager(t.Context(), tmuxcc.ManagerOptions{Logger: zerolog.Nop(), Runner: socketRunner{"/nonexistent/hive-test"}})
	}
	return terminals, set
}

func requireStopped(t *testing.T, m *tmuxcc.Manager) {
	t.Helper()
	require.Eventually(t, func() bool {
		_, err := m.Attach(t.Context(), "probe", 100, 30)
		return errors.Is(err, tmuxcc.ErrUnavailable)
	}, 5*time.Second, 10*time.Millisecond, "the manager was not stopped")
}

func TestServerTerminals_RebuildsAManagerWhenItsHostChanges(t *testing.T) {
	terminals, set := newRecordedServerTerminals(t)
	set(config.ServerConfig{Name: "box", SSH: "box-1"})

	first, err := terminals.Manager(t.Context(), "box")
	require.NoError(t, err)
	again, err := terminals.Manager(t.Context(), "box")
	require.NoError(t, err)
	assert.Same(t, first, again)

	set(config.ServerConfig{Name: "box", SSH: "box-2"})
	rebuilt, err := terminals.Manager(t.Context(), "box")
	require.NoError(t, err)
	assert.NotSame(t, first, rebuilt)
	requireStopped(t, first)
}

func TestServerTerminals_PruneStopsARemovedServerAndKeepsTheRest(t *testing.T) {
	terminals, set := newRecordedServerTerminals(t)
	set(config.ServerConfig{Name: "gone", SSH: "gone"}, config.ServerConfig{Name: "kept", SSH: "kept"})
	gone, err := terminals.Manager(t.Context(), "gone")
	require.NoError(t, err)
	kept, err := terminals.Manager(t.Context(), "kept")
	require.NoError(t, err)

	set(config.ServerConfig{Name: "kept", SSH: "kept"})
	terminals.Prune(t.Context())

	requireStopped(t, gone)
	_, err = terminals.Manager(t.Context(), "gone")
	assert.Equal(t, KindNotFound, KindOf(err))
	same, err := terminals.Manager(t.Context(), "kept")
	require.NoError(t, err)
	assert.Same(t, kept, same)
}

func TestServerTerminals_RefusesAfterStop(t *testing.T) {
	terminals, set := newRecordedServerTerminals(t)
	set(config.ServerConfig{Name: "box", SSH: "box"})
	manager, err := terminals.Manager(t.Context(), "box")
	require.NoError(t, err)

	terminals.Stop(t.Context())

	requireStopped(t, manager)
	_, err = terminals.Manager(t.Context(), "box")
	assert.Equal(t, KindUnavailable, KindOf(err))
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
