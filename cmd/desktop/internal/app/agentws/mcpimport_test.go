package agentws

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMCPImportAcceptsBothShapes(t *testing.T) {
	t.Parallel()

	wrapped := `{"mcpServers": {"playwright": {"command": "npx", "args": ["-y", "@playwright/mcp@latest"], "autoApprove": ["ignored"]}}}`
	servers, err := ParseMCPImport([]byte(wrapped))
	require.NoError(t, err)
	require.Contains(t, servers, "playwright")
	assert.Equal(t, "npx", servers["playwright"].Command)
	assert.Equal(t, []string{"-y", "@playwright/mcp@latest"}, servers["playwright"].Args)

	bare := `{"hass": {"type": "http", "url": "http://homeassistant.local:8123/mcp", "headers": {"Authorization": "Bearer x"}}}`
	servers, err = ParseMCPImport([]byte(bare))
	require.NoError(t, err)
	require.Contains(t, servers, "hass")
	assert.Equal(t, "http://homeassistant.local:8123/mcp", servers["hass"].URL)
}

func TestParseMCPImportRejectsWhatTheLoaderWould(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"not json":              `nope`,
		"empty object":          `{}`,
		"bare single server":    `{"command": "npx"}`,
		"stdio without command": `{"x": {"args": ["-y"]}}`,
		"http without url":      `{"x": {"type": "http"}}`,
		"unknown transport":     `{"x": {"type": "carrier-pigeon", "command": "npx"}}`,
	}
	for name, input := range cases {
		_, err := ParseMCPImport([]byte(input))
		assert.Error(t, err, name)
	}
}
