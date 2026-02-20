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

func TestRenderWindowCommon(t *testing.T) {
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

func TestRenderWindowCommon_Panes(t *testing.T) {
	r := testRenderer()
	w := config.WindowConfig{
		Name:    "agent",
		Command: "claude",
		Panes: []config.PaneConfig{
			{Command: "tail -f {{ .Name }}.log", Horizontal: true},
			{Dir: "/logs/{{ .Name }}"},
		},
	}

	rw, err := renderWindow(r, w, SpawnData{Name: "my-sess", Path: "/tmp/test"})
	require.NoError(t, err)

	require.Len(t, rw.Panes, 2)

	assert.Equal(t, "tail -f my-sess.log", rw.Panes[0].Command)
	assert.True(t, rw.Panes[0].Horizontal)

	assert.Empty(t, rw.Panes[1].Command)
	assert.Equal(t, "/logs/my-sess", rw.Panes[1].Dir)
}

func TestRenderUserCommandWindows(t *testing.T) {
	r := testRenderer()
	windows := []config.WindowConfig{
		{Name: "leader", Command: "claude 'Lead the review for {{ .Form.pr }}'"},
		{Name: "analyst", Command: "claude 'Analyse PR {{ .Form.pr }}'"},
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
