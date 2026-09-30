package agentws

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddLibraryServersToTheSeededFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, libraryFileName), []byte(defaultMCPsYAML), 0o600))

	require.NoError(t, AddLibraryServers(root, map[string]MCPServer{
		"home-assistant": {Title: "Home Assistant", Type: "http", URL: "http://homeassistant.local:8123/mcp"},
		"paperless":      {Command: "uvx", Args: []string{"paperless-mcp"}},
	}))

	raw, err := os.ReadFile(filepath.Join(root, libraryFileName))
	require.NoError(t, err)
	assert.Contains(t, string(raw), "# This is your own MCP server library", "the seeded comments survive the edit")

	lib, err := LoadLibrary(filepath.Join(root, libraryFileName))
	require.NoError(t, err)
	assert.Equal(t, "http://homeassistant.local:8123/mcp", lib.Servers["home-assistant"].URL)
	assert.Equal(t, "uvx", lib.Servers["paperless"].Command)
}

func TestAddLibraryServersRefusesADeclaredID(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	body := "version: 1\nservers:\n  paperless:\n    command: keepme\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, libraryFileName), []byte(body), 0o600))

	err := AddLibraryServers(root, map[string]MCPServer{
		"paperless": {Command: "uvx"},
		"fresh":     {Command: "npx"},
	})
	require.ErrorIs(t, err, ErrLibraryServerExists)

	lib, err := LoadLibrary(filepath.Join(root, libraryFileName))
	require.NoError(t, err)
	assert.Equal(t, "keepme", lib.Servers["paperless"].Command, "a refused import writes nothing at all")
	assert.NotContains(t, lib.Servers, "fresh")
}

func TestAddLibraryServersCreatesAMissingFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, AddLibraryServers(root, map[string]MCPServer{"fresh": {Command: "npx"}}))

	lib, err := LoadLibrary(filepath.Join(root, libraryFileName))
	require.NoError(t, err)
	assert.Equal(t, "npx", lib.Servers["fresh"].Command)
}

func TestRemoveLibraryServer(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	body := "version: 1\n# keep me\nservers:\n  paperless:\n    command: uvx\n  other:\n    command: npx\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, libraryFileName), []byte(body), 0o600))

	found, err := RemoveLibraryServer(root, "paperless")
	require.NoError(t, err)
	assert.True(t, found)

	raw, err := os.ReadFile(filepath.Join(root, libraryFileName))
	require.NoError(t, err)
	assert.Contains(t, string(raw), "# keep me")
	lib, err := LoadLibrary(filepath.Join(root, libraryFileName))
	require.NoError(t, err)
	assert.NotContains(t, lib.Servers, "paperless")
	assert.Contains(t, lib.Servers, "other")

	found, err = RemoveLibraryServer(root, "ghost")
	require.NoError(t, err)
	assert.False(t, found)
}
