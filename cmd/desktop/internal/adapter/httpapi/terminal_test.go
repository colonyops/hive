package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
	"github.com/colonyops/hive/cmd/desktop/internal/app/tmuxcc"
)

const (
	testToken  = "test-terminal-token"
	testOrigin = "wails://localhost"
)

// terminalHarness is main.go's mount shape: the control plane under /api/ and
// the stream as its own raw mount, both over one server.
type terminalHarness struct {
	core   *app.App
	server *httptest.Server
}

func newTerminalHarness(t *testing.T) *terminalHarness {
	t.Helper()
	root := t.TempDir()
	t.Setenv(settings.EnvDataDir, filepath.Join(root, "data"))
	t.Setenv("HIVE_CONFIG", filepath.Join(root, "hive.yaml"))
	t.Setenv(settings.EnvConfigDir, filepath.Join(root, "config"))
	t.Setenv(settings.EnvMockMode, "feed")

	core, err := app.New(t.Context(), app.Config{
		Settings: settings.DefaultSettings(),
		MockMode: settings.MockMode(),
		Logger:   zerolog.Nop(),
	})
	require.NoError(t, err)

	origins := []string{testOrigin}
	mux := http.NewServeMux()
	mux.Handle(PathPrefix, New(core, zerolog.Nop(), Options{
		TerminalToken: testToken, Origins: origins,
	}).Handler())
	streamPath, stream := TerminalStreamHandler(core, testToken, origins, zerolog.Nop())
	mux.Handle(streamPath, stream)
	ptyStreamPath, ptyStream := PTYStreamHandler(core, testToken, origins, zerolog.Nop())
	mux.Handle(ptyStreamPath, ptyStream)

	h := &terminalHarness{core: core, server: httptest.NewServer(mux)}
	t.Cleanup(func() {
		h.server.Close()
		_ = core.Close()
	})
	return h
}

func (h *terminalHarness) post(t *testing.T, path, token string, body any) *http.Response {
	t.Helper()
	encoded, err := json.Marshal(body)
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, h.server.URL+path, bytes.NewReader(encoded))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := h.server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// streamURL builds the data-plane URL with the pieces a caller wants to break.
func (h *terminalHarness) streamURL(slug, token, version string) string {
	query := url.Values{}
	query.Set("slug", slug)
	query.Set("token", token)
	query.Set("v", version)
	return "ws" + h.server.URL[len("http"):] + TerminalStreamPath + "?" + query.Encode()
}

func TestTerminalControlPlaneRequiresTheBearerToken(t *testing.T) {
	h := newTerminalHarness(t)

	// Detaching a slug nothing is attached to succeeds, so this exercises the
	// token gate alone rather than tmux availability.
	body := map[string]any{"slug": "not-attached"}

	resp := h.post(t, "/api/terminal/detach", "", body)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "no Authorization header is rejected")

	resp = h.post(t, "/api/terminal/detach", "wrong-token", body)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "a wrong token is rejected")

	resp = h.post(t, "/api/terminal/detach", testToken, body)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode, "the right token is served")
}

func TestAgentWindowRequiresAuthenticationAndAProfile(t *testing.T) {
	h := newTerminalHarness(t)
	path := "/api/terminal/windows/agent"
	body := map[string]any{"slug": "review-81", "agent": "codex"}
	resp := h.post(t, path, "", body)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	_ = resp.Body.Close()
	resp = h.post(t, path, "wrong-token", body)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	_ = resp.Body.Close()
	resp = h.post(t, path, testToken, map[string]any{"slug": "review-81"})
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
	_ = resp.Body.Close()
	resp = h.post(t, path, testToken, map[string]any{"slug": "review-81", "agent": "unknown-profile"})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	_ = resp.Body.Close()
}

// 0x0 is the whole unsized case — "nothing measured yet". One measured
// dimension without the other is a caller bug, and a resize is never unsized.
func TestTerminalSizeValidationSeparatesAttachFromResize(t *testing.T) {
	h := newTerminalHarness(t)

	half := h.post(t, "/api/terminal/attach", testToken, map[string]any{"slug": "hive-x", "cols": 0, "rows": 24})
	_ = half.Body.Close()
	assert.Equal(t, http.StatusUnprocessableEntity, half.StatusCode, "half a measurement is rejected")

	unsizedResize := h.post(t, "/api/terminal/resize", testToken, map[string]any{"slug": "hive-x", "cols": 0, "rows": 0})
	_ = unsizedResize.Body.Close()
	assert.Equal(t, http.StatusUnprocessableEntity, unsizedResize.StatusCode, "a resize always carries a size")
}

