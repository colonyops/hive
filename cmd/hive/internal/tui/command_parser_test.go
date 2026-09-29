package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseCommandInput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected ParsedCommand
	}{
		{
			name:  "simple command with colon",
			input: ":commit",
			expected: ParsedCommand{
				Name: "commit",
				Args: []string{},
			},
		},
		{
			name:  "simple command without colon",
			input: "commit",
			expected: ParsedCommand{
				Name: "commit",
				Args: []string{},
			},
		},
		{
			name:  "command with single arg",
			input: ":push origin",
			expected: ParsedCommand{
				Name: "push",
				Args: []string{"origin"},
			},
		},
		{
			name:  "command with multiple args",
			input: ":deploy staging --force",
			expected: ParsedCommand{
				Name: "deploy",
				Args: []string{"staging", "--force"},
			},
		},
		{
			name:  "command with extra whitespace",
			input: "  :sync   local   remote  ",
			expected: ParsedCommand{
				Name: "sync",
				Args: []string{"local", "remote"},
			},
		},
		{
			name:  "empty input",
			input: "",
			expected: ParsedCommand{
				Name: "",
				Args: nil,
			},
		},
		{
			name:  "only colon",
			input: ":",
			expected: ParsedCommand{
				Name: "",
				Args: nil,
			},
		},
		{
			name:  "only whitespace",
			input: "   ",
			expected: ParsedCommand{
				Name: "",
				Args: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseCommandInput(tt.input)

			assert.Equal(t, tt.expected.Name, result.Name, "ParseCommandInput(%q) name = %q, want %q", tt.input, result.Name, tt.expected.Name)
			assert.Len(t, result.Args, len(tt.expected.Args), "ParseCommandInput(%q) args length = %d, want %d", tt.input, len(result.Args), len(tt.expected.Args))
			assert.Equal(t, tt.expected.Args, result.Args, "ParseCommandInput(%q) args = %v, want %v", tt.input, result.Args, tt.expected.Args)
		})
	}
}
