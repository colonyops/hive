package configmigrate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func nopLogger() *zerolog.Logger {
	l := zerolog.Nop()
	return &l
}

// tempDirs lays out a source config file and a backup dir OUTSIDE it, mirroring
// <configDir>/<file> and <StateDir>/migration-backups/ living in separate trees.
func tempDirs(t *testing.T) (srcDir, backupDir string) {
	t.Helper()
	root := t.TempDir()
	srcDir = filepath.Join(root, "config")
	backupDir = filepath.Join(root, "state", "migration-backups")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	return srcDir, backupDir
}

func writeFixture(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func globBackups(t *testing.T, backupDir string) []string {
	t.Helper()
	entries, err := os.ReadDir(backupDir)
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestMigrateFile_BelowCurrentRewritesAndBackups(t *testing.T) {
	srcDir, backupDir := tempDirs(t)
	path := writeFixture(t, srcDir, "widget.yaml", "old_name: hello\n")

	data, changed, err := MigrateFile(renameStep(), path, backupDir, nopLogger())
	require.NoError(t, err)
	assert.True(t, changed)
	require.NotNil(t, data)

	doc := decodeDoc(t, data)
	assert.Equal(t, 2, doc["version"])
	assert.Equal(t, "hello", doc["new_name"])

	assert.True(t, strings.HasPrefix(string(data), migratedFileHeader), "rewritten body must carry the comments-not-retained header")

	onDisk, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, data, onDisk, "returned bytes must match what was written to disk")

	backups := globBackups(t, backupDir)
	require.Len(t, backups, 1)
	name := backups[0]
	assert.True(t, strings.HasPrefix(name, "widget-widget.v1."), "backup name must encode set name, source stem, and pre-migration version: %s", name)
	assert.True(t, strings.HasSuffix(name, ".yaml.bak"), "backup name must retain the source extension: %s", name)

	backupBytes, err := os.ReadFile(filepath.Join(backupDir, name))
	require.NoError(t, err)
	assert.Equal(t, "old_name: hello\n", string(backupBytes), "backup must hold the pre-migration bytes verbatim")
}

func TestMigrateFile_CurrentFixtureIsNoOp(t *testing.T) {
	srcDir, backupDir := tempDirs(t)
	original := "version: 2\nnew_name: foo\n"
	path := writeFixture(t, srcDir, "widget.yaml", original)

	data, changed, err := MigrateFile(renameStep(), path, backupDir, nopLogger())
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Equal(t, original, string(data))

	onDisk, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, original, string(onDisk), "current file must not be rewritten")

	assert.Empty(t, globBackups(t, backupDir), "current file must not create a backup")
}

func TestMigrateFile_MissingFileIsNoOp(t *testing.T) {
	srcDir, backupDir := tempDirs(t)
	path := filepath.Join(srcDir, "missing.yaml")

	data, changed, err := MigrateFile(renameStep(), path, backupDir, nopLogger())
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Nil(t, data)
	assert.Empty(t, globBackups(t, backupDir))
}

func TestMigrateFile_VersionTooNewWritesNothing(t *testing.T) {
	srcDir, backupDir := tempDirs(t)
	original := "version: 3\nold_name: hello\n"
	path := writeFixture(t, srcDir, "widget.yaml", original)

	data, changed, err := MigrateFile(renameStep(), path, backupDir, nopLogger())
	require.ErrorIs(t, err, ErrVersionTooNew)
	assert.False(t, changed)
	assert.Nil(t, data)

	onDisk, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, original, string(onDisk))

	assert.Empty(t, globBackups(t, backupDir))
}

func TestMigrateFile_StepErrorLeavesNoBackupOrRewrite(t *testing.T) {
	srcDir, backupDir := tempDirs(t)
	original := "k: v\n"
	path := writeFixture(t, srcDir, "broken.yaml", original)

	boom := errors.New("boom")
	s := Set{
		Name:                "broken",
		Baseline:            1,
		Current:             2,
		AllowMissingVersion: true,
		Migrations: []Migration{
			{To: 2, Migrate: func(doc map[string]any) error { return boom }},
		},
	}

	data, changed, err := MigrateFile(s, path, backupDir, nopLogger())
	require.ErrorIs(t, err, boom)
	assert.False(t, changed)
	assert.Nil(t, data)

	onDisk, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, original, string(onDisk), "step error must leave the source untouched")

	assert.Empty(t, globBackups(t, backupDir), "step error must leave no backup artifact")
}

