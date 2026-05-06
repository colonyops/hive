package lua

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateModuleName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{name: "simple name", input: "commands"},
		{name: "dotted name", input: "commands.hello"},
		{name: "deeply nested", input: "a.b.c.d"},
		{name: "empty", input: "", wantErr: "cannot be empty"},
		{name: "forward slash", input: "commands/hello", wantErr: "must use dot notation"},
		{name: "backslash", input: `commands\hello`, wantErr: "must use dot notation"},
		{name: "leading dot", input: ".hello", wantErr: "is invalid"},
		{name: "trailing dot", input: "hello.", wantErr: "is invalid"},
		{name: "double dot", input: "a..b", wantErr: "is invalid"},
		{name: "parent traversal", input: "..", wantErr: "is invalid"},
		{name: "current dir", input: ".", wantErr: "is invalid"},
		{name: "relative parent", input: "a..b.c", wantErr: "is invalid"},
		{name: "embedded parent segment", input: "a.b...c", wantErr: "is invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateModuleName(tt.input)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestRequireRejectsPathTraversal(t *testing.T) {
	root := t.TempDir()
	entry := filepath.Join(root, "init.lua")
	require.NoError(t, os.WriteFile(entry, []byte(`
local ok, slashErr = pcall(require, "../escape")
if ok then error("expected require to reject path separators") end

ok, dotErr = pcall(require, "a..b")
if ok then error("expected require to reject parent traversal segments") end

return function(hive)
  hive.commands({ Ok = { sh = "echo ok", help = slashErr .. "|" .. dotErr } })
end
`), 0o644))

	plugin := NewConfigPlugin(entry)
	require.NoError(t, plugin.Init(context.Background()))
	t.Cleanup(func() { require.NoError(t, plugin.Close()) })

	help := plugin.Commands()["Ok"].Help
	assert.Contains(t, help, "must use dot notation")
	assert.Contains(t, help, "is invalid")
}

func TestHiveLogFunctionsAreInvocable(t *testing.T) {
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
