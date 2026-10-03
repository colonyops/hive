package agentws

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/configmigrate"
)

func nopLogger() *zerolog.Logger {
	l := zerolog.Nop()
	return &l
}

// TestMigrateRoot deliberately does not run in parallel with itself or other
// tests: it mutates the package-level configmigrate.AgentWorkspaceSet for the
// duration of the sweep, mirroring flow/migrate_test.go's TestMigrateDir_*.
func TestMigrateRoot(t *testing.T) {
	t.Run("MigratesWorkspacesAndIsolatesPerWorkspaceErrors", func(t *testing.T) {
		original := configmigrate.AgentWorkspaceSet
		configmigrate.AgentWorkspaceSet = configmigrate.Set{
			Name:     "agent-workspace",
			Baseline: 1,
			Current:  2,
			Migrations: []configmigrate.Migration{
				{To: 2, Migrate: func(doc map[string]any) error { return nil }},
			},
		}
		t.Cleanup(func() { configmigrate.AgentWorkspaceSet = original })

		root := t.TempDir()
		backupDir := filepath.Join(t.TempDir(), "migration-backups")

		goodDir := filepath.Join(root, "good")
		require.NoError(t, os.MkdirAll(goodDir, 0o700))
		goodPath := filepath.Join(goodDir, manifestFileName)
		require.NoError(t, os.WriteFile(goodPath, []byte("version: 1\nname: Good\nagent: claude\ncommand: claude\n"), 0o600))

		newerDir := filepath.Join(root, "newer")
		require.NoError(t, os.MkdirAll(newerDir, 0o700))
		newerPath := filepath.Join(newerDir, manifestFileName)
		const newerContent = "version: 3\nname: Newer\nagent: claude\ncommand: claude\n"
		require.NoError(t, os.WriteFile(newerPath, []byte(newerContent), 0o600))

		// A directory with no manifest at all must not abort or error the
		// sweep either.
		require.NoError(t, os.MkdirAll(filepath.Join(root, "empty"), 0o700))

		require.NoError(t, MigrateRoot(root, backupDir, nopLogger()))

		migratedGood, err := os.ReadFile(goodPath)
		require.NoError(t, err)
		assert.Contains(t, string(migratedGood), "version: 2", "a good workspace must still migrate past a failing neighbour")

		newerAfter, err := os.ReadFile(newerPath)
		require.NoError(t, err)
		assert.Equal(t, newerContent, string(newerAfter), "a version-too-new workspace must be left byte-unchanged")
	})

	t.Run("MigratesTheLibraryToo", func(t *testing.T) {
		original := configmigrate.MCPLibrarySet
		configmigrate.MCPLibrarySet = configmigrate.Set{
			Name:     "mcps",
			Baseline: 1,
			Current:  2,
			Migrations: []configmigrate.Migration{
				{To: 2, Migrate: func(doc map[string]any) error { return nil }},
			},
		}
		t.Cleanup(func() { configmigrate.MCPLibrarySet = original })

		root := t.TempDir()
		backupDir := filepath.Join(t.TempDir(), "migration-backups")
		libPath := filepath.Join(root, libraryFileName)
		require.NoError(t, os.MkdirAll(root, 0o700))
		require.NoError(t, os.WriteFile(libPath, []byte("version: 1\nservers: {}\n"), 0o600))

		require.NoError(t, MigrateRoot(root, backupDir, nopLogger()))

		migrated, err := os.ReadFile(libPath)
		require.NoError(t, err)
		assert.Contains(t, string(migrated), "version: 2")
	})

	t.Run("MissingRootIsNoOp", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "does-not-exist")
		backupDir := filepath.Join(t.TempDir(), "migration-backups")
		require.NoError(t, MigrateRoot(root, backupDir, nopLogger()))
	})
}
