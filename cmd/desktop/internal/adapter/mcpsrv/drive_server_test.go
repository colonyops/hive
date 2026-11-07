package mcpsrv_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/colonyops/hive/cmd/desktop/internal/app/apptest"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/adapter/mcpsrv"
	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/mcpcatalog"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
)

// testDriveSession is testSession's drive-server twin: same app, same
// in-memory transport, the hive-drive tool table under test instead.
func testDriveSession(t *testing.T) (*app.App, *mcp.ClientSession) {
	t.Helper()
	core, err := app.New(t.Context(), apptest.Config(t))
	require.NoError(t, err)
	t.Cleanup(func() { _ = core.Close() })

	srv := mcpsrv.NewDrive(core, zerolog.Nop(), mcpsrv.Options{Version: "test"}).Server()

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	_, err = srv.Connect(t.Context(), serverTransport, nil)
	require.NoError(t, err)

	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil).
		Connect(t.Context(), clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })

	return core, session
}

// seedAgentSession answers the record id as a chat's agent passes it: the
// text of HIVE_AGENT_SESSION.
func seedAgentSession(t *testing.T, core *app.App, workspace, name string) string {
	t.Helper()
	rec, err := core.Stores.AgentSessions.Create(t.Context(), stores.AgentSessionCreate{
		Workspace: workspace, Name: name, Agent: "claude",
	})
	require.NoError(t, err)
	return rec.ID
}

func TestDriveToolsListDeclaresEveryToolWithAnObjectInputSchema(t *testing.T) {
	t.Parallel()
	_, session := testDriveSession(t)

	res, err := session.ListTools(t.Context(), nil)
	require.NoError(t, err)

	for _, tool := range res.Tools {
		assert.NotEmpty(t, tool.Description, "tool %s has no description", tool.Name)

		require.NotNil(t, tool.InputSchema, "tool %s has no input schema", tool.Name)
		schema, ok := tool.InputSchema.(map[string]any)
		require.True(t, ok, "tool %s input schema is not an object", tool.Name)
		assert.Equal(t, "object", schema["type"], "tool %s input schema is not type object", tool.Name)
	}
}

func TestTabToolsReportTheAsk(t *testing.T) {
	t.Parallel()
	core, session := testDriveSession(t)
	id := seedAgentSession(t, core, "demo", "chat")

	var opened struct {
		Requested string `json:"requested"`
		View      string `json:"view"`
		Path      string `json:"path"`
	}
	call(t, session, "tab_open", map[string]any{"caller": id, "view": "drive"}, &opened)
	assert.Equal(t, "open", opened.Requested, "the result reports the ask, not a tab state nothing acknowledges")
	assert.Equal(t, "drive", opened.View)
	assert.Empty(t, opened.Path)

	text := callErr(t, session, "tab_open", map[string]any{"caller": id, "view": "drive", "path": "ghost"})
	assert.Contains(t, text, "not_found", "pinning the Drive tab to a document requires it to exist")

	var wrote documentView
	call(t, session, "document_put", map[string]any{"caller": id, "path": "plan.md", "body": "x"}, &wrote)
	call(t, session, "tab_open", map[string]any{"caller": id, "view": "drive", "path": "plan.md", "placement": "beside"}, &opened)
	assert.Equal(t, "plan.md", opened.Path)

	var closed struct {
		Requested string `json:"requested"`
		View      string `json:"view"`
	}
	call(t, session, "tab_close", map[string]any{"caller": id, "view": "drive"}, &closed)
	assert.Equal(t, "close", closed.Requested)
	assert.Equal(t, "drive", closed.View)
}

// TestDriveCatalogueAgreement pins the adapter's mount paths to the
// RuntimePaths mcpcatalog declares — the literals live in core so the
// registry stays adapter-free, and this is what keeps them from drifting.
func TestDriveCatalogueAgreement(t *testing.T) {
	t.Parallel()
	desktop, ok := mcpcatalog.Lookup("hive-desktop")
	require.True(t, ok)
	assert.Equal(t, mcpsrv.PathPrefix, desktop.RuntimePath)

	driveEntry, ok := mcpcatalog.Lookup("hive-drive")
	require.True(t, ok)
	assert.Equal(t, mcpsrv.DrivePathPrefix, driveEntry.RuntimePath)

	_, ok = mcpcatalog.Lookup("hive-canvas")
	assert.False(t, ok, "the hive-canvas alias is gone")
}

// The drive mount is an exact-match mux entry beside /mcp; each server
// answers only its own tools, so the mounts must not shadow each other.
func TestDriveMountedBesideTheDesktopServer(t *testing.T) {
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

	require.True(t, core.MountAPI(mcpsrv.PathPrefix,
		mcpsrv.New(core, zerolog.Nop(), mcpsrv.Options{Version: "test"}).Handler()))
	driveHandler := mcpsrv.NewDrive(core, zerolog.Nop(), mcpsrv.Options{Version: "test"}).Handler()
	require.True(t, core.MountAPI(mcpsrv.DrivePathPrefix, driveHandler))
	require.NoError(t, core.Start(t.Context()))

	id := seedAgentSession(t, core, "demo", "chat")

	running, port := core.Webhooks.Endpoint(t.Context())
	require.True(t, running)

	driveSession, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil).
		Connect(t.Context(), &mcp.StreamableClientTransport{
			Endpoint:             fmt.Sprintf("http://127.0.0.1:%d%s", port, mcpsrv.DrivePathPrefix),
			DisableStandaloneSSE: true,
		}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = driveSession.Close() })

	var wrote documentView
	call(t, driveSession, "document_put", map[string]any{"caller": id, "path": "plan.md", "body": "x"}, &wrote)
	assert.Equal(t, "plan.md", wrote.Path)

	// The desktop server still answers at its own path with its own table.
	desktopSession, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil).
		Connect(t.Context(), &mcp.StreamableClientTransport{
			Endpoint:             fmt.Sprintf("http://127.0.0.1:%d%s", port, mcpsrv.PathPrefix),
			DisableStandaloneSSE: true,
		}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = desktopSession.Close() })

	tools, err := desktopSession.ListTools(t.Context(), nil)
	require.NoError(t, err)
	names := make([]string, 0, len(tools.Tools))
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	assert.Contains(t, names, "status_get")
	assert.NotContains(t, names, "document_put", "the drive table must not leak onto the app-control server")
}
