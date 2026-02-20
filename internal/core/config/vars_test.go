package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadVarsFiles_Single(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, writeTestFile(filepath.Join(dir, "vars.yaml"), "editor: nvim\norg: colonyops\n"))

	got, err := loadVarsFiles(dir, []string{"vars.yaml"})
	require.NoError(t, err)
	assert.Equal(t, "nvim", got["editor"])
	assert.Equal(t, "colonyops", got["org"])
}

func TestLoadVarsFiles_Multiple_MergeOrder(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, writeTestFile(filepath.Join(dir, "base.yaml"), "editor: vim\norg: old\n"))
	require.NoError(t, writeTestFile(filepath.Join(dir, "override.yaml"), "editor: nvim\n"))

	got, err := loadVarsFiles(dir, []string{"base.yaml", "override.yaml"})
	require.NoError(t, err)
	assert.Equal(t, "nvim", got["editor"])
	assert.Equal(t, "old", got["org"])
}

func TestLoadVarsFiles_NestedMerge(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, writeTestFile(filepath.Join(dir, "a.yaml"), "github:\n  org: colonyops\n  team: platform\n"))
	require.NoError(t, writeTestFile(filepath.Join(dir, "b.yaml"), "github:\n  team: infra\n"))

	got, err := loadVarsFiles(dir, []string{"a.yaml", "b.yaml"})
	require.NoError(t, err)

	github := got["github"].(map[string]any)
	assert.Equal(t, "colonyops", github["org"])
	assert.Equal(t, "infra", github["team"])
}

func TestLoadVarsFiles_AbsolutePath(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "vars.yaml")
	require.NoError(t, writeTestFile(file, "editor: nvim\n"))

	got, err := loadVarsFiles("ignored", []string{file})
	require.NoError(t, err)
	assert.Equal(t, "nvim", got["editor"])
}

func TestLoadVarsFiles_NotFound(t *testing.T) {
	_, err := loadVarsFiles(t.TempDir(), []string{"missing.yaml"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read vars file")
}

func TestLoadVarsFiles_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, writeTestFile(filepath.Join(dir, "bad.yaml"), "editor: [\n"))

	_, err := loadVarsFiles(dir, []string{"bad.yaml"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse vars file")
}

func TestMergeMaps_InlineOverridesFiles(t *testing.T) {
	fileVars := map[string]any{
		"editor": "vim",
		"github": map[string]any{
			"org":  "colonyops",
			"team": "platform",
		},
	}
	inlineVars := map[string]any{
		"editor": "nvim",
		"github": map[string]any{
			"team": "infra",
		},
	}

	mergeMaps(fileVars, inlineVars)

	assert.Equal(t, "nvim", fileVars["editor"])
	github := fileVars["github"].(map[string]any)
	assert.Equal(t, "colonyops", github["org"])
	assert.Equal(t, "infra", github["team"])
}

func writeTestFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
