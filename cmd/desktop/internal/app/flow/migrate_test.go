package flow

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

func TestMigrateDir_RewritesFlowDefinitionsAndSkipsUISiblings(t *testing.T) {
	original := configmigrate.FlowSet
	configmigrate.FlowSet = configmigrate.Set{
		Name:     "flow",
		Baseline: 1,
		Current:  2,
		Migrations: []configmigrate.Migration{
			{To: 2, Migrate: func(doc map[string]any) error { return nil }},
		},
	}
	t.Cleanup(func() { configmigrate.FlowSet = original })

	dir := t.TempDir()
	backupDir := filepath.Join(t.TempDir(), "migration-backups")

	flowPath := writeFlow(t, dir, "triage.yaml", minimalValidFlowYAML())
	uiPath := writeFlow(t, dir, "triage.ui.yaml", "nodes:\n  src: { x: 10, y: 20 }\n")
	uiBefore, err := os.ReadFile(uiPath)
	require.NoError(t, err)

	require.NoError(t, MigrateDir(dir, backupDir, nopLogger()))

	migrated, err := os.ReadFile(flowPath)
	require.NoError(t, err)
	assert.Contains(t, string(migrated), "version: 2")

	entries, err := os.ReadDir(backupDir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Contains(t, entries[0].Name(), ".v1.")

	uiAfter, err := os.ReadFile(uiPath)
	require.NoError(t, err)
	assert.Equal(t, uiBefore, uiAfter, "a .ui.yaml sibling must be left untouched")
}

// TestMigrateDir_IsolatesPerFileErrors proves the promised per-file isolation:
// a flow MigrateFile cannot migrate (version newer than this build) is logged
// and skipped, and its neighbours still migrate. main.go's non-fatal flow
// sweep relies on this.
func TestMigrateDir_IsolatesPerFileErrors(t *testing.T) {
	original := configmigrate.FlowSet
	configmigrate.FlowSet = configmigrate.Set{
		Name:     "flow",
		Baseline: 1,
		Current:  2,
		Migrations: []configmigrate.Migration{
			{To: 2, Migrate: func(doc map[string]any) error { return nil }},
		},
	}
	t.Cleanup(func() { configmigrate.FlowSet = original })

	dir := t.TempDir()
	backupDir := filepath.Join(t.TempDir(), "migration-backups")

	goodPath := writeFlow(t, dir, "good.yaml", minimalValidFlowYAML())
	const newer = "version: 3\nnodes: []\n"
	newerPath := writeFlow(t, dir, "newer.yaml", newer)

	require.NoError(t, MigrateDir(dir, backupDir, nopLogger()))

	migratedGood, err := os.ReadFile(goodPath)
	require.NoError(t, err)
	assert.Contains(t, string(migratedGood), "version: 2", "a good flow must still migrate past a failing neighbour")

	newerAfter, err := os.ReadFile(newerPath)
	require.NoError(t, err)
	assert.Equal(t, newer, string(newerAfter), "a version-too-new flow must be left byte-unchanged")
}

func TestMigrateDir_MissingDirIsNoOp(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")
	backupDir := filepath.Join(t.TempDir(), "migration-backups")
	assert.NoError(t, MigrateDir(dir, backupDir, nopLogger()))
}
