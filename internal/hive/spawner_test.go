package hive

import (
	"testing"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/pkg/tmpl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testRenderer() *tmpl.Renderer {
	return tmpl.New(tmpl.Config{AgentCommand: "claude", AgentWindow: "claude"})
}

func TestPosixQuote(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "'hello'"},
		{"hello world", "'hello world'"},
		{"it's a test", `'it'\''s a test'`},
		{"don't 'quote' me", `'don'\''t '\''quote'\'' me'`},
		{"", "''"},
		{"line1\nline2", "'line1\nline2'"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			assert.Equal(t, tc.want, posixQuote(tc.input))
		})
	}
}

func TestRenderWindowCommon_NoPrompt(t *testing.T) {
	r := testRenderer()
	w := config.WindowConfig{
		Name:    "agent",
		Command: "claude --session {{ .Name }}",
		Focus:   true,
	}

	rw, err := renderWindow(r, w, SpawnData{Name: "test-session", Path: "/tmp/test"})
	require.NoError(t, err)
	assert.Equal(t, "agent", rw.Name)
	assert.Equal(t, "claude --session test-session", rw.Command)
	assert.True(t, rw.Focus)
}

func TestRenderWindowCommon_PromptWithEmptyCommand(t *testing.T) {
	r := testRenderer()
	w := config.WindowConfig{
		Name:   "worker",
		Prompt: "Do the work in {{ .Path }}",
	}

	rw, err := renderWindow(r, w, SpawnData{Name: "s", Path: "/repo"})
	require.NoError(t, err)
	// When no Command is set, agentCommand is used as the base.
	assert.Equal(t, "worker", rw.Name)
	assert.Equal(t, "claude '"+`Do the work in /repo`+"'", rw.Command)
}

func TestRenderWindowCommon_PromptWithCommand(t *testing.T) {
	r := testRenderer()
	w := config.WindowConfig{
		Name:    "worker",
		Command: "aider",
		Prompt:  "Fix the bug",
	}

	rw, err := renderWindow(r, w, SpawnData{Name: "s", Path: "/repo"})
	require.NoError(t, err)
	// When Command is set, Prompt is appended after posixQuote.
	assert.Equal(t, "aider 'Fix the bug'", rw.Command)
}

func TestRenderWindowCommon_PromptWithSingleQuotes(t *testing.T) {
	r := testRenderer()
	w := config.WindowConfig{
		Name:   "worker",
		Prompt: "it's a prompt",
	}

	rw, err := renderWindow(r, w, SpawnData{})
	require.NoError(t, err)
	assert.Equal(t, `claude 'it'\''s a prompt'`, rw.Command)
}

func TestRenderUserCommandWindows(t *testing.T) {
	r := testRenderer()
	windows := []config.WindowConfig{
		{Name: "leader", Prompt: "Lead the review for {{ .Form.pr }}"},
		{Name: "analyst", Command: "claude", Prompt: "Analyse PR {{ .Form.pr }}"},
	}
	data := map[string]any{
		"Form": map[string]any{"pr": "123"},
		"ID":   "sess-abc",
	}

	rendered, err := RenderUserCommandWindows(r, windows, data)
	require.NoError(t, err)
	require.Len(t, rendered, 2)

	assert.Equal(t, "leader", rendered[0].Name)
	assert.Contains(t, rendered[0].Command, "Lead the review for 123")

	assert.Equal(t, "analyst", rendered[1].Name)
	assert.Contains(t, rendered[1].Command, "Analyse PR 123")
}
