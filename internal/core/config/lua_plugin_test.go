package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hay-kot/criterio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLuaPluginEntry_DefaultResolution(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()

	wantEntry := filepath.Join(home, ".config", "hive", "plugins", "init.lua")
	assert.Equal(t, wantEntry, cfg.LuaPluginEntry())
	assert.Equal(t, filepath.Dir(wantEntry), cfg.LuaPluginModuleRoot())
}

func TestLoad_LuaPluginEntryOverride(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dataDir := t.TempDir()
	configFile := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(configFile, []byte(`
plugins:
  lua:
    entry: ~/lua/plugins/custom.lua
`), 0o644))

	cfg, err := Load(configFile, dataDir)
	require.NoError(t, err)

	wantEntry := filepath.Join(home, "lua", "plugins", "custom.lua")
	assert.Equal(t, wantEntry, cfg.LuaPluginEntry())
	assert.Equal(t, filepath.Join(home, "lua", "plugins"), cfg.LuaPluginModuleRoot())
}

func TestValidateDeep_LuaPluginEntry(t *testing.T) {
	t.Run("default entry missing is a no-op", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		cfg := validConfig(t)

		assert.Equal(t, filepath.Join(home, ".config", "hive", "plugins", "init.lua"), cfg.LuaPluginEntry())
		assert.NoError(t, cfg.ValidateDeep(""))
	})

	t.Run("default entry resolves when file exists", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		entry := filepath.Join(home, ".config", "hive", "plugins", "init.lua")
		require.NoError(t, os.MkdirAll(filepath.Dir(entry), 0o755))
		require.NoError(t, os.WriteFile(entry, []byte("return function() end\n"), 0o644))

		cfg := validConfig(t)

		assert.Equal(t, filepath.Dir(entry), cfg.LuaPluginModuleRoot())
		assert.NoError(t, cfg.ValidateDeep(""))
	})

	t.Run("explicit override derives module root", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		entry := filepath.Join(home, "lua", "plugins", "custom.lua")
		require.NoError(t, os.MkdirAll(filepath.Dir(entry), 0o755))
		require.NoError(t, os.WriteFile(entry, []byte("return function() end\n"), 0o644))

		cfg := validConfig(t)
		cfg.Plugins.Lua = LuaPluginConfig{
			Entry:         "~/lua/plugins/custom.lua",
			entryExplicit: true,
		}

		assert.Equal(t, entry, cfg.LuaPluginEntry())
		assert.Equal(t, filepath.Dir(entry), cfg.LuaPluginModuleRoot())
		assert.NoError(t, cfg.ValidateDeep(""))
	})

	t.Run("missing file errors only when entry is explicit", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		cfg := validConfig(t)
		cfg.Plugins.Lua = LuaPluginConfig{
			Entry:         "~/.config/hive/plugins/init.lua",
			entryExplicit: true,
		}

		err := cfg.ValidateDeep("")
		var fieldErrs criterio.FieldErrors
		require.ErrorAs(t, err, &fieldErrs)
		require.Len(t, fieldErrs, 1)
		assert.Equal(t, "plugins.lua.entry", fieldErrs[0].Field)
		assert.Contains(t, fieldErrs[0].Err.Error(), "file does not exist")
	})
}
