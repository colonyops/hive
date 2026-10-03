package terminal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectTool(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "claude keyword",
			content: "Welcome to Claude Code!",
			want:    "claude",
		},
		{
			name:    "anthropic keyword",
			content: "Powered by Anthropic",
			want:    "claude",
		},
		{
			name:    "ctrl+c to interrupt (claude specific)",
			content: "Thinking... ctrl+c to interrupt",
			want:    "claude",
		},
		{
			name:    "gemini keyword",
			content: "Welcome to Gemini CLI",
			want:    "gemini",
		},
		{
			name:    "aider version header",
			content: "Aider v0.80.0\n\n> ",
			want:    "aider",
		},
		{
			name:    "pi pane title",
			content: "π - hive-19ud3q",
			want:    "pi",
		},
		{
			name:    "opencode keyword",
			content: "OpenCode v1.0",
			want:    "opencode",
		},
		{
			name:    "codex keyword",
			content: "OpenAI Codex",
			want:    "codex",
		},
		{
			name:    "cursor keyword",
			content: "Welcome to Cursor IDE",
			want:    "cursor",
		},
		{
			name:    "crush keyword",
			content: "Crush v2.0 ready",
			want:    "crush",
		},
		{
			name:    "agent keyword",
			content: "Starting Agent session",
			want:    "agent",
		},
		{
			name:    "aider word without version is not enough",
			content: "grep aider README.md",
			want:    "shell",
		},
		{
			name:    "pi substring is not enough",
			content: "pipeline output",
			want:    "shell",
		},
		{
			name:    "unknown - defaults to shell",
			content: "user@host:~$",
			want:    "shell",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectTool(tt.content)
			assert.Equal(t, tt.want, got, "DetectTool() = %v, want %v", got, tt.want)
		})
	}
}

func TestDetectTool_DeterministicOnMultipleKeywords(t *testing.T) {
	content := "Powered by Anthropic Claude, running on OpenAI Codex infra"
	for i := 0; i < 100; i++ {
		got := DetectTool(content)
		assert.Equal(t, "claude", got, "DetectTool() must deterministically prefer claude over codex")
	}
}
