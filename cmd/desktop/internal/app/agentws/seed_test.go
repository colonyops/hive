package agentws

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/mcpcatalog"
)

func TestSeedDefaultsIfMissing(t *testing.T) {
	t.Parallel()

	t.Run("SeedsBothLibrariesAndTheSharedSkillsDir", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		require.NoError(t, SeedDefaultsIfMissing(root))

		lib, err := LoadLibrary(filepath.Join(root, libraryFileName))
		require.NoError(t, err)
		assert.Equal(t, 1, lib.Version)
		assert.Empty(t, lib.Servers)

		// The seeded hive package is what makes the seeded hive workspace
		// carry the shipped skills without enumerating them.
		skills, err := LoadSkillLibrary(SkillLibraryPath(root))
		require.NoError(t, err)
		assert.Equal(t, 1, skills.Version)
		require.Contains(t, skills.Packages, "hive")
		assert.Equal(t, []string{"hive-*"}, skills.Packages["hive"].Include)

		info, err := os.Stat(SharedSkillsDir(root))
		require.NoError(t, err)
		assert.True(t, info.IsDir())

		_, err = os.Stat(filepath.Join(root, ".shared", "prompts"))
		assert.True(t, os.IsNotExist(err), ".shared/prompts/ must not be created")
	})

	t.Run("NeverReplacesAnExistingFile", func(t *testing.T) {
		t.Parallel()

		for _, name := range []string{libraryFileName, skillLibraryFileName} {
			root := t.TempDir()
			require.NoError(t, os.MkdirAll(root, 0o700))
			path := filepath.Join(root, name)
			require.NoError(t, os.WriteFile(path, []byte("not even valid yaml: ["), 0o600))

			require.NoError(t, SeedDefaultsIfMissing(root))

			data, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, "not even valid yaml: [", string(data), "%s was replaced", name)
		}
	})
}

func TestSeedCreatesTheHiveWorkspaceOnRootCreation(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "workspaces")
	created, err := EnsureRoot(root)
	require.NoError(t, err)
	require.True(t, created)

	require.NoError(t, SeedHiveWorkspace(root))

	manifestPath := filepath.Join(root, "hive", manifestFileName)
	ws, err := LoadWorkspace(manifestPath)
	require.NoError(t, err)
	require.NoError(t, ws.Validate())
	assert.Equal(t, PresetCommand("claude-ask"), ws.Command)
	assert.False(t, CommandIsDangerous(ws.Command), "the seeded workspace must not ship a permission bypass")
	assert.Equal(t, []string{"hive"}, ws.Skills, "the seed enables the hive package, not individual skills")
	assert.Equal(t, []string{"hive-desktop", "hive-canvas"}, ws.MCPs, "the seed wires both app-hosted servers")
	for _, id := range ws.MCPs {
		_, ok := mcpcatalog.Lookup(id)
		assert.True(t, ok, "seeded server %q is one this build ships", id)
	}

	_, err = os.Stat(filepath.Join(root, "hive", "AGENTS.md"))
	require.NoError(t, err)
}

func TestSeedOrchestratorWorkspace(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, SeedOrchestratorWorkspace(root))

	ws, err := LoadWorkspace(filepath.Join(root, OrchestratorWorkspaceDir, manifestFileName))
	require.NoError(t, err)
	require.NoError(t, ws.Validate())
	assert.Equal(t, PresetCommand("claude-ask"), ws.Command)
	assert.False(t, CommandIsDangerous(ws.Command), "the seeded orchestrator must not ship a permission bypass")
	assert.Equal(t, []string{mcpcatalog.Orchestrator, "hive-desktop"}, ws.MCPs)
	for _, id := range ws.MCPs {
		_, ok := mcpcatalog.Lookup(id)
		assert.True(t, ok, "seeded MCP %q is not in the shipped catalogue", id)
	}

	agents, err := os.ReadFile(filepath.Join(root, OrchestratorWorkspaceDir, "AGENTS.md"))
	require.NoError(t, err)
	for _, tool := range []string{"start_session", "send_prompt", "send_keys", "wait_for_session", "wait_for_message"} {
		assert.Contains(t, string(agents), tool, "AGENTS.md should teach %s", tool)
	}
}

func TestSeedShippedWorkspaces(t *testing.T) {
	t.Parallel()

	exists := func(t *testing.T, root, dir string) bool {
		t.Helper()
		_, err := os.Stat(filepath.Join(root, dir))
		return err == nil
	}

	t.Run("a new root gets every shipped workspace", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		seeded, err := SeedShippedWorkspaces(root, true)
		require.NoError(t, err)
		assert.Equal(t, []string{HiveWorkspaceDir, OrchestratorWorkspaceDir}, seeded)
	})

	t.Run("an existing root gets only what was added since", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		seeded, err := SeedShippedWorkspaces(root, false)
		require.NoError(t, err)
		assert.Equal(t, []string{OrchestratorWorkspaceDir}, seeded)
		assert.False(t, exists(t, root, HiveWorkspaceDir), "a pre-marker root already had its chance at hive")
	})

	t.Run("a deleted workspace stays deleted", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		_, err := SeedShippedWorkspaces(root, true)
		require.NoError(t, err)
		require.NoError(t, os.RemoveAll(filepath.Join(root, OrchestratorWorkspaceDir)))

		seeded, err := SeedShippedWorkspaces(root, false)
		require.NoError(t, err)
		assert.Empty(t, seeded)
		assert.False(t, exists(t, root, OrchestratorWorkspaceDir))
	})

	t.Run("a user directory with the same name is never written to", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		mine := filepath.Join(root, OrchestratorWorkspaceDir)
		require.NoError(t, os.MkdirAll(mine, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(mine, "AGENTS.md"), []byte("my own"), 0o600))

		seeded, err := SeedShippedWorkspaces(root, false)
		require.NoError(t, err)
		assert.Empty(t, seeded)
		data, err := os.ReadFile(filepath.Join(mine, "AGENTS.md"))
		require.NoError(t, err)
		assert.Equal(t, "my own", string(data))
		_, err = os.Stat(filepath.Join(mine, manifestFileName))
		assert.True(t, os.IsNotExist(err))
	})
	t.Run("a failed seed leaves nothing behind and is retried", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		failing := func(root string) error {
			require.NoError(t, os.MkdirAll(filepath.Join(root, OrchestratorWorkspaceDir), 0o700))
			return errors.New("disk full")
		}
		_, err := seedShipped(root, true, []shippedWorkspace{
			{dir: HiveWorkspaceDir, seed: SeedHiveWorkspace},
			{dir: OrchestratorWorkspaceDir, seed: failing},
		})
		require.ErrorContains(t, err, "disk full")
		assert.True(t, exists(t, root, HiveWorkspaceDir))
		assert.False(t, exists(t, root, OrchestratorWorkspaceDir))
		entries, err := os.ReadDir(root)
		require.NoError(t, err)
		for _, e := range entries {
			assert.NotContains(t, e.Name(), ".seeding-", "no staging directory is left behind")
		}

		seeded, err := SeedShippedWorkspaces(root, false)
		require.NoError(t, err)
		assert.Equal(t, []string{OrchestratorWorkspaceDir}, seeded, "only the failed workspace is retried")
	})
}
