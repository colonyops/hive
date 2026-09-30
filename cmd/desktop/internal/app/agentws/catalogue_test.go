package agentws

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func findEntry(t *testing.T, entries []CatalogueEntry, id string) CatalogueEntry {
	t.Helper()
	for _, e := range entries {
		if e.ID == id {
			return e
		}
	}
	t.Fatalf("no catalogue entry %q", id)
	return CatalogueEntry{}
}

func TestCatalogueUserEntryShadowsShipped(t *testing.T) {
	t.Parallel()

	lib := Library{Version: 1, Servers: map[string]MCPServer{
		"playwright": {Command: "/opt/homebrew/bin/custom-playwright"},
	}}

	entries := Catalogue(t.Context(), lib, nil)
	entry := findEntry(t, entries, "playwright")

	assert.False(t, entry.Shipped, "the user entry, not the shipped one, must win")
	assert.Equal(t, "playwright", entry.Shadows)
	assert.Equal(t, "/opt/homebrew/bin/custom-playwright", entry.Server.Command)

	// Only one "playwright" row — the shipped entry must not also appear.
	count := 0
	for _, e := range entries {
		if e.ID == "playwright" {
			count++
		}
	}
	assert.Equal(t, 1, count)
}

func TestCatalogueUnshadowedShippedEntryStillAppears(t *testing.T) {
	t.Parallel()

	entries := Catalogue(t.Context(), Library{Version: 1}, nil)
	entry := findEntry(t, entries, "playwright")
	assert.True(t, entry.Shipped)
	assert.Empty(t, entry.Shadows)
}

func TestCatalogueFlagsACommandNotOnPATH(t *testing.T) {
	t.Parallel()

	// os.Args[0] is the compiled test binary: guaranteed to exist and be
	// executable without depending on PATH content.
	real := os.Args[0]

	lib := Library{Version: 1, Servers: map[string]MCPServer{
		"ghost": {Command: "definitely-not-a-real-binary-xyz"},
		"real":  {Command: real},
	}}

	entries := Catalogue(t.Context(), lib, nil)
	ghost := findEntry(t, entries, "ghost")
	realEntry := findEntry(t, entries, "real")

	assert.NotEmpty(t, ghost.Problem)
	assert.Empty(t, realEntry.Problem)
}

// The check has to ask the PATH a session launches with, not this process's:
// a desktop launch inherits launchd's /usr/bin:/bin:/usr/sbin:/sbin, where
// none of npx, mise or uvx live, and validating against it warns on every
// stdio entry including the shipped ones (#266).
func TestCatalogueResolvesAgainstTheInjectedLookPath(t *testing.T) {
	t.Parallel()

	lib := Library{Version: 1, Servers: map[string]MCPServer{
		"resolved":   {Command: "hive-test-npx"},
		"unresolved": {Command: "hive-test-uvx"},
	}}

	entries := Catalogue(t.Context(), lib, func(_ context.Context, name string) (string, error) {
		if name == "hive-test-npx" {
			return "/opt/homebrew/bin/hive-test-npx", nil
		}
		return "", exec.ErrNotFound
	})

	assert.Empty(t, findEntry(t, entries, "resolved").Problem)
	assert.NotEmpty(t, findEntry(t, entries, "unresolved").Problem)
}

// The shipped entries are the ones the bug fired on: both are stdio npx
// invocations, so a stock install with npx only on the login shell's PATH
// used to warn on the entries it ships enabled.
func TestCatalogueDoesNotFlagShippedEntriesTheResolverCanFind(t *testing.T) {
	t.Parallel()

	entries := Catalogue(t.Context(), Library{Version: 1}, func(_ context.Context, name string) (string, error) {
		return "/opt/homebrew/bin/" + name, nil
	})

	for _, entry := range entries {
		assert.Emptyf(t, entry.Problem, "shipped entry %q", entry.ID)
	}
}

func TestCatalogueDoesNotFlagNonStdioServers(t *testing.T) {
	t.Parallel()

	lib := Library{Version: 1, Servers: map[string]MCPServer{
		"remote": {Type: "http", URL: "http://example.com/mcp"},
	}}
	entries := Catalogue(t.Context(), lib, nil)
	entry := findEntry(t, entries, "remote")
	require.Empty(t, entry.Problem)
}
