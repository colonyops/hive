package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/adapter/httpapi"
	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
)

// What this adapter still serves is the Wails frontend's terminal control
// planes (covered by terminal_test.go, terminal_tmux_test.go,
// popup_terminal_test.go and ctrl_agent_workspaces_test.go) plus the two
// liveness routes below. The agent-facing surface it used to carry — inbox,
// feeds, profiles, actions, source refresh, flow dry runs — is the MCP
// server's now, and its coverage moved with it to
// internal/adapter/mcpsrv (ADR mcp-replaces-the-agent-facing-http-api).

// TestServedOverWebhookListener exercises main.go's real path: MountAPI onto
// the webhook listener, Start binds one loopback port, and the API answers
// over TCP on it — the shared-port design end to end.
func TestServedOverWebhookListener(t *testing.T) {
	root := t.TempDir()
	t.Setenv(settings.EnvDataDir, filepath.Join(root, "data"))
	t.Setenv("HIVE_CONFIG", filepath.Join(root, "hive.yaml"))
	t.Setenv(settings.EnvConfigDir, filepath.Join(root, "config"))
	t.Setenv(settings.EnvMockMode, "feed")
	t.Setenv(settings.EnvHTTPEnabled, "true")
	t.Setenv(settings.EnvHTTPPort, "0")

	cfg, err := settings.NewStore(filepath.Join(root, "config", "settings.yaml")).Effective()
	require.NoError(t, err)
	core, err := app.New(t.Context(), app.Config{Settings: cfg, MockMode: cfg.MockMode(), Logger: zerolog.Nop()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = core.Close() })

	require.True(t, core.MountAPI(httpapi.PathPrefix, httpapi.New(core, zerolog.Nop(), httpapi.Options{}).Handler()),
		"the webhook listener exists, so the API mounts")
	require.NoError(t, core.Start(t.Context()))

	running, port := core.Webhooks.Endpoint(t.Context())
	require.True(t, running)
	require.NotZero(t, port)
	base := fmt.Sprintf("http://127.0.0.1:%d", port)

	statusResp, err := http.Get(base + "/api/status") //nolint:noctx // loopback test
	require.NoError(t, err)
	defer statusResp.Body.Close() //nolint:errcheck // test
	require.Equal(t, http.StatusOK, statusResp.StatusCode, "the API is reachable over the webhook port")

	var status struct {
		Webhook struct {
			Running bool `json:"running"`
			Port    int  `json:"port"`
		} `json:"webhook"`
	}
	require.NoError(t, json.NewDecoder(statusResp.Body).Decode(&status))
	assert.True(t, status.Webhook.Running)
	assert.Equal(t, port, status.Webhook.Port, "status reports the port it was reached on")

	versionResp, err := http.Get(base + "/api/version") //nolint:noctx // loopback test
	require.NoError(t, err)
	defer versionResp.Body.Close() //nolint:errcheck // test
	assert.Equal(t, http.StatusOK, versionResp.StatusCode)
}

// The agent-facing routes are gone rather than deprecated, so a caller still
// pointed at one gets a 404 instead of a stale answer.
func TestRetiredAgentRoutesAreAbsent(t *testing.T) {
	root := t.TempDir()
	t.Setenv(settings.EnvDataDir, filepath.Join(root, "data"))
	t.Setenv("HIVE_CONFIG", filepath.Join(root, "hive.yaml"))
	t.Setenv(settings.EnvConfigDir, filepath.Join(root, "config"))
	t.Setenv(settings.EnvMockMode, "feed")

	core, err := app.New(t.Context(), app.Config{
		Settings: settings.DefaultSettings(), MockMode: settings.MockMode(), Logger: zerolog.Nop(),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = core.Close() })
	handler := httpapi.New(core, zerolog.Nop(), httpapi.Options{}).Handler()

	for _, target := range []string{
		"/api/", "/api/openapi.json", "/api/inbox", "/api/feeds",
		"/api/profiles", "/api/actions", "/api/inbox/events",
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
		assert.Equalf(t, http.StatusNotFound, rec.Code, "%s must be gone, not serving a stale surface", target)
	}
}
