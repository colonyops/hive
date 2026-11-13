package app

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	tmuxcc "github.com/colonyops/hive/internal/platform/tmux/control"
	"github.com/colonyops/hive/internal/platform/tmuxtest"
)

// A natively rendered pane is drawn outside the webview: its output goes to
// the tap, starting from a repaint, and the session stream no longer carries
// it. Closing the tap hands the pane back to the stream.
func TestTapPaneTakesThePaneOffTheSessionStream(t *testing.T) {
	t.Parallel()
	server := tmuxtest.New(t)
	require.NoError(t, server.Run("-f", "/dev/null", "new-session", "-d", "-s", "hive-tap", "-n", "shell", "-x", "80", "-y", "24", "cat"))
	terminals := newTestTerminals(t, server, &spawningStarter{tmux: server.Run})
	ctx := t.Context()

	windows, err := terminals.Attach(ctx, "hive-tap", 80, 24)
	require.NoError(t, err)
	pane := windows[0].ActivePane

	events, unsubscribe, err := terminals.Subscribe(ctx, "hive-tap")
	require.NoError(t, err)
	defer unsubscribe()
	var mu sync.Mutex
	var stream strings.Builder
	streamed := func() string {
		mu.Lock()
		defer mu.Unlock()
		return stream.String()
	}
	go func() {
		for ev := range events {
			if o, ok := ev.(tmuxcc.Output); ok {
				mu.Lock()
				stream.Write(o.Data)
				mu.Unlock()
			}
		}
	}()

	require.NoError(t, terminals.Write(ctx, "hive-tap", pane, []byte("before\r")))
	time.Sleep(200 * time.Millisecond)

	tap, err := terminals.TapPane(ctx, "hive-tap", pane)
	require.NoError(t, err)
	var tapped strings.Builder
	waitTap := func(want string) {
		t.Helper()
		deadline := time.After(5 * time.Second)
		for !strings.Contains(tapped.String(), want) {
			select {
			case p := <-tap.C:
				tapped.Write(p)
			case <-deadline:
				t.Fatalf("tap never carried %q; got %q", want, tapped.String())
			}
		}
	}
	waitTap("before")

	require.NoError(t, terminals.Write(ctx, "hive-tap", pane, []byte("tapped-only\r")))
	waitTap("tapped-only")

	tap.Close()
	require.NoError(t, terminals.Write(ctx, "hive-tap", pane, []byte("back-on-stream\r")))
	require.Eventually(t, func() bool { return strings.Contains(streamed(), "back-on-stream") }, 5*time.Second, 10*time.Millisecond)
	require.NotContains(t, streamed(), "tapped-only")
}
