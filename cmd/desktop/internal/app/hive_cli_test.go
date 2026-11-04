package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/mcpcatalog"
	"github.com/colonyops/hive/internal/platform/execenv"
)

type fakeHiveCLI struct {
	path string
	ok   bool
}

func (f fakeHiveCLI) Usable(context.Context) (string, bool) { return f.path, f.ok }

func TestServeToolsetsFollowTheOperationCatalogue(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{"actions", "diagnostics", "flows", "inbox", "sessions", "workspaces"}, serveToolsets("/mcp"))
	assert.Equal(t, []string{"drive", "layout"}, serveToolsets("/mcp/drive"))

	for id, want := range map[string]string{"hive-desktop": "/mcp", "hive-drive": "/mcp/drive", "hive-canvas": "/mcp/drive"} {
		d, ok := mcpcatalog.Lookup(id)
		require.True(t, ok)
		assert.Equal(t, want, servePathOf(d), id)
	}
}

func TestCataloguePicksStdioWhenHiveIsUsable(t *testing.T) {
	t.Parallel()
	svc := newTestAgentWorkspacesService(t, t.TempDir(), map[string]string{"claude": "true"})
	svc.hiveCLI = fakeHiveCLI{path: "/opt/bin/hive", ok: true}
	svc.hiveDataDir = func() string { return "/data/hive" }

	byID := map[string]mcpcatalog.Server{}
	for _, e := range svc.catalogue(t.Context()) {
		byID[e.ID] = e.Server
	}
	assert.Equal(t, mcpcatalog.Server{
		Transport: mcpcatalog.TransportStdio,
		Command:   "/opt/bin/hive",
		Args:      []string{"--data-dir", "/data/hive", "mcp", "serve", "--toolset", "actions,diagnostics,flows,inbox,sessions,workspaces"},
	}, byID["hive-desktop"])
	for _, id := range []string{"hive-drive", "hive-canvas"} {
		assert.Equal(t, mcpcatalog.TransportStdio, byID[id].Transport, id)
		assert.Empty(t, byID[id].URL, id)
		assert.Equal(t, []string{"--data-dir", "/data/hive", "mcp", "serve", "--toolset", "drive,layout"}, byID[id].Args, id)
	}
}

func TestCatalogueKeepsHTTPWithoutAUsableHive(t *testing.T) {
	t.Parallel()
	svc := newTestAgentWorkspacesService(t, t.TempDir(), map[string]string{"claude": "true"})
	svc.hiveCLI = fakeHiveCLI{}
	svc.hiveDataDir = func() string { return "/data/hive" }

	found := false
	for _, e := range svc.catalogue(t.Context()) {
		if e.ID == "hive-drive" {
			found = true
			assert.Equal(t, mcpcatalog.TransportHttp, e.Server.Transport)
			assert.Equal(t, testMCPBaseURL+"/mcp/drive", e.Server.URL)
		}
	}
	assert.True(t, found)
}

func TestCatalogueStdioStillNeedsTheHTTPServer(t *testing.T) {
	t.Parallel()
	svc := newTestAgentWorkspacesService(t, t.TempDir(), map[string]string{"claude": "true"})
	svc.hiveCLI = fakeHiveCLI{path: "/opt/bin/hive", ok: true}
	svc.hiveDataDir = func() string { return "/data/hive" }
	svc.mcpBase = mcpBaseFunc(func(context.Context) string { return "" })

	found := false
	for _, e := range svc.catalogue(t.Context()) {
		if e.ID == "hive-desktop" {
			found = true
			assert.Contains(t, e.Problem, "http.enabled")
			assert.NotEqual(t, mcpcatalog.TransportStdio, e.Server.Transport)
		}
	}
	assert.True(t, found)
}

func TestHiveCLIResolverMatchesVersionAndCachesTheProbe(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	bin := filepath.Join(dir, "hive")
	require.NoError(t, os.WriteFile(bin, []byte("x"), 0o755))

	execEnv := execenv.NewResolver(execenv.Options{
		Shell: "none",
		Probe: func(context.Context, string) (map[string]string, error) { return map[string]string{}, nil },
	})
	probes := 0
	var probeErr error
	r := newHiveCLIResolver(func(context.Context) string { return bin }, execEnv, "1.2.3", zerolog.Nop())
	r.probe = func(context.Context, string, []string) (string, error) {
		probes++
		return "hive version 1.2.3 (abc1234) 2026-01-01", probeErr
	}

	path, ok := r.Usable(t.Context())
	assert.True(t, ok)
	assert.Equal(t, bin, path)
	_, _ = r.Usable(t.Context())
	assert.Equal(t, 1, probes, "same path and mtime probes once")

	r.version = "9.9.9"
	r.cache = map[hiveProbeKey]bool{}
	_, ok = r.Usable(t.Context())
	assert.False(t, ok, "version mismatch")

	r.cache = map[hiveProbeKey]bool{}
	probeErr = errors.New("boom")
	_, ok = r.Usable(t.Context())
	assert.False(t, ok, "probe failure")

	r.configuredPath = func(context.Context) string { return filepath.Join(dir, "missing") }
	_, ok = r.Usable(t.Context())
	assert.False(t, ok, "missing binary")
}

func TestParseHiveVersion(t *testing.T) {
	t.Parallel()
	got, ok := parseHiveVersion("hive version v0.5.0 (abc) now\n")
	assert.True(t, ok)
	assert.Equal(t, "v0.5.0", got)
	_, ok = parseHiveVersion("garbage")
	assert.False(t, ok)
}

func TestDriveMCPCommand(t *testing.T) {
	t.Parallel()
	stdio := &stdioServe{hive: "/opt/bin/hive", dataDir: "/data"}
	tail := []string{"--", "/opt/bin/hive", "--data-dir", "/data", "mcp", "serve", "--toolset", "drive,layout"}

	cmd, args, err := driveMCPCommand("claude", "", stdio)
	require.NoError(t, err)
	assert.Equal(t, "claude", cmd)
	assert.Equal(t, append([]string{"mcp", "add", "--scope", "user", "hive-drive"}, tail...), args)

	cmd, args, err = driveMCPCommand("codex", "", stdio)
	require.NoError(t, err)
	assert.Equal(t, "codex", cmd)
	assert.Equal(t, append([]string{"mcp", "add", "hive-drive"}, tail...), args)

	_, args, _ = driveMCPCommand("claude", "http://x/mcp/drive", nil)
	assert.Equal(t, []string{"mcp", "add", "--scope", "user", "--transport", "http", "hive-drive", "http://x/mcp/drive"}, args)
	_, args, _ = driveMCPCommand("codex", "http://x/mcp/drive", nil)
	assert.Equal(t, []string{"mcp", "add", "hive-drive", "--url", "http://x/mcp/drive"}, args)

	_, _, err = driveMCPCommand("other", "", nil)
	assert.Equal(t, KindInvalid, KindOf(err))
}
