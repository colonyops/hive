package mcpsrv_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

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

type bearerTransport struct{ token string }

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	if b.token != "" {
		r.Header.Set("Authorization", "Bearer "+b.token)
	}
	return http.DefaultTransport.RoundTrip(r)
}

type orchestratorFixture struct {
	core *app.App
	url  string
}

// testOrchestrator serves the orchestrator over HTTP, because its auth reads
// the request header and the in-memory transport has none. It seeds two
// workspaces: orch declares hive-orchestrator, plain does not.
func testOrchestrator(t *testing.T) orchestratorFixture {
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
	t.Cleanup(func() { _ = core.Close() })

	_, err = core.AgentWorkspaces.CreateWorkspace(t.Context(), app.WorkspaceEdit{Dir: "orch", Name: "Orch", Command: "claude", MCPs: []string{mcpcatalog.Orchestrator}})
	require.NoError(t, err)
	_, err = core.AgentWorkspaces.CreateWorkspace(t.Context(), app.WorkspaceEdit{Dir: "plain", Name: "Plain", Command: "claude"})
	require.NoError(t, err)
	for _, ws := range []string{"orch", "plain"} {
		_, err := core.Stores.AgentSessions.Create(t.Context(), stores.AgentSessionCreate{
			Workspace: ws, Name: "chat", Agent: "claude", EndToken: ws + "-token",
		})
		require.NoError(t, err)
	}

	server := httptest.NewServer(mcpsrv.NewOrchestrator(core, zerolog.Nop(), mcpsrv.Options{Version: "test"}).Handler())
	t.Cleanup(server.Close)
	return orchestratorFixture{core: core, url: server.URL}
}

func (f orchestratorFixture) connect(t *testing.T, token string) *mcp.ClientSession {
	t.Helper()
	transport := &mcp.StreamableClientTransport{Endpoint: f.url, HTTPClient: &http.Client{Transport: bearerTransport{token: token}}}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil).Connect(t.Context(), transport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func callJSON(t *testing.T, session *mcp.ClientSession, name string, args map[string]any, out any) *mcp.CallToolResult {
	t.Helper()
	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	if !res.IsError && out != nil {
		data, err := json.Marshal(res.StructuredContent)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(data, out))
	}
	return res
}

func errorText(res *mcp.CallToolResult) string {
	if len(res.Content) == 0 {
		return ""
	}
	if text, ok := res.Content[0].(*mcp.TextContent); ok {
		return text.Text
	}
	return ""
}

func TestOrchestratorCatalogueAgreement(t *testing.T) {
	entry, ok := mcpcatalog.Lookup(mcpcatalog.Orchestrator)
	require.True(t, ok)
	assert.Equal(t, mcpsrv.OrchestratorPathPrefix, entry.RuntimePath)
	assert.Equal(t, "HIVE_AGENT_SESSION_TOKEN", entry.Server.BearerTokenEnv)
}

func TestOrchestratorListsToolsWithoutAToken(t *testing.T) {
	f := testOrchestrator(t)
	session := f.connect(t, "")

	res, err := session.ListTools(t.Context(), nil)
	require.NoError(t, err)
	names := make([]string, 0, len(res.Tools))
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
		assert.NotEmpty(t, tool.Description, "tool %s has no description", tool.Name)
	}
	assert.ElementsMatch(t, []string{
		"list_repositories", "start_session", "list_sessions", "peek_session", "send_prompt", "send_keys",
		"wait_for_session", "publish_message", "wait_for_message", "sleep",
	}, names)
}

func TestOrchestratorRefusesCallsWithoutTheGrant(t *testing.T) {
	f := testOrchestrator(t)
	tests := []struct {
		name  string
		token string
		want  string
	}{
		{"no token", "", "unauthenticated: a session token is required"},
		{"unknown token", "nobody", "unauthenticated: no session holds that token"},
		{"workspace without the server", "plain-token", `unauthenticated: workspace "plain" does not declare the hive-orchestrator MCP server`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := callJSON(t, f.connect(t, tt.token), "list_sessions", map[string]any{}, nil)
			require.True(t, res.IsError)
			assert.Contains(t, errorText(res), tt.want)
		})
	}
}

