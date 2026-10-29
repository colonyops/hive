//go:build remote

package remote

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/server"
	"github.com/colonyops/hive/internal/platform/tmux/control"
)

// serverTerminals is the tmux control manager Desktop runs for a server.
func serverTerminals(t *testing.T, s Server) *tmuxcc.Manager {
	t.Helper()
	// /tmp, not t.TempDir or $TMPDIR: ssh's socket path must fit in 104 bytes.
	controlDir, err := os.MkdirTemp("/tmp", "hvssh") //nolint:usetesting // socket path length
	require.NoError(t, err)
	m := tmuxcc.NewManager(t.Context(), tmuxcc.ManagerOptions{
		Logger: zerolog.Nop(),
		Binary: func() (string, error) { return "tmux", nil },
		Runner: tmuxcc.SSHRunner{Host: server.Host{Name: s.Name, ConfigFile: sshConfig}, ControlDir: controlDir},
	})
	t.Cleanup(func() {
		_ = m.Stop(context.WithoutCancel(t.Context()))
		// Ends the shared connection that ControlPersist keeps for a minute.
		_ = run(t, nil, "ssh", "-F", sshConfig, "-o", "ControlPath="+controlDir+"/%C", "-O", "exit", s.Name)
		_ = os.RemoveAll(controlDir)
	})
	return m
}

func TestServerTerminal_AttachTypeResizeAndRecoverFromADrop(t *testing.T) {
	r := serverA.Hive(t, "session", "create", "--json", "--remote", tFixtureRepo, "terminal").Ok(t)
	slug := DecodeJSON[Session](t, r).Name
	serverA.DeleteOnCleanup(t, DecodeJSON[Session](t, r).ID)
	m := serverTerminals(t, serverA)
	ctx := t.Context()

	require.NoError(t, m.Available(ctx))
	exists, err := m.HasSession(ctx, slug)
	require.NoError(t, err)
	require.True(t, exists)
	require.NoError(t, m.CheckLaunch(ctx, slug))

	windows, err := m.Attach(ctx, slug, 100, 30)
	require.NoError(t, err)
	require.Len(t, windows, 1)
	assert.Equal(t, "agent", windows[0].Name)
	client, ok := m.Client(slug)
	require.True(t, ok)

	shellWindow, err := client.NewWindow(ctx)
	require.NoError(t, err)
	pane := awaitPane(t, client, shellWindow)

	require.NoError(t, client.Write(ctx, pane, []byte("echo typed-$((40+2))\r")))
	awaitCapture(t, m, pane, "typed-42")

	// A paste goes through load-buffer, the one command with stdin.
	require.NoError(t, client.Paste(ctx, pane, []byte("echo pasted-$((50+5))")))
	require.NoError(t, client.Write(ctx, pane, []byte("\r")))
	awaitCapture(t, m, pane, "pasted-55")

	require.NoError(t, client.Resize(ctx, 90, 20))
	require.Eventually(t, func() bool {
		out := serverA.Shell(t, "tmux display-message -p -t "+shellQuote(slug+":")+" '#{window_width}x#{window_height}'")
		return out.Err == nil && strings.TrimSpace(out.Stdout) == "90x20"
	}, 10*time.Second, 100*time.Millisecond, "the server's window follows the size vote")

	// Killing the control client on the server ends its ssh, which is what a
	// network drop looks like from here.
	serverA.Shell(t, "pkill -f 'tmux -C attach'").Ok(t)
	require.Eventually(t, func() bool {
		_, attached := m.Client(slug)
		return !attached
	}, 10*time.Second, 50*time.Millisecond, "the client exits when its ssh ends")

	exists, err = m.HasSession(ctx, slug)
	require.NoError(t, err)
	require.True(t, exists, "the tmux session on the server survives the drop")

	_, err = m.Attach(ctx, slug, 100, 30)
	require.NoError(t, err)
	events, unsubscribe, err := m.Subscribe(slug)
	require.NoError(t, err)
	defer unsubscribe()
	awaitOutput(t, events, "typed-42")
}

func awaitPane(t *testing.T, client *tmuxcc.Client, windowID string) string {
	t.Helper()
	var pane string
	require.Eventually(t, func() bool {
		for _, w := range client.Windows() {
			if w.ID == windowID && w.ActivePane != "" {
				pane = w.ActivePane
				return true
			}
		}
		return false
	}, 10*time.Second, 50*time.Millisecond, "window %s never reported its pane", windowID)
	return pane
}

func awaitCapture(t *testing.T, m *tmuxcc.Manager, pane, want string) {
	t.Helper()
	var last string
	require.Eventually(t, func() bool {
		screen, err := m.CapturePaneID(t.Context(), pane)
		last = screen
		return err == nil && strings.Contains(screen, want)
	}, 10*time.Second, 100*time.Millisecond, "pane %s never showed %q; last screen:\n%s", pane, want, last)
}

// awaitOutput reads a re-attach's first paint, which replays the scrollback.
func awaitOutput(t *testing.T, events <-chan tmuxcc.Event, want string) {
	t.Helper()
	var seen strings.Builder
	timeout := time.After(10 * time.Second)
	for {
		select {
		case ev, ok := <-events:
			require.True(t, ok, "the stream closed before %q", want)
			if out, isOutput := ev.(tmuxcc.Output); isOutput {
				seen.Write(out.Data)
				if strings.Contains(seen.String(), want) {
					return
				}
			}
		case <-timeout:
			t.Fatalf("the repaint never showed %q", want)
		}
	}
}
