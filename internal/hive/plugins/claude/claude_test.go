package claude

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/colonyops/hive/internal/core/config"
)

func TestPlugin_Available(t *testing.T) {
	t.Run("returns false when explicitly disabled", func(t *testing.T) {
		disabled := false
		plugin := New(config.ClaudePluginConfig{
			Enabled: &disabled,
		})

		assert.False(t, plugin.Available())
	})

	t.Run("checks claude executable when enabled", func(t *testing.T) {
		enabled := true
		plugin := New(config.ClaudePluginConfig{
			Enabled: &enabled,
		})

		_ = plugin.Available()
	})
}

func TestPlugin_StatusProvider(t *testing.T) {
	plugin := New(config.ClaudePluginConfig{})

	assert.Nil(t, plugin.StatusProvider())
}

func TestPlugin_Commands(t *testing.T) {
	plugin := New(config.ClaudePluginConfig{})

	commands := plugin.Commands()
	assert.Contains(t, commands, "ClaudeFork")
}
