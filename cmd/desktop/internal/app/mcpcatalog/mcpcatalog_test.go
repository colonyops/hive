package mcpcatalog

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegistryDocsBijection is the coupling that lets the catalogue document
// itself: every registered type must carry a docs/<type>.md, and no orphan
// doc may linger for a type that was removed. Mirrors
// internal/app/actions/docs_test.go's TestActionDocsCoverEveryRegisteredType.
func TestRegistryDocsBijection(t *testing.T) {
	for _, mcpType := range Types() {
		doc, err := Doc(mcpType)
		require.NoErrorf(t, err, "MCP type %q has no docs/%s.md", mcpType, mcpType)
		assert.NotEmptyf(t, strings.TrimSpace(doc), "docs/%s.md is empty", mcpType)
		assert.Truef(t, strings.HasPrefix(strings.TrimSpace(doc), "# "),
			"docs/%s.md must open with an H1 naming the MCP type", mcpType)
	}

	entries, err := docsFS.ReadDir("docs")
	require.NoError(t, err)
	for _, entry := range entries {
		mcpType := strings.TrimSuffix(entry.Name(), ".md")
		assert.Containsf(t, registry, mcpType,
			"docs/%s documents %q, which is not a registered MCP type", entry.Name(), mcpType)
	}
}

func TestTypesIsSorted(t *testing.T) {
	types := Types()
	require.Len(t, types, len(registry))
	assert.IsIncreasing(t, types)
}

// TestEveryDescriptorRendersAServer keeps a shipped entry launchable: its
// transport must be one go-enum knows about, and it must carry the fields
// that transport needs to actually connect — a stdio server needs a Command
// to exec, an http/sse server needs a URL to reach.
func TestEveryDescriptorRendersAServer(t *testing.T) {
	for _, mcpType := range Types() {
		descriptor, ok := Lookup(mcpType)
		require.Truef(t, ok, "Types() reported %q but Lookup could not find it", mcpType)

		server := descriptor.Server
		require.Containsf(t, TransportNames(), server.Transport.String(),
			"MCP type %q declares transport %q, not one of %v", mcpType, server.Transport, TransportNames())

		switch {
		case server.Transport == TransportStdio:
			assert.NotEmptyf(t, server.Command, "MCP type %q is stdio but has no Command", mcpType)
		case descriptor.RuntimeURL:
			// This install's own endpoint: the port is allocated at startup,
			// so a URL here would be a guess. It is filled in when the
			// catalogue is rendered, and must be absent until then.
			assert.Emptyf(t, server.URL,
				"MCP type %q declares RuntimeURL but ships a static URL, which would be served instead of the live one", mcpType)
			assert.Truef(t, strings.HasPrefix(descriptor.RuntimePath, "/"),
				"MCP type %q declares RuntimeURL but no absolute RuntimePath, so no live URL could ever be composed", mcpType)
		case server.Transport == TransportHttp, server.Transport == TransportSse:
			assert.NotEmptyf(t, server.URL, "MCP type %q is %s but has no URL", mcpType, server.Transport)
		}
	}
}
