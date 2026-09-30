package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrationOrderScript(t *testing.T) {
	repoRoot := testCommandOutput(t, "", "git", "rev-parse", "--show-toplevel")
	script, err := os.ReadFile(filepath.Join(repoRoot, "cmd", "desktop", "scripts", "check-migration-order.sh"))
	require.NoError(t, err)

	dir := t.TempDir()
	migrationDir := filepath.Join(dir, "cmd", "desktop", "internal", "app", "data", "queries", "migrations")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "scripts"), 0o755))
	require.NoError(t, os.MkdirAll(migrationDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "scripts", "check-migration-order.sh"), script, 0o755))
	first := filepath.Join(migrationDir, "0001_baseline.up.sql")
	require.NoError(t, os.WriteFile(first, []byte("CREATE TABLE baseline (id INTEGER);\n"), 0o644))

	testCommandOutput(t, dir, "git", "init", "--quiet")
	testCommandOutput(t, dir, "git", "add", ".")
	testCommandOutput(t, dir, "git", "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--quiet", "-m", "baseline")
	testCommandOutput(t, dir, "git", "tag", "desktop-v0.1.0-dev.1")

	output, err := testMigrationOrder(t, dir)
	require.NoError(t, err, output)

	require.NoError(t, os.WriteFile(first, []byte("CREATE TABLE changed (id INTEGER);\n"), 0o644))
	output, err = testMigrationOrder(t, dir)
	require.Error(t, err)
	assert.Contains(t, output, "released migration was modified")

	require.NoError(t, os.WriteFile(first, []byte("CREATE TABLE baseline (id INTEGER);\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(migrationDir, "0003_gap.up.sql"), []byte("SELECT 1;\n"), 0o644))
	output, err = testMigrationOrder(t, dir)
	require.Error(t, err)
	assert.Contains(t, output, "expected 0002")
}

func testMigrationOrder(t *testing.T, dir string) (string, error) {
	t.Helper()
	cmd := exec.Command(filepath.Join(dir, "scripts", "check-migration-order.sh"))
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func testCommandOutput(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))
	return strings.TrimSpace(string(output))
}
