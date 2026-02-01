package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseExitCondition(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		envName  string
		envValue string
		expected bool
	}{
		{
			name:     "empty string returns false",
			input:    "",
			expected: false,
		},
		{
			name:     "true string returns true",
			input:    "true",
			expected: true,
		},
		{
			name:     "false string returns false",
			input:    "false",
			expected: false,
		},
		{
			name:     "1 returns true",
			input:    "1",
			expected: true,
		},
		{
			name:     "0 returns false",
			input:    "0",
			expected: false,
		},
		{
			name:     "TRUE returns true",
			input:    "TRUE",
			expected: true,
		},
		{
			name:     "invalid string returns false",
			input:    "invalid",
			expected: false,
		},
		{
			name:     "env var set to true",
			input:    "$TEST_EXIT_VAR",
			envName:  "TEST_EXIT_VAR",
			envValue: "true",
			expected: true,
		},
		{
			name:     "env var set to 1",
			input:    "$TEST_EXIT_VAR",
			envName:  "TEST_EXIT_VAR",
			envValue: "1",
			expected: true,
		},
		{
			name:     "env var set to false",
			input:    "$TEST_EXIT_VAR",
			envName:  "TEST_EXIT_VAR",
			envValue: "false",
			expected: false,
		},
		{
			name:     "env var unset returns false",
			input:    "$TEST_EXIT_VAR_UNSET",
			expected: false,
		},
		{
			name:     "env var set to empty string returns false",
			input:    "$TEST_EXIT_VAR",
			envName:  "TEST_EXIT_VAR",
			envValue: "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envName != "" {
				t.Setenv(tt.envName, tt.envValue)
			}

			result := ParseExitCondition(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestKeybinding_ShouldExit(t *testing.T) {
	tests := []struct {
		name     string
		exit     string
		envName  string
		envValue string
		expected bool
	}{
		{
			name:     "static true",
			exit:     "true",
			expected: true,
		},
		{
			name:     "static false",
			exit:     "false",
			expected: false,
		},
		{
			name:     "env var true",
			exit:     "$HIVE_POPUP",
			envName:  "HIVE_POPUP",
			envValue: "true",
			expected: true,
		},
		{
			name:     "env var false",
			exit:     "$HIVE_POPUP",
			envName:  "HIVE_POPUP",
			envValue: "false",
			expected: false,
		},
		{
			name:     "env var unset",
			exit:     "$HIVE_POPUP_UNSET",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envName != "" {
				t.Setenv(tt.envName, tt.envValue)
			}

			kb := Keybinding{Exit: tt.exit}
			assert.Equal(t, tt.expected, kb.ShouldExit())
		})
	}
}
