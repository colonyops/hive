package hive

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/pkg/tmpl"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubExecutor captures commands passed to RunStream for inspection in tests.
type stubExecutor struct {
	cmds []string
}

func (s *stubExecutor) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return nil, nil
}

func (s *stubExecutor) RunDir(_ context.Context, _ string, _ string, _ ...string) ([]byte, error) {
	return nil, nil
}

func (s *stubExecutor) RunStream(_ context.Context, _, _ io.Writer, _ string, args ...string) error {
	if len(args) > 0 {
		s.cmds = append(s.cmds, args[len(args)-1])
	}
	return nil
}

func (s *stubExecutor) RunDirStream(_ context.Context, _ string, _, _ io.Writer, _ string, _ ...string) error {
	return nil
}

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

func TestRenderWindowHIVESessionID(t *testing.T) {
	r := testRenderer()
	w := config.WindowConfig{Name: "agent", Command: "claude"}

	t.Run("injects env var prefix when ID is set", func(t *testing.T) {
		rw, err := renderWindow(r, w, SpawnData{ID: "abc123", Name: "s"})
		require.NoError(t, err)
		assert.Equal(t, "HIVE_SESSION_ID=abc123 claude", rw.Command)
	})

	t.Run("skips injection when ID is empty", func(t *testing.T) {
		rw, err := renderWindow(r, w, SpawnData{ID: "", Name: "s"})
		require.NoError(t, err)
		assert.Equal(t, "claude", rw.Command)
	})

	t.Run("env var appears at start of command", func(t *testing.T) {
		rw, err := renderWindow(r, w, SpawnData{ID: "xyz99", Name: "s"})
		require.NoError(t, err)
		assert.True(t, len(rw.Command) > 0 && rw.Command[:len("HIVE_SESSION_ID=")] == "HIVE_SESSION_ID=",
			"HIVE_SESSION_ID must be first in command, got: %q", rw.Command)
	})
}

func TestSpawnHIVESessionID(t *testing.T) {
	r := testRenderer()

	t.Run("injects env var prefix when ID is set", func(t *testing.T) {
		exec := &stubExecutor{}
		s := NewSpawner(zerolog.Nop(), exec, r, nil, io.Discard, io.Discard)
		err := s.Spawn(context.Background(), []string{"claude"}, SpawnData{ID: "abc-123", Name: "s"})
		require.NoError(t, err)
		require.Len(t, exec.cmds, 1)
		assert.Equal(t, "HIVE_SESSION_ID=abc-123 claude", exec.cmds[0])
	})

	t.Run("skips injection when ID is empty", func(t *testing.T) {
		exec := &stubExecutor{}
		s := NewSpawner(zerolog.Nop(), exec, r, nil, io.Discard, io.Discard)
		err := s.Spawn(context.Background(), []string{"claude"}, SpawnData{ID: "", Name: "s"})
		require.NoError(t, err)
		require.Len(t, exec.cmds, 1)
		assert.Equal(t, "claude", exec.cmds[0])
	})

	t.Run("env var appears at start of command", func(t *testing.T) {
		exec := &stubExecutor{}
		s := NewSpawner(zerolog.Nop(), exec, r, nil, io.Discard, io.Discard)
		err := s.Spawn(context.Background(), []string{"claude"}, SpawnData{ID: "xyz-99", Name: "s"})
		require.NoError(t, err)
		require.Len(t, exec.cmds, 1)
		assert.True(t, strings.HasPrefix(exec.cmds[0], "HIVE_SESSION_ID="),
			"expected command to start with HIVE_SESSION_ID=, got: %q", exec.cmds[0])
	})
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
