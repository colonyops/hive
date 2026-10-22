package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadReloadRejectsIncompleteAndInvalidFiles(t *testing.T) {
	t.Setenv("HIVE_DEFAULT_AGENT", "")
	current, err := Load("", t.TempDir())
	require.NoError(t, err)
	for _, tt := range []struct{ name, contents, want string }{
		{"missing", "", "read config"},
		{"empty", " ", "is empty"},
		{"yaml", "rules: [", "parse config"},
		{"template", "usercommands:\n  bad: 'echo {{'", "template"},
		{"restart", "database:\n  max_open_conns: 4", "restart required for database"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if tt.name != "missing" {
				require.NoError(t, os.WriteFile(path, []byte(tt.contents), 0o600))
			}
			cfg, err := LoadReload(path, current)
			require.ErrorContains(t, err, tt.want)
			assert.Nil(t, cfg)
			assert.Equal(t, 2, current.Database.MaxOpenConns)
		})
	}
}

func TestLoadReloadReplacesOverridesWithDefaults(t *testing.T) {
	t.Setenv("HIVE_DEFAULT_AGENT", "")
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("usercommands:\n  old: echo old\nviews:\n  global:\n    keybindings:\n      x: {cmd: old}\nrules:\n  - pattern: acme\n    max_recycled: 5\n"), 0o600))
	current, err := Load(path, t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, []byte("{}"), 0o600))
	cfg, err := LoadReload(path, current)
	require.NoError(t, err)
	assert.NotContains(t, cfg.UserCommands, "old")
	assert.NotContains(t, cfg.Views.Global.Keybindings, "x")
	assert.Equal(t, current.DataDir, cfg.DataDir)
	assert.Contains(t, current.UserCommands, "old")
}
