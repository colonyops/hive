package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestUserCommand_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		expected UserCommand
	}{
		{
			name: "string shorthand",
			yaml: `sh: "echo hello"`,
			expected: UserCommand{
				Sh: "echo hello",
			},
		},
		{
			name: "full object",
			yaml: `
sh: "echo hello"
help: "Print hello"
confirm: "Really print?"
silent: true
exit: "true"`,
			expected: UserCommand{
				Sh:      "echo hello",
				Help:    "Print hello",
				Confirm: "Really print?",
				Silent:  true,
				Exit:    "true",
			},
		},
		{
			name: "minimal object",
			yaml: `sh: "ls -la"`,
			expected: UserCommand{
				Sh: "ls -la",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cmd UserCommand
			err := yaml.Unmarshal([]byte(tt.yaml), &cmd)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, cmd)
		})
	}
}

func TestUserCommand_StringShorthand(t *testing.T) {
	// Test the string shorthand works in a map context (as it would be used in config)
	yamlData := `
usercommands:
  simple: "echo hello"
  complex:
    sh: "echo world"
    help: "Complex command"
`

	type testConfig struct {
		UserCommands map[string]UserCommand `yaml:"usercommands"`
	}

	var cfg testConfig
	err := yaml.Unmarshal([]byte(yamlData), &cfg)
	require.NoError(t, err)

	assert.Len(t, cfg.UserCommands, 2)

	simple := cfg.UserCommands["simple"]
	assert.Equal(t, "echo hello", simple.Sh)
	assert.Empty(t, simple.Help)

	complex := cfg.UserCommands["complex"]
	assert.Equal(t, "echo world", complex.Sh)
	assert.Equal(t, "Complex command", complex.Help)
}

func TestUserCommand_Scope(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		expected UserCommand
	}{
		{
			name: "single scope",
			yaml: `
sh: "echo hello"
scope: ["review"]`,
			expected: UserCommand{
				Sh:    "echo hello",
				Scope: []string{"review"},
			},
		},
		{
			name: "multiple scopes",
			yaml: `
sh: "echo hello"
scope: ["review", "sessions"]`,
			expected: UserCommand{
				Sh:    "echo hello",
				Scope: []string{"review", "sessions"},
			},
		},
		{
			name: "global scope",
			yaml: `
sh: "echo hello"
scope: ["global"]`,
			expected: UserCommand{
				Sh:    "echo hello",
				Scope: []string{"global"},
			},
		},
		{
			name: "no scope (omitted)",
			yaml: `sh: "echo hello"`,
			expected: UserCommand{
				Sh:    "echo hello",
				Scope: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cmd UserCommand
			err := yaml.Unmarshal([]byte(tt.yaml), &cmd)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, cmd)
		})
	}
}