func TestMigrateFile_MultipleDocumentsLeavesNoBackupOrRewrite(t *testing.T) {
	srcDir, backupDir := tempDirs(t)
	original := "old_name: hello\n---\nother: value\n"
	path := writeFixture(t, srcDir, "widget.yaml", original)

	data, changed, err := MigrateFile(renameStep(), path, backupDir, nopLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "multiple YAML documents")
	assert.False(t, changed)
	assert.Nil(t, data)

	onDisk, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, original, string(onDisk), "multi-document input must leave the source untouched")
	assert.Empty(t, globBackups(t, backupDir), "multi-document input must not create a backup")
}

func TestMigrateFile_BackupWriteFailureAbortsRewrite(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: chmod 0o500 does not block writes")
	}

	srcDir, backupDir := tempDirs(t)
	original := "old_name: hello\n"
	path := writeFixture(t, srcDir, "widget.yaml", original)

	require.NoError(t, os.MkdirAll(backupDir, 0o755))
	require.NoError(t, os.Chmod(backupDir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(backupDir, 0o755) })

	data, changed, err := MigrateFile(renameStep(), path, backupDir, nopLogger())
	require.Error(t, err)
	assert.False(t, changed)
	assert.Nil(t, data)

	onDisk, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, original, string(onDisk), "backup failure must abort the rewrite; source stays byte-unchanged")
}

// TestMigrateFile_RewriteFailureAfterBackupLeavesSourceIntact exercises the
// half-state the backup-first ordering is designed for: the backup lands, then
// the source rewrite fails. The atomic rename means the source stays byte-for-
// byte the pre-migration file, with the backup left as recoverable state.
func TestMigrateFile_RewriteFailureAfterBackupLeavesSourceIntact(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: chmod 0o500 does not block writes")
	}

	srcDir, backupDir := tempDirs(t)
	original := "old_name: hello\n"
	path := writeFixture(t, srcDir, "widget.yaml", original)

	// backupDir stays writable so the backup lands; srcDir goes read-only so
	// writeAtomic's temp-file creation for the source rewrite fails afterward.
	require.NoError(t, os.Chmod(srcDir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(srcDir, 0o755) })

	data, changed, err := MigrateFile(renameStep(), path, backupDir, nopLogger())
	require.Error(t, err)
	assert.False(t, changed)
	assert.Nil(t, data)

	onDisk, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, original, string(onDisk), "rewrite failure must leave the source byte-unchanged")

	assert.Len(t, globBackups(t, backupDir), 1, "the backup is written before the rewrite, so it survives the failure")
}

func TestMigrateFile_IsIdempotent(t *testing.T) {
	srcDir, backupDir := tempDirs(t)
	path := writeFixture(t, srcDir, "widget.yaml", "old_name: hello\n")

	_, changed, err := MigrateFile(renameStep(), path, backupDir, nopLogger())
	require.NoError(t, err)
	require.True(t, changed)
	require.Len(t, globBackups(t, backupDir), 1)

	afterFirst, err := os.ReadFile(path)
	require.NoError(t, err)

	_, changedAgain, err := MigrateFile(renameStep(), path, backupDir, nopLogger())
	require.NoError(t, err)
	assert.False(t, changedAgain, "re-running MigrateFile on an already-migrated file must be a no-op")
	assert.Len(t, globBackups(t, backupDir), 1, "second run must not create a second backup")

	afterSecond, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, afterFirst, afterSecond, "second run must not rewrite the file")
}

func TestBackupName_SelfDescribing(t *testing.T) {
	now := time.Date(2026, 7, 28, 15, 4, 5, 0, time.UTC)
	name := backupName(FlowSet, "/config/flows/triage.yaml", 1, now)
	assert.Equal(t, "flow-triage.v1.20260728T150405Z.yaml.bak", name)
}
