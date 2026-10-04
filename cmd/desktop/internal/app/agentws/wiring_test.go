package agentws

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/mcpcatalog"
)

// TestRenderCodexTOMLEscapesReservedStrings pins the hand-rolled writer's
// escaping byte-for-byte: a command carrying a quote and a backslash, an arg
// carrying a newline, and an env value carrying a backslash and a quote.
// There is no TOML library backing this, so this is this feature's own
// correctness to prove.
func TestRenderCodexTOMLEscapesReservedStrings(t *testing.T) {
	t.Parallel()

	servers := map[string]mcpcatalog.Server{
		"demo": {
			Transport: mcpcatalog.TransportStdio,
			Command:   "npx \"pkg\" \\bin",
			Args:      []string{"line1\nline2"},
			Env:       map[string]string{"TOKEN": "a\\b\"c"},
		},
	}

	got, err := renderCodexTOML(servers)
	require.NoError(t, err)

	want := "[mcp_servers.demo]\ncommand = \"npx \\\"pkg\\\" \\\\bin\"\nargs = [\"line1\\nline2\"]\nenv = { TOKEN = \"a\\\\b\\\"c\" }\n"
	assert.Equal(t, want, string(got))
}

func TestRenderCodexTOMLSortsServersByID(t *testing.T) {
	t.Parallel()

	servers := map[string]mcpcatalog.Server{
		"zulu":  {Transport: mcpcatalog.TransportStdio, Command: "true"},
		"alpha": {Transport: mcpcatalog.TransportStdio, Command: "true"},
	}

	got, err := renderCodexTOML(servers)
	require.NoError(t, err)

	assert.Less(t, strings.Index(string(got), "mcp_servers.alpha"), strings.Index(string(got), "mcp_servers.zulu"))
}

func TestRenderCodexTOMLQuotesAReservedTableKey(t *testing.T) {
	t.Parallel()

	servers := map[string]mcpcatalog.Server{
		"has space": {Transport: mcpcatalog.TransportStdio, Command: "true"},
	}

	got, err := renderCodexTOML(servers)
	require.NoError(t, err)
	assert.Equal(t, "[mcp_servers.\"has space\"]\ncommand = \"true\"\n", string(got))
}

func TestRenderMCPJSONIsKeySortedAndTypesRemoteServers(t *testing.T) {
	t.Parallel()

	servers := map[string]mcpcatalog.Server{
		"zulu-remote": {Transport: mcpcatalog.TransportHttp, URL: "https://example.com/mcp", Headers: map[string]string{"Authorization": "Bearer xyz"}},
		"alpha-local": {Transport: mcpcatalog.TransportStdio, Command: "npx", Args: []string{"-y", "@playwright/mcp@latest"}},
	}

	got, err := renderMCPJSON(servers)
	require.NoError(t, err)
	assert.Less(t, strings.Index(string(got), `"alpha-local"`), strings.Index(string(got), `"zulu-remote"`))

	var decoded struct {
		MCPServers map[string]mcpJSONServer `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal(got, &decoded))
	require.Len(t, decoded.MCPServers, 2)

	local := decoded.MCPServers["alpha-local"]
	assert.Empty(t, local.Type)
	assert.Equal(t, "npx", local.Command)
	assert.Equal(t, []string{"-y", "@playwright/mcp@latest"}, local.Args)

	remote := decoded.MCPServers["zulu-remote"]
	assert.Equal(t, "http", remote.Type)
	assert.Equal(t, "https://example.com/mcp", remote.URL)
	assert.Empty(t, remote.Command)
}

func TestRenderMCPJSONEmptyServersIsAnEmptyObject(t *testing.T) {
	t.Parallel()

	got, err := renderMCPJSON(map[string]mcpcatalog.Server{})
	require.NoError(t, err)
	assert.JSONEq(t, `{"mcpServers":{}}`, string(got))
}

// TestClaudeConversationExistence pins the probe's fail-toward-resume
// polarity: absence is declared only over a readable projects tree, because a
// wrong "absent" abandons a real conversation while a wrong "present" merely
// reproduces the resume error the probe exists to avoid.
func TestClaudeConversationExistence(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	const id = "3f2c8a54-1111-4222-8333-444455556666"

	assert.True(t, HasConversation("claude", id),
		"no projects tree cannot confirm absence — resume and let claude report its own state")

	projectDir := filepath.Join(cfg, "projects", "-some-workspace")
	require.NoError(t, os.MkdirAll(projectDir, 0o700))
	assert.False(t, HasConversation("claude", id),
		"a readable projects tree without the file is a conversation that never persisted")

	require.NoError(t, os.WriteFile(filepath.Join(projectDir, id+".jsonl"), []byte("{}\n"), 0o600))
	assert.True(t, HasConversation("claude", id))
}

func TestHasConversationIsTrueWithoutAProbe(t *testing.T) {
	t.Parallel()

	assert.True(t, HasConversation("codex", "any-id"), "no probe means resume decides for itself")
	assert.True(t, HasConversation("mystery-agent", "any-id"))
}

func TestRenderBearerTokenEnvNamesTheVariableNotItsValue(t *testing.T) {
	t.Setenv("HIVE_AGENT_SESSION_TOKEN", "secret-value")
	servers := map[string]mcpcatalog.Server{
		"hive-orchestrator": {
			Transport:      mcpcatalog.TransportHttp,
			URL:            "http://127.0.0.1:1/mcp/orchestrator",
			Headers:        map[string]string{"X-Extra": "1"},
			BearerTokenEnv: "HIVE_AGENT_SESSION_TOKEN",
		},
	}

	claude, err := renderMCPJSON(servers)
	require.NoError(t, err)
	assert.Contains(t, string(claude), `"Authorization": "Bearer ${HIVE_AGENT_SESSION_TOKEN:-}"`)
	assert.Contains(t, string(claude), `"X-Extra": "1"`)
	assert.NotContains(t, string(claude), "secret-value")
	assert.NotContains(t, servers["hive-orchestrator"].Headers, "Authorization", "rendering must not mutate the catalogue entry")

	codex, err := renderCodexTOML(servers)
	require.NoError(t, err)
	assert.Contains(t, string(codex), `bearer_token_env_var = "HIVE_AGENT_SESSION_TOKEN"`)
	assert.NotContains(t, string(codex), "secret-value")
}