func TestTerminalControlPlaneAnswersPreflight(t *testing.T) {
	h := newTerminalHarness(t)

	preflight := func(t *testing.T, origin string) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodOptions, h.server.URL+"/api/terminal/attach", nil)
		require.NoError(t, err)
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", "POST")
		resp, err := h.server.Client().Do(req)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })
		return resp
	}

	allowed := preflight(t, testOrigin)
	_ = allowed.Body.Close()
	assert.Equal(t, http.StatusNoContent, allowed.StatusCode)
	assert.Equal(t, testOrigin, allowed.Header.Get("Access-Control-Allow-Origin"))
	assert.Contains(t, allowed.Header.Get("Access-Control-Allow-Methods"), "POST")
	assert.Contains(t, allowed.Header.Get("Access-Control-Allow-Headers"), "Authorization")

	// macOS appends the dev server's per-run port to the webview origin, so the
	// wails scheme matches structurally, not against the list.
	ported := preflight(t, "wails://localhost:53579")
	_ = ported.Body.Close()
	assert.Equal(t, http.StatusNoContent, ported.StatusCode)
	assert.Equal(t, "wails://localhost:53579", ported.Header.Get("Access-Control-Allow-Origin"))

	denied := preflight(t, "https://evil.example")
	_ = denied.Body.Close()
	assert.Equal(t, http.StatusForbidden, denied.StatusCode)
	assert.Empty(t, denied.Header.Get("Access-Control-Allow-Origin"))
}

// The stream rejects before upgrading, so every failure is a plain HTTP status
// the frontend can read rather than a close code it cannot.
func TestTerminalStreamRejectsBadHandshakes(t *testing.T) {
	h := newTerminalHarness(t)

	dial := func(t *testing.T, target string, header http.Header) int {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		conn, resp, err := websocket.Dial(ctx, target, &websocket.DialOptions{HTTPHeader: header})
		if conn != nil {
			_ = conn.CloseNow()
		}
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		require.Error(t, err, "the handshake must not succeed")
		require.NotNil(t, resp)
		return resp.StatusCode
	}

	assert.Equal(t, http.StatusBadRequest, dial(t, h.streamURL("any", testToken, "1"), nil),
		"a wire version whose client frames named windows is refused")
	assert.Equal(t, http.StatusBadRequest, dial(t, h.streamURL("any", testToken, "9"), nil),
		"an unknown wire version is refused")
	assert.Equal(t, http.StatusUnauthorized, dial(t, h.streamURL("any", "wrong-token", terminalWireVersion), nil),
		"a wrong token is refused")
	assert.Equal(t, http.StatusUnauthorized, dial(t, h.streamURL("any", "", terminalWireVersion), nil),
		"a missing token is refused")
	assert.Equal(t, http.StatusForbidden,
		dial(t, h.streamURL("any", testToken, terminalWireVersion), http.Header{"Origin": []string{"https://evil.example"}}),
		"an origin outside the allowlist is refused")
	assert.Equal(t, http.StatusNotFound, dial(t, h.streamURL("not-attached", testToken, terminalWireVersion), nil),
		"a slug with no attached client is a 404, not an upgrade")
}

func TestTerminalFramesRoundTrip(t *testing.T) {
	t.Parallel()

	raw := []byte{0x1b, '[', '1', 'm', 'h', 'i', 0x00, 0xff}

	windowID, paneID, data, err := decodeOutputFrame(encodeOutputFrame("@12", "%34", raw))
	require.NoError(t, err)
	assert.Equal(t, "@12", windowID)
	assert.Equal(t, "%34", paneID)
	assert.Equal(t, raw, data)

	input, err := decodeClientFrame(encodeInputFrame("%34", raw))
	require.NoError(t, err)
	assert.Equal(t, clientFrame{kind: frameInput, paneID: "%34", data: raw}, input)

	chunk, err := decodeClientFrame(append([]byte{framePasteChunk, 0x03, '%', '3', '4'}, raw...))
	require.NoError(t, err)
	assert.Equal(t, clientFrame{kind: framePasteChunk, paneID: "%34", data: raw}, chunk)

	commit, err := decodeClientFrame([]byte{framePasteCommit, 0x03, '%', '3', '4'})
	require.NoError(t, err)
	assert.Equal(t, framePasteCommit, commit.kind)
	assert.Empty(t, commit.data, "a commit carries no payload")

	_, err = decodeClientFrame([]byte{frameOutput, 0x00})
	require.Error(t, err, "a server frame kind is not one a client may send")
	_, err = decodeClientFrame([]byte{frameInput, 0x04, '%', '1'})
	require.Error(t, err, "a truncated id is refused")
	_, err = decodeClientFrame([]byte{frameInput, 0x00})
	require.Error(t, err, "an empty pane id is refused")
}

