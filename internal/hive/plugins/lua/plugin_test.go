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

func TestPluginInitReplacesPriorRunOnReinitialization(t *testing.T) {
	entry := filepath.Join(t.TempDir(), "init.lua")
	require.NoError(t, os.WriteFile(entry, []byte(`
return function(hive)
  hive.commands({ First = { sh = "echo first" } })
end
`), 0o644))

	plugin := NewConfigPlugin(entry)
	require.NoError(t, plugin.Init(context.Background()))
	t.Cleanup(func() { require.NoError(t, plugin.Close()) })

	require.Contains(t, plugin.Commands(), "First")

	require.NoError(t, os.WriteFile(entry, []byte(`
return function(hive)
  hive.commands({ Second = { sh = "echo second" } })
end
`), 0o644))

	require.NoError(t, plugin.Init(context.Background()))

	assert.NotContains(t, plugin.Commands(), "First", "stale command from prior init must not survive re-initialization")
	assert.Contains(t, plugin.Commands(), "Second")
}

func TestPluginInitDoesNotLeakCommandsOnFailure(t *testing.T) {
	entry := filepath.Join(t.TempDir(), "init.lua")
	require.NoError(t, os.WriteFile(entry, []byte(`
return function(hive)
  hive.commands({ Good = { sh = "echo good" } })
  error("boom")
end
`), 0o644))

	plugin := NewConfigPlugin(entry)
	err := plugin.Init(context.Background())
	require.Error(t, err)

	assert.Empty(t, plugin.Commands(), "Commands() must be empty after a failed Init")
}

// NewConfigPlugin builds a Plugin from a single entry file path. Shared by
// the lifecycle, runtime, and per-module tests in this package.
func NewConfigPlugin(entry string) *Plugin {
	return New(config.LuaPluginConfig{Entry: entry})
}
