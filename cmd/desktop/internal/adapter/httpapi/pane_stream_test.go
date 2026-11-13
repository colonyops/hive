package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

// dialPane opens a pane stream and reports the handshake's HTTP status.
func (h *terminalHarness) dialPane(t *testing.T, slug, pane string, header http.Header) (*websocket.Conn, int, error) {
	t.Helper()
	query := url.Values{"slug": {slug}, "pane": {pane}}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	conn, resp, err := websocket.Dial(ctx, "ws"+h.server.URL[len("http"):]+PaneStreamPath+"?"+query.Encode(), &websocket.DialOptions{HTTPHeader: header})
	status := 0
	if resp != nil {
		status = resp.StatusCode
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
	}
	if conn != nil {
		conn.SetReadLimit(-1)
	}
	return conn, status, err
}

// readPaneUntil reads a pane stream until what it has carried contains want.
func readPaneUntil(t *testing.T, conn *websocket.Conn, want string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	var seen bytes.Buffer
	for !strings.Contains(seen.String(), want) {
		_, p, err := conn.Read(ctx)
		require.NoErrorf(t, err, "waiting for %q; got %q", want, seen.String())
		seen.Write(p)
	}
	return seen.String()
}

// A native renderer reads one pane: it starts from a snapshot, its input
// reaches the shell, and a repaint request clears the surface before a fresh
// snapshot, so text that reflowed differently in the surface is redrawn from
// tmux's screen.
func TestPaneStreamCarriesOnePaneAndRepaintsOnRequest(t *testing.T) {
	t.Parallel()
	tmux := startTmux(t, "hive-pane")
	h := newTerminalHarnessOn(t, tmux.server)
	attached := h.attach(t, tmux.slug)
	pane := attached.Windows[0].ActivePane
	session := h.dial(t, tmux.slug)
	defer func() { _ = session.CloseNow() }()
	readUntil(t, session, "the attached lifecycle frame", func(f []byte) bool { return isLifecycle(f, "attached") })

	_, status, err := h.dialPane(t, tmux.slug, pane, http.Header{"Authorization": {"Bearer wrong"}})
	require.Error(t, err)
	require.Equal(t, http.StatusUnauthorized, status)
	_, status, err = h.dialPane(t, tmux.slug, pane, http.Header{"Authorization": {"Bearer " + testToken}, "Origin": {testOrigin}})
	require.Error(t, err, "a page cannot open a pane stream")
	require.Equal(t, http.StatusForbidden, status)

	conn, _, err := h.dialPane(t, tmux.slug, pane, http.Header{"Authorization": {"Bearer " + testToken}})
	require.NoError(t, err)
	defer func() { _ = conn.CloseNow() }()

	require.NoError(t, conn.Write(t.Context(), websocket.MessageBinary, []byte("echo pane-$((40+2))\r")))
	readPaneUntil(t, conn, "pane-42")
	tmux.awaitPane(pane, "pane-42")

	require.NoError(t, conn.Write(t.Context(), websocket.MessageText, []byte(paneRepaint)))
	after := readPaneUntil(t, conn, paneClear)
	if _, snapshot, _ := strings.Cut(after, paneClear); !strings.Contains(snapshot, "pane-42") {
		readPaneUntil(t, conn, "pane-42")
	}
}
