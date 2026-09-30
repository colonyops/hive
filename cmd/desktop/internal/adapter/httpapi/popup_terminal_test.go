package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/ptyterm"
)

func (h *terminalHarness) popupStreamURL(id, token, version string) string {
	query := url.Values{}
	query.Set("id", id)
	query.Set("token", token)
	query.Set("v", version)
	return "ws" + h.server.URL[len("http"):] + PTYStreamPath + "?" + query.Encode()
}

// The pop-up routes sit under the terminal prefix so they inherit its
// bearer-token gate rather than declaring a second one. A regression here would
// leave arbitrary command execution unauthenticated (ADR terminal-transport).
func TestPopupTerminalControlPlaneRequiresTheBearerToken(t *testing.T) {
	h := newTerminalHarness(t)
	body := map[string]any{"id": "nothing-here"}

	resp := h.post(t, PopupTerminalPathPrefix+"close", "", body)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "no Authorization header is rejected")

	resp = h.post(t, PopupTerminalPathPrefix+"close", "wrong-token", body)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "a wrong token is rejected")

	resp = h.post(t, PopupTerminalPathPrefix+"close", testToken, body)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode, "the right token is served")
}

func TestPopupTerminalControlPlaneAnswersPreflight(t *testing.T) {
	h := newTerminalHarness(t)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodOptions, h.server.URL+PopupTerminalPathPrefix+"open", nil)
	require.NoError(t, err)
	req.Header.Set("Origin", testOrigin)
	req.Header.Set("Access-Control-Request-Method", "POST")
	resp, err := h.server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Equal(t, testOrigin, resp.Header.Get("Access-Control-Allow-Origin"))
}

// An id whose terminal is gone answers closed=false rather than 404: an exited
// terminal is dropped, so a caller closing one it already saw exit is not an
// error.
func TestPopupTerminalCloseIsIdempotent(t *testing.T) {
	h := newTerminalHarness(t)

	resp := h.post(t, PopupTerminalPathPrefix+"close", testToken, map[string]any{"id": "t99"})
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body popupCloseResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.False(t, body.Closed)
}

func TestPopupTerminalListsNothingBeforeAnyOpen(t *testing.T) {
	h := newTerminalHarness(t)

	resp := h.post(t, PopupTerminalPathPrefix+"list", testToken, struct{}{})
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body popupListResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Empty(t, body.Terminals)
}

