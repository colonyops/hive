package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstallHooks(t *testing.T) {
	t.Run("creates settings.json with hooks when none exist", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, InstallHooks(dir))

		settingsPath := filepath.Join(dir, ".claude", "settings.json")
		data, err := os.ReadFile(settingsPath)
		require.NoError(t, err)

		var raw map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(data, &raw))

		var hooksMap map[string][]claudeHookMatcher
		require.NoError(t, json.Unmarshal(raw["hooks"], &hooksMap))

		assert.Contains(t, hooksMap, "Stop")
		assert.Contains(t, hooksMap, "PreToolUse")
		assert.NotEmpty(t, hooksMap["Stop"])
		assert.NotEmpty(t, hooksMap["PreToolUse"])
	})

	t.Run("stop hook writes ready status", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, InstallHooks(dir))

		settingsPath := filepath.Join(dir, ".claude", "settings.json")
		data, err := os.ReadFile(settingsPath)
		require.NoError(t, err)

		var raw map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(data, &raw))

		var hooksMap map[string][]claudeHookMatcher
		require.NoError(t, json.Unmarshal(raw["hooks"], &hooksMap))

		stopHooks := hooksMap["Stop"]
		require.NotEmpty(t, stopHooks)
		assert.Equal(t, hookStopCommand, stopHooks[0].Hooks[0].Command)
		assert.Equal(t, "command", stopHooks[0].Hooks[0].Type)
	})

	t.Run("pre-tool-use hook writes active status", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, InstallHooks(dir))

		settingsPath := filepath.Join(dir, ".claude", "settings.json")
		data, err := os.ReadFile(settingsPath)
		require.NoError(t, err)

		var raw map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(data, &raw))

		var hooksMap map[string][]claudeHookMatcher
		require.NoError(t, json.Unmarshal(raw["hooks"], &hooksMap))

		preToolHooks := hooksMap["PreToolUse"]
		require.NotEmpty(t, preToolHooks)
		assert.Equal(t, hookPreToolUseCommand, preToolHooks[0].Hooks[0].Command)
	})

	t.Run("is idempotent — does not duplicate hooks on repeated calls", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, InstallHooks(dir))
		require.NoError(t, InstallHooks(dir))
		require.NoError(t, InstallHooks(dir))

		settingsPath := filepath.Join(dir, ".claude", "settings.json")
		data, err := os.ReadFile(settingsPath)
		require.NoError(t, err)

		var raw map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(data, &raw))

		var hooksMap map[string][]claudeHookMatcher
		require.NoError(t, json.Unmarshal(raw["hooks"], &hooksMap))

		assert.Len(t, hooksMap["Stop"], 1, "Stop hooks must not be duplicated")
		assert.Len(t, hooksMap["PreToolUse"], 1, "PreToolUse hooks must not be duplicated")
	})

	t.Run("preserves existing hooks when merging", func(t *testing.T) {
		dir := t.TempDir()
		claudeDir := filepath.Join(dir, ".claude")
		require.NoError(t, os.MkdirAll(claudeDir, 0o755))

		existing := `{
  "hooks": {
    "Stop": [
      {
        "matcher": "myTool",
        "hooks": [{"type": "command", "command": "echo existing"}]
      }
    ]
  }
}`
		require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(existing), 0o644))

		require.NoError(t, InstallHooks(dir))

		data, err := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
		require.NoError(t, err)

		var raw map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(data, &raw))

		var hooksMap map[string][]claudeHookMatcher
		require.NoError(t, json.Unmarshal(raw["hooks"], &hooksMap))

		// Should have both the existing hook and our new one.
		assert.Len(t, hooksMap["Stop"], 2)

		// Original entry must still be present.
		found := false
		for _, m := range hooksMap["Stop"] {
			for _, e := range m.Hooks {
				if e.Command == "echo existing" {
					found = true
				}
			}
		}
		assert.True(t, found, "existing hook must be preserved")
	})

	t.Run("preserves non-hooks settings fields", func(t *testing.T) {
		dir := t.TempDir()
		claudeDir := filepath.Join(dir, ".claude")
		require.NoError(t, os.MkdirAll(claudeDir, 0o755))

		existing := `{"permissions": {"allow": ["Bash"]}, "model": "claude-opus-4-5"}`
		require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(existing), 0o644))

		require.NoError(t, InstallHooks(dir))

		data, err := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
		require.NoError(t, err)

		var raw map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(data, &raw))

		// Both existing fields must survive.
		assert.Contains(t, raw, "permissions")
		assert.Contains(t, raw, "model")
		assert.Contains(t, raw, "hooks")
	})
}