func TestOrchestratorEveryToolRequiresAToken(t *testing.T) {
	f := testOrchestrator(t)
	session := f.connect(t, "")

	validArgs := map[string]map[string]any{
		"list_repositories": {},
		"start_session":     {"repository": "git@x:y/z", "name": "x", "prompt": "go"},
		"list_sessions":     {},
		"peek_session":      {"session": "s1"},
		"send_prompt":       {"session": "s1", "text": "hi"},
		"send_keys":         {"session": "s1", "keys": []string{"Enter"}},
		"wait_for_session":  {"session": "s1"},
		"publish_message":   {"topic": "t", "payload": "p"},
		"wait_for_message":  {"topic": "t"},
		"sleep":             {"seconds": 1},
	}
	tools, err := session.ListTools(t.Context(), nil)
	require.NoError(t, err)
	for _, tool := range tools.Tools {
		args, ok := validArgs[tool.Name]
		require.True(t, ok, "add valid arguments for %s", tool.Name)
		res := callJSON(t, session, tool.Name, args, nil)
		require.True(t, res.IsError, tool.Name)
		assert.Contains(t, errorText(res), "unauthenticated", tool.Name)
	}
}

func TestOrchestratorMessageRoundTrip(t *testing.T) {
	f := testOrchestrator(t)
	session := f.connect(t, "orch-token")

	var published struct {
		Topics []string `json:"topics"`
	}
	res := callJSON(t, session, "publish_message", map[string]any{"topic": "work.results", "payload": `{"done":true}`}, &published)
	require.False(t, res.IsError, errorText(res))
	assert.Equal(t, []string{"work.results"}, published.Topics)

	var got struct {
		Messages []struct {
			Topic   string `json:"topic"`
			Payload string `json:"payload"`
			Sender  string `json:"sender"`
		} `json:"messages"`
		TimedOut bool `json:"timedOut"`
	}
	res = callJSON(t, session, "wait_for_message", map[string]any{"topic": "work.*", "timeoutSeconds": 1}, &got)
	require.False(t, res.IsError, errorText(res))
	require.Len(t, got.Messages, 1)
	assert.Equal(t, `{"done":true}`, got.Messages[0].Payload)
	assert.Equal(t, "agentws.orch", got.Messages[0].Sender)
	assert.False(t, got.TimedOut)

	got.Messages = nil
	res = callJSON(t, session, "wait_for_message", map[string]any{"topic": "work.results", "timeoutSeconds": 1}, &got)
	require.False(t, res.IsError, errorText(res))
	assert.Empty(t, got.Messages, "an acknowledged message is handed back once")
	assert.True(t, got.TimedOut)
}

func TestOrchestratorSessionLookups(t *testing.T) {
	f := testOrchestrator(t)
	session := f.connect(t, "orch-token")

	var list struct {
		Sessions []json.RawMessage `json:"sessions"`
	}
	res := callJSON(t, session, "list_sessions", map[string]any{}, &list)
	require.False(t, res.IsError, errorText(res))
	assert.NotNil(t, list.Sessions)
	assert.Empty(t, list.Sessions)

	for tool, args := range map[string]map[string]any{
		"peek_session":     {"session": "nope"},
		"send_prompt":      {"session": "nope", "text": "hi"},
		"wait_for_session": {"session": "nope", "timeoutSeconds": 1},
	} {
		res = callJSON(t, session, tool, args, nil)
		require.True(t, res.IsError, tool)
		assert.Contains(t, errorText(res), "not_found", tool)
	}

	res = callJSON(t, session, "start_session", map[string]any{"repository": "", "name": "x", "prompt": "p"}, nil)
	require.True(t, res.IsError)
	assert.Contains(t, errorText(res), "invalid: repository is required")
}

func TestOrchestratorAccessTokens(t *testing.T) {
	f := testOrchestrator(t)
	created, err := f.core.Orchestration.CreateToken(t.Context(), "worker")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(created.Token, "hvo_"))
	assert.Equal(t, created.Token[len(created.Token)-4:], created.Hint)

	_, err = f.core.Orchestration.CreateToken(t.Context(), "Worker")
	assert.Equal(t, app.KindConflict, app.KindOf(err))

	session := f.connect(t, created.Token)
	res := callJSON(t, session, "publish_message", map[string]any{"topic": "jobs", "payload": "x"}, nil)
	require.False(t, res.IsError, errorText(res))
	var got struct {
		Messages []struct {
			Sender string `json:"sender"`
		} `json:"messages"`
	}
	res = callJSON(t, session, "wait_for_message", map[string]any{"topic": "jobs", "timeoutSeconds": 1}, &got)
	require.False(t, res.IsError, errorText(res))
	require.Len(t, got.Messages, 1)
	assert.Equal(t, fmt.Sprintf("token.%d", created.ID), got.Messages[0].Sender)

	tokens, err := f.core.Orchestration.Tokens(t.Context())
	require.NoError(t, err)
	require.Len(t, tokens, 1)
	assert.NotZero(t, tokens[0].LastUsedAt)

	require.NoError(t, f.core.Orchestration.RevokeToken(t.Context(), created.ID))
	res = callJSON(t, session, "list_sessions", map[string]any{}, nil)
	require.True(t, res.IsError)
	assert.Contains(t, errorText(res), "unauthenticated: the access token is unknown or revoked")
}
