package lua

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommandsModuleSuccessCases(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		entry   string
		asserts func(t *testing.T, entry string, plugin *Plugin)
	}{
		{
			name: "loads commands and exposes metadata",
			files: map[string]string{
				"init.lua": `
return function(hive)
  hive.commands({
    LuaHello = {
      sh = "echo hello",
      help = hive.plugin.entry,
      scope = {"sessions"},
      silent = true,
      confirm = "Run it?",
      exit = true,
    },
  })
end
`,
			},
			entry: "init.lua",
			asserts: func(t *testing.T, entry string, plugin *Plugin) {
				t.Helper()
				assert.Equal(t, "lua", plugin.Name())
				assert.Nil(t, plugin.StatusProvider())
				assert.Equal(t, map[string]any{
					"sh":      "echo hello",
					"help":    entry,
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
			},
		},
		{
			name: "exit false coerces to string",
			files: map[string]string{
				"init.lua": `
return function(hive)
  hive.commands({
    LuaHello = {
      sh = "echo hello",
      exit = false,
    },
  })
end
`,
			},
			entry: "init.lua",
			asserts: func(t *testing.T, _ string, plugin *Plugin) {
				t.Helper()
				assert.Equal(t, "false", plugin.Commands()["LuaHello"].Exit)
			},
		},
		{
			name: "supports require relative to entrypoint",
			files: map[string]string{
				"plugins/init.lua": `
local commands = require("commands.hello")

return function(hive)
  hive.commands(commands)
end
`,
				"plugins/commands/hello.lua": `
return {
  LuaHello = {
    sh = "echo hello from module",
    help = "loaded via require",
  },
}
`,
			},
			entry: "plugins/init.lua",
			asserts: func(t *testing.T, _ string, plugin *Plugin) {
				t.Helper()
				assert.Equal(t, "echo hello from module", plugin.Commands()["LuaHello"].Sh)
				assert.Equal(t, "loaded via require", plugin.Commands()["LuaHello"].Help)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for name, contents := range tt.files {
				fullPath := filepath.Join(root, name)
				require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o755))
				require.NoError(t, os.WriteFile(fullPath, []byte(contents), 0o644))
			}

			entry := filepath.Join(root, tt.entry)
			plugin := NewConfigPlugin(entry)
			require.NoError(t, plugin.Init(context.Background()))
			t.Cleanup(func() {
				require.NoError(t, plugin.Close())
			})

			tt.asserts(t, entry, plugin)
		})
	}
}

func TestCommandsModuleRejectsInvalidInputs(t *testing.T) {
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
		{
			name: "invalid command field type",
			script: `
return function(hive)
  hive.commands({
    Broken = {
      sh = 42,
    },
  })
end
`,
			errMsg: `command "Broken"`,
		},
		{
			name: "invalid template syntax",
			script: `
return function(hive)
  hive.commands({
    Broken = {
      sh = "{{ .Name",
    },
  })
end
`,
			errMsg: "template error in sh",
		},
		{
			name: "callback style values",
			script: `
return function(hive)
  hive.commands({
    Broken = {
      sh = function()
        return "echo hi"
      end,
    },
  })
end
`,
			errMsg: `field "sh" does not support callback values`,
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

func TestCommandsModuleRejectsUnsupportedFields(t *testing.T) {
	for _, field := range []string{"action", "windows", "options", "form"} {
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

func TestCommandsModule_ReplaceByName(t *testing.T) {
	entry := filepath.Join(t.TempDir(), "init.lua")
	require.NoError(t, os.WriteFile(entry, []byte(`
return function(hive)
  hive.commands({ LuaHello = { sh = "echo first" } })
  hive.commands({ LuaHello = { sh = "echo second" } })
end
`), 0o644))

	plugin := NewConfigPlugin(entry)
	require.NoError(t, plugin.Init(context.Background()))
	t.Cleanup(func() { require.NoError(t, plugin.Close()) })

	assert.Equal(t, "echo second", plugin.Commands()["LuaHello"].Sh)
}

// TestCommandsModule_TickerFiredRegistration is the regression test for the
// dynamic-registration scenario the epic targets: an entrypoint registers a
// command, schedules a ticker callback that re-registers the same command
// with a different sh, and the post-tick value is observable in the shared
// CommandSet. Uses a 1s after() because tickerMinInterval floors at 1s.
func TestCommandsModule_TickerFiredRegistration(t *testing.T) {
	entry := filepath.Join(t.TempDir(), "init.lua")
	require.NoError(t, os.WriteFile(entry, []byte(`
return function(hive)
  hive.commands({ Foo = { sh = "first" } })
  hive.ticker.after("1s", function()
    hive.commands({ Foo = { sh = "second" } })
  end)
end
`), 0o644))

	plugin, set := newConfigPluginWithSet(entry)
	require.NoError(t, plugin.Init(context.Background()))
	t.Cleanup(func() { require.NoError(t, plugin.Close()) })

	// Initial registration is synchronous through the entrypoint.
	require.Equal(t, "first", set.Plugin("lua")["Foo"].Sh)

	// Wait for the ticker to fire and replace.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if set.Plugin("lua")["Foo"].Sh == "second" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("ticker did not fire registration within deadline; got Sh=%q", set.Plugin("lua")["Foo"].Sh)
}
