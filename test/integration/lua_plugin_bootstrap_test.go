//go:build integration

package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLuaPluginBootstrapRequiresModuleAndExecutesCommand(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "lua-plugin-repo")

	entry := filepath.Join(h.DataDir(), "lua", "plugins", "init.lua")
	module := filepath.Join(h.DataDir(), "lua", "plugins", "commands", "hello.lua")
	require.NoError(t, os.MkdirAll(filepath.Dir(module), 0o755))
	require.NoError(t, os.WriteFile(module, []byte(`
return {
  LuaHello = {
    sh = "printf 'lua command ran' > .lua-plugin-output",
    help = "lua command",
    scope = {"sessions"},
    silent = true,
  },
}
`), 0o644))
	require.NoError(t, os.WriteFile(entry, []byte(`
local commands = require("commands.hello")

return function(hive)
  hive.commands(commands)
end
`), 0o644))

	h.WithConfig(fmt.Sprintf(`version: "0.2.4"
git_path: git
agents:
  default: testbash
  testbash:
    command: bash
rules:
  - spawn:
      - "tmux new-session -d -s {{ .Name | shq }} -c {{ .Path | shq }}"
    batch_spawn:
      - "tmux new-session -d -s {{ .Name | shq }} -c {{ .Path | shq }}"
plugins:
  lua:
    entry: %q
`, entry))

	sessionName := "lua-plugin-command"
	sessionOut, err := h.Run("new", "--remote", repo, sessionName)
	require.NoError(t, err)
	sessionPath := parseCreatedSessionPath(t, sessionOut)

	uiSession := "lua-plugin-ui"
	cleanupTmuxSession(t, sessionName)
	cleanupTmuxSession(t, uiSession)

	cmd := exec.Command("tmux", "new-session", "-d", "-s", uiSession, hiveBin)
	cmd.Env = h.command().Env
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "tmux new-session: %s", out)

	assertTmuxSessionExists(t, uiSession)

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		out, err := exec.Command("tmux", "capture-pane", "-t", uiSession, "-p").CombinedOutput()
		assert.NoError(c, err, "tmux capture-pane: %s", out)
		assert.Contains(c, string(out), sessionName)
	}, 5*time.Second, 200*time.Millisecond)

	_, err = exec.Command("tmux", "send-keys", "-t", uiSession, ":", "LuaHello", "Enter").CombinedOutput()
	require.NoError(t, err)

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		content, err := os.ReadFile(filepath.Join(sessionPath, ".lua-plugin-output"))
		assert.NoError(c, err)
		assert.Equal(c, "lua command ran", string(content))
	}, 5*time.Second, 200*time.Millisecond)
}
