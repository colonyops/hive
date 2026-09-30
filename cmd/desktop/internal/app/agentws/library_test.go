package agentws

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMCPServerValidate(t *testing.T) {
	t.Parallel()

	t.Run("StdioRequiresCommand", func(t *testing.T) {
		t.Parallel()
		require.Error(t, MCPServer{Type: "stdio"}.Validate())
		require.NoError(t, MCPServer{Type: "stdio", Command: "npx"}.Validate())
	})

	t.Run("EmptyTypeDefaultsToStdio", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, MCPServer{Command: "npx"}.Validate())
		require.Error(t, MCPServer{}.Validate())
	})

	t.Run("StdioRejectsURLAndHeaders", func(t *testing.T) {
		t.Parallel()
		require.Error(t, MCPServer{Command: "npx", URL: "http://example.com"}.Validate())
		require.Error(t, MCPServer{Command: "npx", Headers: map[string]string{"a": "b"}}.Validate())
	})

	t.Run("HTTPRequiresURL", func(t *testing.T) {
		t.Parallel()
		require.Error(t, MCPServer{Type: "http"}.Validate())
		require.NoError(t, MCPServer{Type: "http", URL: "http://example.com"}.Validate())
	})

	t.Run("SSERequiresURL", func(t *testing.T) {
		t.Parallel()
		require.Error(t, MCPServer{Type: "sse"}.Validate())
		require.NoError(t, MCPServer{Type: "sse", URL: "http://example.com"}.Validate())
	})

	t.Run("HTTPRejectsCommandArgsEnv", func(t *testing.T) {
		t.Parallel()
		require.Error(t, MCPServer{Type: "http", URL: "http://example.com", Command: "npx"}.Validate())
		require.Error(t, MCPServer{Type: "http", URL: "http://example.com", Args: []string{"x"}}.Validate())
		require.Error(t, MCPServer{Type: "http", URL: "http://example.com", Env: map[string]string{"a": "b"}}.Validate())
	})

	t.Run("InvalidTypeRejected", func(t *testing.T) {
		t.Parallel()
		require.Error(t, MCPServer{Type: "carrier-pigeon", Command: "npx"}.Validate())
	})
}

func TestLibraryValidate(t *testing.T) {
	t.Parallel()

	t.Run("EmptyServersIsValid", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, Library{Version: 1}.Validate())
	})

	t.Run("EmptyIDRejected", func(t *testing.T) {
		t.Parallel()
		l := Library{Version: 1, Servers: map[string]MCPServer{"": {Command: "npx"}}}
		require.Error(t, l.Validate())
	})

	t.Run("InvalidServerRejected", func(t *testing.T) {
		t.Parallel()
		l := Library{Version: 1, Servers: map[string]MCPServer{"broken": {Type: "http"}}}
		require.Error(t, l.Validate())
	})
}
