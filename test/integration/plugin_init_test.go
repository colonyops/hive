//go:build integration

package integration

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginInit_ScaffoldsDefaultEntry(t *testing.T) {
	h := NewHarness(t)

	out, err := h.Run("plugin", "init")
	require.NoError(t, err, "plugin init: %s", out)

	plugins := filepath.Join(h.HomeDir(), ".config", "hive", "plugins")
	assert.FileExists(t, filepath.Join(plugins, "init.lua"))
	assert.FileExists(t, filepath.Join(plugins, "commands", "hello.lua"))
}
