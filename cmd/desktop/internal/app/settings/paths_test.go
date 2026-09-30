package settings

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlowsDirFollowsTheConfigRoot(t *testing.T) {
	root := t.TempDir()
	t.Setenv(EnvConfigDir, filepath.Join(root, "config"))

	assert.Equal(t, filepath.Join(root, "config", "flows"), FlowsDir())
}

// The onboarding mock mode stands in for a fresh install, and first run is
// gated on having no workspaces. Reading a config root that already holds
// flows would boot it straight to the feed — the one thing the mode exists to
// avoid.
func TestFlowsDirIsScratchInTheOnboardingMockMode(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "config", "flows")
	require.NoError(t, os.MkdirAll(real, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(real, "triage.yaml"), []byte("nodes: []\n"), 0o600))

	t.Setenv(EnvConfigDir, filepath.Join(root, "config"))
	t.Setenv(EnvMockMode, MockOnboarding)

	dir := FlowsDir()
	require.NotEqual(t, real, dir)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "the onboarding mock mode starts with no workspaces")

	// Resolved once per process: the flow store, the watcher, and the prompts
	// env all call this and must agree on one directory.
	assert.Equal(t, dir, FlowsDir())
}

// An explicit override is how the e2e harness — and anyone wanting a specific
// fixture set — opts out of the scratch directory.
func TestResolvePathsUsesResolvedYAMLMockMode(t *testing.T) {
	root := t.TempDir()
	t.Setenv(EnvConfigDir, filepath.Join(root, "config"))
	unsetEnv(t, EnvMockMode)

	paths := ResolvePaths(Bootstrap{}, ResolveOptions{MockMode: MockOnboarding})
	require.NotEqual(t, filepath.Join(root, "config", "flows"), paths.FlowsDir)
}

func TestFlowsDirOverrideWinsOverTheOnboardingMockMode(t *testing.T) {
	t.Setenv(EnvMockMode, MockOnboarding)
	t.Setenv(EnvFlowsDir, "/tmp/explicit-flows")

	assert.Equal(t, "/tmp/explicit-flows", FlowsDir())
}

func TestHiveDataDirDefaultsToDataDir(t *testing.T) {
	data := t.TempDir()
	t.Setenv(EnvDataDir, data)
	unsetEnv(t, EnvHiveDataDir)

	assert.Equal(t, data, ResolvePaths(Bootstrap{}, ResolveOptions{}).HiveDataDir)
}

func TestHiveDataDirOverrideKeepsDesktopStateIsolated(t *testing.T) {
	data := t.TempDir()
	hive := t.TempDir()
	t.Setenv(EnvDataDir, data)
	t.Setenv(EnvHiveDataDir, hive)

	paths := ResolvePaths(Bootstrap{}, ResolveOptions{})
	assert.Equal(t, hive, paths.HiveDataDir, "hive.db follows the override")
	assert.Equal(t, data, paths.DataDir)
	assert.Equal(t, filepath.Join(data, "desktop"), paths.StateDir, "desktop state stays under the isolated data dir")
}

func TestAgentWorkspacesRootResolution(t *testing.T) {
	t.Run("DefaultsUnderConfigDir", func(t *testing.T) {
		unsetEnv(t, EnvAgentWorkspacesDir)
		config := filepath.Join(t.TempDir(), "config")

		paths := ResolvePaths(Bootstrap{ConfigDir: config}, ResolveOptions{})
		assert.Equal(t, filepath.Join(config, "workspaces"), paths.AgentWorkspacesDir)
	})

	// A non-empty ResolveOptions field is the settings-driven re-run's way of
	// carrying agent_workspaces.dir past the immutable Paths snapshot.
	t.Run("ResolveOptionsFieldWins", func(t *testing.T) {
		unsetEnv(t, EnvAgentWorkspacesDir)
		root := t.TempDir()
		custom := filepath.Join(root, "custom-workspaces")

		paths := ResolvePaths(Bootstrap{ConfigDir: filepath.Join(root, "config")}, ResolveOptions{AgentWorkspacesDir: custom})
		assert.Equal(t, custom, paths.AgentWorkspacesDir)
	})

	t.Run("TildeExpands", func(t *testing.T) {
		unsetEnv(t, EnvAgentWorkspacesDir)
		home, err := os.UserHomeDir()
		require.NoError(t, err)

		paths := ResolvePaths(Bootstrap{}, ResolveOptions{AgentWorkspacesDir: "~/agent-workspaces"})
		assert.Equal(t, filepath.Join(home, "agent-workspaces"), paths.AgentWorkspacesDir)
	})

	// The env var exists so an ops override can take effect without editing
	// settings.yaml, which only works if it wins over a persisted value.
	t.Run("EnvWinsOverYAML", func(t *testing.T) {
		root := t.TempDir()
		envDir := filepath.Join(root, "env-workspaces")
		t.Setenv(EnvAgentWorkspacesDir, envDir)

		paths := ResolvePaths(Bootstrap{}, ResolveOptions{AgentWorkspacesDir: filepath.Join(root, "yaml-workspaces")})
		assert.Equal(t, envDir, paths.AgentWorkspacesDir)
	})
}
