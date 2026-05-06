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

	// Pass env explicitly via tmux -e so the TUI sees the current harness's
	// data dir and config rather than whatever env tmux's persistent server
	// captured when an earlier test first started it. Use a wider window so
	// the session name is not truncated in the TUI tree view.
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

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		out, err := exec.Command("tmux", "capture-pane", "-t", uiSession, "-p").CombinedOutput()
		assert.NoError(c, err, "tmux capture-pane: %s", out)
		assert.Contains(c, string(out), sessionName)
	}, 5*time.Second, 200*time.Millisecond)

	// Move cursor down past the repo header onto the session row so the
	// command palette has a selected session for the Lua command to act on.
	_, err = exec.Command("tmux", "send-keys", "-t", uiSession, "j").CombinedOutput()
	require.NoError(t, err)

	_, err = exec.Command("tmux", "send-keys", "-t", uiSession, ":", "LuaHello", "Enter").CombinedOutput()
	require.NoError(t, err)

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		content, err := os.ReadFile(filepath.Join(sessionPath, ".lua-plugin-output"))
		assert.NoError(c, err)
		assert.Equal(c, "lua command ran", string(content))
	}, 5*time.Second, 200*time.Millisecond)
}

func TestLuaPluginInvalidEntrypointLogsWarningAndStartupContinues(t *testing.T) {
	tests := []struct {
		name     string
		script   string
		logMatch string
	}{
		{
			name: "syntax error",
			script: `
return function(hive)
  this is not valid lua
end
`,
			logMatch: "load lua plugin",
		},
		{
			name:     "wrong return type",
			script:   "return {}\n",
			logMatch: "must return a function",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHarness(t)

			entry := filepath.Join(h.DataDir(), "lua", "plugins", "init.lua")
			require.NoError(t, os.MkdirAll(filepath.Dir(entry), 0o755))
			require.NoError(t, os.WriteFile(entry, []byte(tt.script), 0o644))

			h.WithConfig(fmt.Sprintf(`version: "0.2.4"
plugins:
  lua:
    entry: %q
`, entry))

			out, err := h.RunStdout("config")
			require.NoError(t, err)
			_, err = parseJSON(out)
			require.NoError(t, err, "parse config JSON: %s", out)

			logPath := filepath.Join(h.DataDir(), "hive.log")
			logContent, err := os.ReadFile(logPath)
			require.NoError(t, err)
			assert.Contains(t, string(logContent), "plugin initialization failed")
			assert.Contains(t, string(logContent), tt.logMatch)
		})
	}
}
