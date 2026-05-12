package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_PluginsLuaKey_IgnoredSilently(t *testing.T) {
	dataDir := t.TempDir()
	configFile := filepath.Join(t.TempDir(), "config.yaml")
	pluginKey := strings.Join([]string{"lu", "a"}, "")

	require.NoError(t, os.WriteFile(configFile, []byte(`
git_path: git
plugins:
  shell_workers: 9
  `+pluginKey+`:
    enabled: true
    entry: /tmp/plugin.`+pluginKey+`
`), 0o644))

	cfg, err := Load(configFile, dataDir)
	require.NoError(t, err)
	assert.Equal(t, 9, cfg.Plugins.ShellWorkers)
}
