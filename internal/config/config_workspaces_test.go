package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_WorkspacesKey(t *testing.T) {
	dataDir := t.TempDir()
	configFile := filepath.Join(t.TempDir(), "config.yaml")

	require.NoError(t, os.WriteFile(configFile, []byte(`
git_path: git
workspaces:
  - ~/projects
  - ~/work
`), 0o644))

	cfg, err := Load(configFile, dataDir)
	require.NoError(t, err)
	assert.Equal(t, []string{"~/projects", "~/work"}, cfg.Workspaces)
}

func TestLoad_RepoDirsBackwardsCompat(t *testing.T) {
	dataDir := t.TempDir()
	configFile := filepath.Join(t.TempDir(), "config.yaml")

	require.NoError(t, os.WriteFile(configFile, []byte(`
git_path: git
repo_dirs:
  - ~/old-projects
`), 0o644))

	cfg, err := Load(configFile, dataDir)
	require.NoError(t, err)
	assert.Equal(t, []string{"~/old-projects"}, cfg.Workspaces, "old repo_dirs key should migrate to Workspaces")
	assert.Nil(t, cfg.RepoDirsCompat, "compat field should be cleared after migration")
}

func TestLoad_WorkspacesTakesPrecedenceOverRepoDirs(t *testing.T) {
	dataDir := t.TempDir()
	configFile := filepath.Join(t.TempDir(), "config.yaml")

	require.NoError(t, os.WriteFile(configFile, []byte(`
git_path: git
workspaces:
  - ~/new-projects
repo_dirs:
  - ~/old-projects
`), 0o644))

	cfg, err := Load(configFile, dataDir)
	require.NoError(t, err)
	assert.Equal(t, []string{"~/new-projects"}, cfg.Workspaces, "workspaces should take precedence over repo_dirs")
}

func TestLoad_NeitherWorkspacesNorRepoDirs(t *testing.T) {
	dataDir := t.TempDir()
	configFile := filepath.Join(t.TempDir(), "config.yaml")

	require.NoError(t, os.WriteFile(configFile, []byte(`
git_path: git
`), 0o644))

	cfg, err := Load(configFile, dataDir)
	require.NoError(t, err)
	assert.Empty(t, cfg.Workspaces)
}
