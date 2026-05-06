package lua

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogModuleFunctionsAreInvocable(t *testing.T) {
	entry := filepath.Join(t.TempDir(), "init.lua")
	require.NoError(t, os.WriteFile(entry, []byte(`
return function(hive)
  hive.log.debug("d")
  hive.log.info("i")
  hive.log.warn("w")
  hive.log.error("e")
  hive.commands({ Logged = { sh = "echo done" } })
end
`), 0o644))

	plugin := NewConfigPlugin(entry)
	require.NoError(t, plugin.Init(context.Background()))
	t.Cleanup(func() { require.NoError(t, plugin.Close()) })

	assert.Contains(t, plugin.Commands(), "Logged")
}
