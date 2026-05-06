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
	assert.Equal(t, wantEntry, cfg.Plugins.Lua.ResolvedEntry())
	assert.Equal(t, filepath.Dir(wantEntry), cfg.Plugins.Lua.ModuleRoot())
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
	assert.Equal(t, wantEntry, cfg.Plugins.Lua.ResolvedEntry())
	assert.Equal(t, filepath.Join(home, "lua", "plugins"), cfg.Plugins.Lua.ModuleRoot())
}

func TestValidateDeep_LuaPluginEntry(t *testing.T) {
	tests := []struct {
		name      string
		entry     string
		prepare   func(t *testing.T, home string)
		wantError string
		wantRoot  string
	}{
		{
			name:     "unset entry is a no-op when file is missing",
			wantRoot: filepath.Join(".config", "hive", "plugins"),
		},
		{
			name: "unset entry resolves when default file exists",
			prepare: func(t *testing.T, home string) {
				entry := filepath.Join(home, ".config", "hive", "plugins", "init.lua")
				require.NoError(t, os.MkdirAll(filepath.Dir(entry), 0o755))
				require.NoError(t, os.WriteFile(entry, []byte("return function() end\n"), 0o644))
			},
			wantRoot: filepath.Join(".config", "hive", "plugins"),
		},
		{
			name:  "explicit override derives module root",
			entry: "~/lua/plugins/custom.lua",
			prepare: func(t *testing.T, home string) {
				entry := filepath.Join(home, "lua", "plugins", "custom.lua")
				require.NoError(t, os.MkdirAll(filepath.Dir(entry), 0o755))
				require.NoError(t, os.WriteFile(entry, []byte("return function() end\n"), 0o644))
			},
			wantRoot: filepath.Join("lua", "plugins"),
		},
		{
			name:      "missing override errors",
			entry:     "~/lua/plugins/custom.lua",
			wantError: "file does not exist",
		},
		{
			name:  "override pointing at a directory errors",
			entry: "~/lua/plugins",
			prepare: func(t *testing.T, home string) {
				require.NoError(t, os.MkdirAll(filepath.Join(home, "lua", "plugins"), 0o755))
			},
			wantError: "is a directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)

			if tt.prepare != nil {
				tt.prepare(t, home)
			}

			cfg := validConfig(t)
			cfg.Plugins.Lua.Entry = tt.entry

			err := cfg.ValidateDeep("")
			if tt.wantError != "" {
				var fieldErrs criterio.FieldErrors
				require.ErrorAs(t, err, &fieldErrs)
				require.Len(t, fieldErrs, 1)
				assert.Equal(t, "plugins.lua.entry", fieldErrs[0].Field)
				assert.Contains(t, fieldErrs[0].Err.Error(), tt.wantError)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, filepath.Join(home, tt.wantRoot), cfg.Plugins.Lua.ModuleRoot())
		})
	}
}

func TestValidate_LuaPluginEntryRejectsRelativePath(t *testing.T) {
	cfg := validConfig(t)
	cfg.Plugins.Lua.Entry = "plugins/init.lua"

	err := cfg.Validate()
	var fieldErrs criterio.FieldErrors
	require.ErrorAs(t, err, &fieldErrs)
	require.Len(t, fieldErrs, 1)
	assert.Equal(t, "plugins.lua.entry", fieldErrs[0].Field)
	assert.Contains(t, fieldErrs[0].Err.Error(), "must be an absolute path")
}
