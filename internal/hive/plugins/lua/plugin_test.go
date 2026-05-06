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
      exit = true,
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
		"exit":    "true",
		"scope":   []string{"sessions"},
	}, map[string]any{
		"sh":      plugin.Commands()["LuaHello"].Sh,
		"help":    plugin.Commands()["LuaHello"].Help,
		"confirm": plugin.Commands()["LuaHello"].Confirm,
		"silent":  plugin.Commands()["LuaHello"].Silent,
		"exit":    plugin.Commands()["LuaHello"].Exit,
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

func TestPluginInitSupportsRequireRelativeToEntrypoint(t *testing.T) {
	root := t.TempDir()
	entry := filepath.Join(root, "plugins", "init.lua")
	module := filepath.Join(root, "plugins", "commands", "hello.lua")

	require.NoError(t, os.MkdirAll(filepath.Dir(module), 0o755))
	require.NoError(t, os.WriteFile(module, []byte(`
return {
  LuaHello = {
    sh = "echo hello from module",
    help = "loaded via require",
  },
}
`), 0o644))
	require.NoError(t, os.WriteFile(entry, []byte(`
local commands = require("commands.hello")

return function(hive)
  hive.commands(commands)
end
`), 0o644))

	plugin := NewConfigPlugin(entry)
	require.NoError(t, plugin.Init(context.Background()))
	t.Cleanup(func() {
		require.NoError(t, plugin.Close())
	})

	assert.Equal(t, "echo hello from module", plugin.Commands()["LuaHello"].Sh)
	assert.Equal(t, "loaded via require", plugin.Commands()["LuaHello"].Help)
}

func TestPluginInitRejectsMalformedCommandMaps(t *testing.T) {
	tests := []struct {
		name   string
		script string
		errMsg string
	}{
		{
			name: "non-string command name",
			script: `
return function(hive)
  hive.commands({
    [1] = { sh = "echo bad" },
  })
end
`,
			errMsg: "command names must be strings",
		},
		{
			name: "non-table command value",
			script: `
return function(hive)
  hive.commands({
    Broken = "echo bad",
  })
end
`,
			errMsg: `command "Broken" must be a table`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := filepath.Join(t.TempDir(), "init.lua")
			require.NoError(t, os.WriteFile(entry, []byte(tt.script), 0o644))

			plugin := NewConfigPlugin(entry)
			err := plugin.Init(context.Background())
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
}

func TestPluginInitRejectsUnsupportedCommandFields(t *testing.T) {
	tests := []string{"action", "windows", "options", "form"}

	for _, field := range tests {
		t.Run(field, func(t *testing.T) {
			entry := filepath.Join(t.TempDir(), "init.lua")
			require.NoError(t, os.WriteFile(entry, []byte(`
return function(hive)
  hive.commands({
    Broken = {
      sh = "echo hi",
      `+field+` = true,
    },
  })
end
`), 0o644))

			plugin := NewConfigPlugin(entry)
			err := plugin.Init(context.Background())
			require.Error(t, err)
			assert.Contains(t, err.Error(), `field "`+field+`" is not supported`)
		})
	}
}

func TestPluginInitRejectsDuplicateCommandNames(t *testing.T) {
	entry := filepath.Join(t.TempDir(), "init.lua")
	require.NoError(t, os.WriteFile(entry, []byte(`
return function(hive)
  hive.commands({
    LuaHello = { sh = "echo first" },
  })
  hive.commands({
    LuaHello = { sh = "echo second" },
  })
end
`), 0o644))

	plugin := NewConfigPlugin(entry)
	err := plugin.Init(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), `duplicate command "LuaHello"`)
}

func TestPluginInitRejectsInvalidTemplateSyntax(t *testing.T) {
	entry := filepath.Join(t.TempDir(), "init.lua")
	require.NoError(t, os.WriteFile(entry, []byte(`
return function(hive)
  hive.commands({
    Broken = {
      sh = "{{ .Name",
    },
  })
end
`), 0o644))

	plugin := NewConfigPlugin(entry)
	err := plugin.Init(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "template error in sh")
}

func TestPluginInitRejectsCallbackStyleValues(t *testing.T) {
	entry := filepath.Join(t.TempDir(), "init.lua")
	require.NoError(t, os.WriteFile(entry, []byte(`
return function(hive)
  hive.commands({
    Broken = {
      sh = function()
        return "echo hi"
      end,
    },
  })
end
`), 0o644))

	plugin := NewConfigPlugin(entry)
	err := plugin.Init(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), `field "sh" does not support callback values`)
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
