package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const relayWorkspaceManifest = "version: 2\nname: Demo\nagent: claude\ncommand: true\nmcps:\n  - hive-relay\n"

func TestAccountURLEntryWithoutAResolverIsAProblem(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	writeAgentWorkspaceManifest(t, root, "demo", relayWorkspaceManifest)
	svc := newManifestOnlyService(t, root, map[string]string{"claude": "true"})

	assert.NotEmpty(t, mcpCatalogueItem(t, svc, "hive-relay").Problem)

	res, err := svc.Open(t.Context(), "demo")
	require.NoError(t, err)
	assert.Empty(t, res.MissingMCPs, "an unresolved account entry is a problem, not a missing id")
	assert.Contains(t, res.Workspace.Problem, "hive-relay")
}

func TestAccountURLEntryReportsTheResolversError(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	writeAgentWorkspaceManifest(t, root, "demo", relayWorkspaceManifest)
	svc := newManifestOnlyService(t, root, map[string]string{"claude": "true"})
	svc.RegisterAccountResolver("hive-relay", func(context.Context, string) (string, map[string]string, error) {
		return "", nil, errNoRelayAccount
	})

	assert.Empty(t, mcpCatalogueItem(t, svc, "hive-relay").Problem)

	res, err := svc.Open(t.Context(), "demo")
	require.NoError(t, err)
	assert.Empty(t, res.MissingMCPs)
	assert.Contains(t, res.Workspace.Problem, "Settings ▸ Integrations")

	generated, err := os.ReadFile(filepath.Join(root, "demo", ".mcp.json"))
	require.NoError(t, err)
	assert.NotContains(t, string(generated), "hive-relay")
}

func TestAccountURLEntryWritesTheResolvedURLAndHeaders(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	writeAgentWorkspaceManifest(t, root, "demo", relayWorkspaceManifest)
	svc := newManifestOnlyService(t, root, map[string]string{"claude": "true"})
	var asked string
	svc.RegisterAccountResolver("hive-relay", func(_ context.Context, workspace string) (string, map[string]string, error) {
		if workspace == "" {
			return "", nil, errors.New("no workspace")
		}
		asked = workspace
		return "https://relay.example.com/", map[string]string{"Authorization": "Bearer ws-key"}, nil
	})

	res, err := svc.Open(t.Context(), "demo")
	require.NoError(t, err)
	assert.Empty(t, res.Workspace.Problem)
	assert.Equal(t, "demo", asked)

	generated, err := os.ReadFile(filepath.Join(root, "demo", ".mcp.json"))
	require.NoError(t, err)
	assert.Contains(t, string(generated), `"url": "https://relay.example.com/mcp"`)
	assert.Contains(t, string(generated), `"Authorization": "Bearer ws-key"`)
}