// The wire kinds are tmuxcc's own strings; a rename here would silently break
// the frontend's tab handling.
func TestTerminalControlFramesCarryStringKinds(t *testing.T) {
	t.Parallel()

	frame, ok := encodeEvent(tmuxcc.LifecycleChanged{Kind: tmuxcc.LifecycleExited, Message: "overflow"})
	require.True(t, ok)
	require.Equal(t, frameLifecycle, frame[0])

	var lifecycle lifecyclePayload
	require.NoError(t, json.Unmarshal(frame[1:], &lifecycle))
	assert.Equal(t, "exited", lifecycle.Kind)
	assert.Equal(t, "overflow", lifecycle.Message)

	split, err := tmuxcc.ParseLayout("f91d,213x55,0,0{106x55,0,0,3,106x55,107,0,4}")
	require.NoError(t, err)
	frame, ok = encodeEvent(tmuxcc.WindowChanged{
		Kind:   tmuxcc.WindowRenamed,
		Window: tmuxcc.Window{ID: "@3", Name: "shell", Active: true, ActivePane: "%4", Width: 213, Height: 55, Layout: split},
	})
	require.True(t, ok)
	require.Equal(t, frameWindowEvent, frame[0])

	var window windowEventPayload
	require.NoError(t, json.Unmarshal(frame[1:], &window))
	assert.Equal(t,
		windowEventPayload{
			Kind:     "renamed",
			WindowID: "@3", Name: "shell", Active: true, ActivePane: "%4", Width: 213, Height: 55,
			Layout: &terminalLayout{
				Split: "leftright", Width: 213, Height: 55,
				Cells: []terminalLayout{
					{PaneID: "%3", Width: 106, Height: 55},
					{PaneID: "%4", Width: 106, Height: 55, X: 107},
				},
			},
		},
		window,
		"every window event carries tmux's size and layout, not only the layout-changed one")

	// The size the renderer must draw at is tmux's, so it rides the frame the
	// frontend already resizes on.
	frame, ok = encodeEvent(tmuxcc.WindowChanged{
		Kind:   tmuxcc.WindowLayoutChanged,
		Window: tmuxcc.Window{ID: "@3", Name: "shell", Width: 80, Height: 24, Zoomed: true},
	})
	require.True(t, ok)
	var unread windowEventPayload
	require.NoError(t, json.Unmarshal(frame[1:], &unread))
	assert.Equal(t, "layout-changed", unread.Kind)
	assert.Equal(t, 80, unread.Width)
	assert.Equal(t, 24, unread.Height)
	assert.True(t, unread.Zoomed)
	assert.Nil(t, unread.Layout)
	assert.NotContains(t, string(frame), `"layout"`, "an unread layout is absent, not empty")
}

// A 0x0 cell never comes from a real tmux, but the converter must not be what
// panics the write pump on one.
func TestWindowEventEncodesAZeroSizedCell(t *testing.T) {
	frame, ok := encodeEvent(tmuxcc.WindowChanged{
		Kind: tmuxcc.WindowLayoutChanged,
		Window: tmuxcc.Window{ID: "@1", Width: 80, Height: 24, Layout: tmuxcc.Layout{
			Split: tmuxcc.SplitLeftRight, Width: 80, Height: 24,
			Cells: []tmuxcc.Layout{
				{Pane: "%1", Width: 80, Height: 24},
				{Split: tmuxcc.SplitTopBottom, X: 80, Cells: []tmuxcc.Layout{{Pane: "%2", X: 80}}},
			},
		}},
	})
	require.True(t, ok)

	var window windowEventPayload
	require.NoError(t, json.Unmarshal(frame[1:], &window))
	require.NotNil(t, window.Layout)
	require.Len(t, window.Layout.Cells, 2)
	assert.Equal(t,
		terminalLayout{Split: "topbottom", X: 80, Cells: []terminalLayout{{PaneID: "%2", X: 80}}},
		window.Layout.Cells[1])
}
