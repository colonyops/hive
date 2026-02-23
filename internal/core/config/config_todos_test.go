package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateTodos(t *testing.T) {
	t.Run("empty actions passes", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Todos.Actions = nil
		assert.NoError(t, cfg.validateTodos())
	})

	t.Run("custom scheme with valid template passes", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Todos.Actions = map[string]string{
			"jira": "open https://jira.example.com/{{ .Value }}",
		}
		assert.NoError(t, cfg.validateTodos())
	})

	t.Run("built-in scheme override rejected", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Todos.Actions = map[string]string{
			"review": "echo {{ .Value }}",
		}
		err := cfg.validateTodos()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot override built-in scheme")
	})

	t.Run("malformed template rejected", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Todos.Actions = map[string]string{
			"custom": "{{ .Invalid",
		}
		err := cfg.validateTodos()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid template")
	})
}
