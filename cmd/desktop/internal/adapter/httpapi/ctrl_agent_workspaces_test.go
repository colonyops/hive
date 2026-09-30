package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
)

// newAgentHarness mirrors newTerminalHarness (terminal_test.go) but mounts only
// the ptyterm stream the agent-workspace surface needs.
func newAgentHarness(t *testing.T) *terminalHarness {
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
	// Sessions ride the shared ptyterm stream, not an agent-specific one
	// (ADR ptyterm-terminals-are-caller-addressed) — mounted the way main.go mounts it.
	ptyStreamPath, ptyStream := PTYStreamHandler(core, testToken, origins, zerolog.Nop())
	mux.Handle(ptyStreamPath, ptyStream)

	h := &terminalHarness{core: core, server: httptest.NewServer(mux)}
	t.Cleanup(func() {
		h.server.Close()
		_ = core.Close()
	})
	return h
}

// The agent routes sit under the terminal prefix so they inherit its
// bearer-token gate rather than declaring a second one (ADR a-workspace-declares-its-own-authority): starting a
// session spawns an agent CLI, arbitrary command execution just as a pop-up
// shell or a tmux attach is. Both a body route and the shared PTY stream a
// session rides must enforce it.
func TestAgentControlPlaneRequiresTheBearerToken(t *testing.T) {
	h := newAgentHarness(t)

	resp := h.post(t, AgentWorkspacesPathPrefix+"workspaces", "", struct{}{})
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "no Authorization header is rejected")

	resp = h.post(t, AgentWorkspacesPathPrefix+"workspaces", "wrong-token", struct{}{})
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "a wrong token is rejected")

	resp = h.post(t, AgentWorkspacesPathPrefix+"workspaces", testToken, struct{}{})
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode, "the right token is served")

	dial := func(target string) int {
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
	assert.Equal(t, http.StatusUnauthorized, dial(h.popupStreamURL("agentws-1", "wrong", ptyWireVersion)),
		"the shared PTY stream a session rides enforces the same token")
	assert.Equal(t, http.StatusUnauthorized, dial(h.popupStreamURL("agentws-1", "", ptyWireVersion)),
		"no token fails the stream the same way")
}

// The phase otherwise has no criterion that drives a handler at all: a valid
// token must return the workspace payload with root populated.
func TestAgentWorkspacesListsOverTheWire(t *testing.T) {
	h := newAgentHarness(t)

	resp := h.post(t, AgentWorkspacesPathPrefix+"workspaces", testToken, struct{}{})
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body agentWorkspacesResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.NotEmpty(t, body.Root, "root is the configured workspace root regardless of availability")
}
