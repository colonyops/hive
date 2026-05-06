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

func TestLuaPluginBootstrapRegistersKnownCommand(t *testing.T) {
	h := NewHarness(t)

	entry := filepath.Join(h.DataDir(), "lua", "init.lua")
	require.NoError(t, os.MkdirAll(filepath.Dir(entry), 0o755))
	require.NoError(t, os.WriteFile(entry, []byte(`
return function(hive)
  hive.commands({
    LuaHello = {
      sh = "echo lua",
      help = "lua command",
      scope = {"sessions"},
    },
  })
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

	sessionName := "lua-plugin-bootstrap"
	cleanupTmuxSession(t, sessionName)

	cmd := exec.Command("tmux", "new-session", "-d", "-s", sessionName, hiveBin)
	cmd.Env = h.command().Env
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "tmux new-session: %s", out)

	assertTmuxSessionExists(t, sessionName)

	// Wait for the TUI to finish initial startup before opening the palette.
	time.Sleep(500 * time.Millisecond)

	_, err = exec.Command("tmux", "send-keys", "-t", sessionName, ":", "LuaHello").CombinedOutput()
	require.NoError(t, err)

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		out, err := exec.Command("tmux", "capture-pane", "-t", sessionName, "-p").CombinedOutput()
		assert.NoError(c, err, "tmux capture-pane: %s", out)
		assert.Contains(c, string(out), "LuaHello")
		assert.Contains(c, string(out), "lua command")
	}, 5*time.Second, 200*time.Millisecond)
}
