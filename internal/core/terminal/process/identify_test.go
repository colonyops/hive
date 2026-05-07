package process

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLooksLikeClaude(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		argv []string
		want bool
	}{
		{
			name: "CLAUDECODE=1 in env",
			env:  map[string]string{"CLAUDECODE": "1"},
			argv: nil,
			want: true,
		},
		{
			name: "CLAUDECODE absent but argv contains claude",
			env:  map[string]string{"PATH": "/usr/bin"},
			argv: []string{"/usr/local/bin/claude"},
			want: true,
		},
		{
			name: "neither env nor argv match",
			env:  map[string]string{"PATH": "/usr/bin"},
			argv: []string{"/bin/bash"},
			want: false,
		},
		{
			name: "nil env but argv has claude",
			env:  nil,
			argv: []string{"/usr/local/bin/claude"},
			want: true,
		},
		{
			name: "nil env and nil argv",
			env:  nil,
			argv: nil,
			want: false,
		},
		{
			name: "CLAUDECODE not equal to 1",
			env:  map[string]string{"CLAUDECODE": "0"},
			argv: []string{"/bin/bash"},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := looksLikeClaude(tt.env, tt.argv)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestToolFromArgv(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		want string
	}{
		{
			name: "npx claude",
			argv: []string{"npx", "claude"},
			want: "claude",
		},
		{
			name: "node with gemini path",
			argv: []string{"node", "/path/to/gemini"},
			want: "gemini",
		},
		{
			name: "direct aider",
			argv: []string{"/usr/local/bin/aider"},
			want: "aider",
		},
		{
			name: "empty argv",
			argv: []string{},
			want: "",
		},
		{
			name: "nil argv",
			argv: nil,
			want: "",
		},
		{
			name: "bash shell",
			argv: []string{"/bin/bash"},
			want: "",
		},
		{
			name: "npx with unknown tool",
			argv: []string{"npx", "some-unknown-tool"},
			want: "",
		},
		{
			name: "direct claude binary",
			argv: []string{"/usr/local/bin/claude"},
			want: "claude",
		},
		{
			name: "codex",
			argv: []string{"/usr/bin/codex"},
			want: "codex",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toolFromArgv(tt.argv)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestToolFromBasename(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "claude exact",
			input: "claude",
			want:  "claude",
		},
		{
			name:  "claude with version suffix",
			input: "claude-3.5",
			want:  "claude",
		},
		{
			name:  "aider",
			input: "aider",
			want:  "aider",
		},
		{
			name:  "gemini",
			input: "gemini",
			want:  "gemini",
		},
		{
			name:  "codex",
			input: "codex",
			want:  "codex",
		},
		{
			name:  "cursor",
			input: "cursor",
			want:  "cursor",
		},
		{
			name:  "opencode",
			input: "opencode",
			want:  "opencode",
		},
		{
			name:  "cline",
			input: "cline",
			want:  "cline",
		},
		{
			name:  "bash returns empty",
			input: "bash",
			want:  "",
		},
		{
			name:  "zsh returns empty",
			input: "zsh",
			want:  "",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "uppercase CLAUDE",
			input: "CLAUDE",
			want:  "claude",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toolFromBasename(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}
