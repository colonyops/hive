//go:build integration

package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigCommand_LuaPluginOverrideEntry(t *testing.T) {
	h := NewHarness(t)

	entry := filepath.Join(h.HomeDir(), "lua", "plugins", "custom.lua")
	require.NoError(t, os.MkdirAll(filepath.Dir(entry), 0o755))
	require.NoError(t, os.WriteFile(entry, []byte("return function() end\n"), 0o644))

	h.WithConfig(fmt.Sprintf(`
plugins:
  lua:
    entry: %q
`, entry))

	out, err := h.RunStdout("config")
	require.NoError(t, err)

	result, err := parseJSON(out)
	require.NoError(t, err, "parse config JSON: %s", out)

	plugins := result["plugins"].(map[string]any)
	lua := plugins["lua"].(map[string]any)
	assert.Equal(t, entry, lua["entry"])
}
