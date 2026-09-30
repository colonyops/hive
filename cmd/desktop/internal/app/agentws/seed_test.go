package agentws

import (
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