func TestPopupTerminalResizeAlwaysCarriesASize(t *testing.T) {
	h := newTerminalHarness(t)

	resp := h.post(t, PopupTerminalPathPrefix+"resize", testToken, map[string]any{"id": "t1", "cols": 0, "rows": 0})
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

func TestPTYStreamRejectsBadHandshakes(t *testing.T) {
	h := newTerminalHarness(t)

	dial := func(t *testing.T, target string) int {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		conn, resp, err := websocket.Dial(ctx, target, nil) //nolint:bodyclose // closed below
		if conn != nil {
			_ = conn.CloseNow()
		}
		if resp != nil {
			defer func() { _ = resp.Body.Close() }()
			return resp.StatusCode
		}
		require.Error(t, err)
		return 0
	}

	assert.Equal(t, http.StatusBadRequest, dial(t, h.popupStreamURL("t1", testToken, "999")), "a stale wire version fails before the upgrade")
	assert.Equal(t, http.StatusUnauthorized, dial(t, h.popupStreamURL("t1", "wrong", ptyWireVersion)), "a wrong token fails before the upgrade")
	assert.Equal(t, http.StatusNotFound, dial(t, h.popupStreamURL("no-such-terminal", testToken, ptyWireVersion)), "an id with no terminal fails before the upgrade")
}

// The whole path a pop-up actually takes: open over HTTP, stream over the
// socket, and type into a shell this process owns. Everything below it is
// tested without a transport, so this is the one place the two meet.
func TestPTYStreamOpensAndEchoesOverTheWire(t *testing.T) {
	h := newTerminalHarness(t)
	dir := t.TempDir()

	opened := h.post(t, PopupTerminalPathPrefix+"open", testToken, map[string]any{"dir": dir, "cols": 80, "rows": 24})
	defer func() { _ = opened.Body.Close() }()
	require.Equal(t, http.StatusOK, opened.StatusCode)

	var term popupTerminal
	require.NoError(t, json.NewDecoder(opened.Body).Decode(&term))
	require.NotEmpty(t, term.ID)
	require.Equal(t, dir, term.Dir)

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	conn, resp, err := websocket.Dial(ctx, h.popupStreamURL(term.ID, testToken, ptyWireVersion), nil) //nolint:bodyclose // closed below
	// A completed upgrade hands back a 101 whose body is nil.
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	require.NoError(t, err)
	defer func() { _ = conn.CloseNow() }()

	require.NoError(t, conn.Write(ctx, websocket.MessageBinary, append([]byte{popupFrameInput}, "echo hive-popup-ok\n"...)))

	var seen string
	for !strings.Contains(seen, "hive-popup-ok") {
		kind, frame, err := conn.Read(ctx)
		require.NoError(t, err, "saw %q before the echo", seen)
		require.Equal(t, websocket.MessageBinary, kind)
		if frame[0] == popupFrameOutput {
			seen += string(frame[1:])
		}
	}

	closed := h.post(t, PopupTerminalPathPrefix+"close", testToken, map[string]any{"id": term.ID})
	defer func() { _ = closed.Body.Close() }()

	var closeBody popupCloseResponse
	require.NoError(t, json.NewDecoder(closed.Body).Decode(&closeBody))
	assert.True(t, closeBody.Closed)
}

// A launcher is opened by id and brings its own command from the catalog — a
// caller-supplied command beside the id, or an id the catalog does not hold,
// is a refusal. The session-scoping gate has its own test below.
func TestPopupTerminalLauncherOpenRefusesCallerOverrides(t *testing.T) {
	h := newTerminalHarness(t)

	conflict := h.post(t, PopupTerminalPathPrefix+"open", testToken, map[string]any{"launcher": "lazygit", "command": "rm -rf /"})
	defer func() { _ = conflict.Body.Close() }()
	assert.Equal(t, http.StatusBadRequest, conflict.StatusCode, "a launcher brings its own command")

	unknown := h.post(t, PopupTerminalPathPrefix+"open", testToken, map[string]any{"launcher": "no-such-launcher"})
	defer func() { _ = unknown.Body.Close() }()
	assert.Equal(t, http.StatusNotFound, unknown.StatusCode)
}

// The gate an HTTP caller cannot talk its way past: a session-scoped launcher
// takes a live session, and neither omitting the slug nor sending a directory
// beside it opens a terminal (ADR quick-terminal-launchers-are-session-scoped).
func TestPopupTerminalRefusesASessionScopedLauncherWithoutASession(t *testing.T) {
	h := newTerminalHarness(t)

	sessionless := h.post(t, PopupTerminalPathPrefix+"open", testToken, map[string]any{"launcher": "lazygit"})
	defer func() { _ = sessionless.Body.Close() }()
	assert.Equal(t, http.StatusBadRequest, sessionless.StatusCode, "no session is a refusal, not the home directory")

	substituted := h.post(t, PopupTerminalPathPrefix+"open", testToken, map[string]any{"launcher": "lazygit", "dir": t.TempDir()})
	defer func() { _ = substituted.Body.Close() }()
	assert.Equal(t, http.StatusBadRequest, substituted.StatusCode, "a directory does not stand in for the session")

	stale := h.post(t, PopupTerminalPathPrefix+"open", testToken, map[string]any{"launcher": "lazygit", "sessionSlug": "deleted-yesterday"})
	defer func() { _ = stale.Body.Close() }()
	assert.Equal(t, http.StatusNotFound, stale.StatusCode, "a slug whose session is gone is not found")

	listed := h.post(t, PopupTerminalPathPrefix+"list", testToken, struct{}{})
	defer func() { _ = listed.Body.Close() }()
	var body popupListResponse
	require.NoError(t, json.NewDecoder(listed.Body).Decode(&body))
	assert.Empty(t, body.Terminals, "a refused launch spawns nothing")
}

// One socket carries one terminal, so an output frame is a tag and the bytes —
// no ids to parse and nothing for the renderer to decode before writing.
func TestPopupFramesCarryNoIDs(t *testing.T) {
	output, ok := encodePopupEvent(ptyterm.Output{Data: []byte("hello")})
	require.True(t, ok)
	require.Equal(t, popupFrameOutput, output[0])
	assert.Equal(t, "hello", string(output[1:]))

	exit, ok := encodePopupEvent(ptyterm.Exited{Reason: "exited with status 0"})
	require.True(t, ok)
	require.Equal(t, popupFrameExit, exit[0])

	var payload popupExitPayload
	require.NoError(t, json.Unmarshal(exit[1:], &payload))
	assert.Equal(t, popupExitPayload{Reason: "exited with status 0"}, payload)

	data, err := decodePopupInputFrame(append([]byte{popupFrameInput}, "ls\n"...))
	require.NoError(t, err)
	assert.Equal(t, "ls\n", string(data))

	_, err = decodePopupInputFrame([]byte{popupFrameOutput, 'x'})
	assert.Error(t, err, "a server frame is not a valid input frame")
}
