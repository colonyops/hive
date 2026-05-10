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

func TestLuaPluginDynamicCommandRegistrationFromTicker(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "lua-dynamic-repo")

	entry := filepath.Join(h.DataDir(), "lua", "plugins", "init.lua")
	require.NoError(t, os.MkdirAll(filepath.Dir(entry), 0o755))

	// Sentinel path is in the harness data dir so the test can read it back.
	sentinel := filepath.Join(h.DataDir(), "dynamic-output")

	// Register LuaDynamic with the "first" payload at init, then re-register it
	// from a ticker callback with "second". After 1s the ticker fires; the
	// command palette should resolve LuaDynamic to the second definition.
	require.NoError(t, os.WriteFile(entry, []byte(fmt.Sprintf(`
return function(hive)
  hive.commands({
    LuaDynamic = {
      sh = [[printf 'first' > %s]],
      help = "dynamic command (initial)",
      scope = {"sessions"},
      silent = true,
    },
  })
  hive.ticker.after("1s", function()
    hive.commands({
      LuaDynamic = {
        sh = [[printf 'second' > %s]],
        help = "dynamic command (replaced)",
        scope = {"sessions"},
        silent = true,
      },
    })
  end)
end
`, sentinel, sentinel)), 0o644))

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

	sessionName := "lua-dynamic-cmd"
	_, err := h.Run("new", "--remote", repo, sessionName)
	require.NoError(t, err)

	uiSession := "lua-dynamic-ui"
	cleanupTmuxSession(t, sessionName)
	cleanupTmuxSession(t, uiSession)

	tmuxArgs := []string{"new-session", "-d", "-s", uiSession, "-x", "200", "-y", "50"}
	for _, kv := range h.command().Env {
		tmuxArgs = append(tmuxArgs, "-e", kv)
	}
	tmuxArgs = append(tmuxArgs, hiveBin)
	cmd := exec.Command("tmux", tmuxArgs...)
	cmd.Env = h.command().Env
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "tmux new-session: %s", out)

	assertTmuxSessionExists(t, uiSession)

	// Wait for the TUI to render the session row.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		out, err := exec.Command("tmux", "capture-pane", "-t", uiSession, "-p").CombinedOutput()
		assert.NoError(c, err, "tmux capture-pane: %s", out)
		assert.Contains(c, string(out), sessionName)
	}, 5*time.Second, 200*time.Millisecond)

	// Wait for the ticker to fire so the second registration is in the
	// CommandSet before we open the palette. tickerMinInterval is 1s, so
	// wait a bit longer than that.
	time.Sleep(1500 * time.Millisecond)

	// Move cursor down past the repo header onto the session row.
	_, err = exec.Command("tmux", "send-keys", "-t", uiSession, "j").CombinedOutput()
	require.NoError(t, err)

	// Open the palette and run :LuaDynamic. The palette consults the live
	// CommandSet at open, so the second-write-wins registration must be
	// resolved here — proving both the dynamic registration and the
	// Phase 2 palette migration.
	_, err = exec.Command("tmux", "send-keys", "-t", uiSession, ":", "LuaDynamic", "Enter").CombinedOutput()
	require.NoError(t, err)

	// Assert the sentinel contains "second" (the post-ticker payload).
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		content, err := os.ReadFile(sentinel)
		if !assert.NoError(c, err) {
			return
		}
		assert.Equal(c, "second", string(content))
	}, 5*time.Second, 200*time.Millisecond)
}
