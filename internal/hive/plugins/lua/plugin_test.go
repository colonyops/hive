package lua

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginAvailable(t *testing.T) {
	entry := filepath.Join(t.TempDir(), "init.lua")
	require.NoError(t, os.WriteFile(entry, []byte("return function() end\n"), 0o644))

	plugin := NewConfigPlugin(entry)
	assert.True(t, plugin.Available())

	missing := NewConfigPlugin(filepath.Join(t.TempDir(), "missing.lua"))
	assert.False(t, missing.Available())
}

func TestPluginInitLoadsCommandsAndHasNoStatusProvider(t *testing.T) {
	entry := filepath.Join(t.TempDir(), "init.lua")
	require.NoError(t, os.WriteFile(entry, []byte(`
return function(hive)
  hive.commands({
    LuaHello = {
      sh = "echo hello",
      help = "say hello",
      scope = {"sessions"},
      silent = true,
      confirm = "Run it?",
    },
  })
end
`), 0o644))

	plugin := NewConfigPlugin(entry)
	require.NoError(t, plugin.Init(context.Background()))
	t.Cleanup(func() {
		require.NoError(t, plugin.Close())
	})

	assert.Equal(t, "lua", plugin.Name())
	assert.Nil(t, plugin.StatusProvider())
	assert.Equal(t, map[string]any{
		"sh":      "echo hello",
		"help":    "say hello",
		"confirm": "Run it?",
		"silent":  true,
		"scope":   []string{"sessions"},
	}, map[string]any{
		"sh":      plugin.Commands()["LuaHello"].Sh,
		"help":    plugin.Commands()["LuaHello"].Help,
		"confirm": plugin.Commands()["LuaHello"].Confirm,
		"silent":  plugin.Commands()["LuaHello"].Silent,
		"scope":   plugin.Commands()["LuaHello"].Scope,
	})
}

func TestPluginInitPassesHiveMetadata(t *testing.T) {
	entry := filepath.Join(t.TempDir(), "init.lua")
	require.NoError(t, os.WriteFile(entry, []byte(`
return function(hive)
  hive.commands({
    LuaInit = {
      sh = "echo init",
      help = hive.plugin.entry,
    },
  })
end
`), 0o644))

	plugin := NewConfigPlugin(entry)
	require.NoError(t, plugin.Init(context.Background()))
	t.Cleanup(func() {
		require.NoError(t, plugin.Close())
	})

	assert.Equal(t, entry, plugin.Commands()["LuaInit"].Help)
}

func TestPluginInitRejectsInvalidCommand(t *testing.T) {
	entry := filepath.Join(t.TempDir(), "init.lua")
	require.NoError(t, os.WriteFile(entry, []byte(`
return function(hive)
  hive.commands({
    Broken = {
      sh = 42,
    },
  })
end
`), 0o644))

	plugin := NewConfigPlugin(entry)
	err := plugin.Init(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), `command "Broken"`)
}

func TestPluginInitRejectsNonFunctionEntrypoint(t *testing.T) {
	entry := filepath.Join(t.TempDir(), "init.lua")
	require.NoError(t, os.WriteFile(entry, []byte("return {}\n"), 0o644))

	plugin := NewConfigPlugin(entry)
	err := plugin.Init(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must return a function")
}

func NewConfigPlugin(entry string) *Plugin {
	return New(config.LuaPluginConfig{Entry: entry})
}
