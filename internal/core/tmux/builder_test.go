package tmux

import (
	"context"
	"fmt"
	"testing"

	"github.com/colonyops/hive/pkg/executil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuilder_HasSession(t *testing.T) {
	t.Run("exists", func(t *testing.T) {
		rec := &executil.RecordingExecutor{}
		b := NewBuilder(rec)

		assert.True(t, b.HasSession(context.Background(), "my-session"))
		require.Len(t, rec.Commands, 1)
		assert.Equal(t, "tmux", rec.Commands[0].Cmd)
		assert.Equal(t, []string{"has-session", "-t", "my-session"}, rec.Commands[0].Args)
	})

	t.Run("not exists", func(t *testing.T) {
		rec := &executil.RecordingExecutor{
			Errors: map[string]error{"tmux": fmt.Errorf("session not found")},
		}
		b := NewBuilder(rec)

		assert.False(t, b.HasSession(context.Background(), "missing"))
	})
}

func TestBuilder_CreateSession(t *testing.T) {
	t.Run("single window", func(t *testing.T) {
		rec := &executil.RecordingExecutor{}
		b := NewBuilder(rec)

		err := b.CreateSession(context.Background(), "sess", "/work", []RenderedWindow{
			{Name: "agent", Command: "claude", Focus: true},
		}, true)
		require.NoError(t, err)

		require.Len(t, rec.Commands, 2) // new-session + select-window
		assert.Equal(t, []string{"new-session", "-d", "-s", "sess", "-n", "agent", "-c", "/work", "--", "sh", "-c", "claude"}, rec.Commands[0].Args)
		assert.Equal(t, []string{"select-window", "-t", "sess:agent"}, rec.Commands[1].Args)
	})

	t.Run("two windows", func(t *testing.T) {
		rec := &executil.RecordingExecutor{}
		b := NewBuilder(rec)

		err := b.CreateSession(context.Background(), "sess", "/work", []RenderedWindow{
			{Name: "agent", Command: "claude", Focus: true},
			{Name: "shell"},
		}, true)
		require.NoError(t, err)

		require.Len(t, rec.Commands, 3) // new-session + new-window + select-window
		assert.Equal(t, []string{"new-session", "-d", "-s", "sess", "-n", "agent", "-c", "/work", "--", "sh", "-c", "claude"}, rec.Commands[0].Args)
		assert.Equal(t, []string{"new-window", "-t", "sess", "-n", "shell", "-c", "/work"}, rec.Commands[1].Args)
		assert.Equal(t, []string{"select-window", "-t", "sess:agent"}, rec.Commands[2].Args)
	})

	t.Run("three windows with dir override", func(t *testing.T) {
		rec := &executil.RecordingExecutor{}
		b := NewBuilder(rec)

		err := b.CreateSession(context.Background(), "sess", "/work", []RenderedWindow{
			{Name: "agent", Command: "claude", Focus: true},
			{Name: "shell"},
			{Name: "logs", Command: "tail -f /var/log/app.log", Dir: "/var/log"},
		}, true)
		require.NoError(t, err)

		require.Len(t, rec.Commands, 4) // new-session + 2*new-window + select-window
		// Third window uses custom dir
		assert.Equal(t, []string{"new-window", "-t", "sess", "-n", "logs", "-c", "/var/log", "--", "sh", "-c", "tail -f /var/log/app.log"}, rec.Commands[2].Args)
	})

	t.Run("focus second window", func(t *testing.T) {
		rec := &executil.RecordingExecutor{}
		b := NewBuilder(rec)

		err := b.CreateSession(context.Background(), "sess", "/work", []RenderedWindow{
			{Name: "shell"},
			{Name: "agent", Command: "claude", Focus: true},
		}, true)
		require.NoError(t, err)

		// Last command should select the focused window
		last := rec.Commands[len(rec.Commands)-1]
		assert.Equal(t, []string{"select-window", "-t", "sess:agent"}, last.Args)
	})

	t.Run("no focus defaults to first", func(t *testing.T) {
		rec := &executil.RecordingExecutor{}
		b := NewBuilder(rec)

		err := b.CreateSession(context.Background(), "sess", "/work", []RenderedWindow{
			{Name: "first"},
			{Name: "second"},
		}, true)
		require.NoError(t, err)

		last := rec.Commands[len(rec.Commands)-1]
		assert.Equal(t, []string{"select-window", "-t", "sess:first"}, last.Args)
	})

	t.Run("no windows returns error", func(t *testing.T) {
		rec := &executil.RecordingExecutor{}
		b := NewBuilder(rec)

		err := b.CreateSession(context.Background(), "sess", "/work", nil, true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least one window")
	})

	t.Run("window without command gets no sh -c", func(t *testing.T) {
		rec := &executil.RecordingExecutor{}
		b := NewBuilder(rec)

		err := b.CreateSession(context.Background(), "sess", "/work", []RenderedWindow{
			{Name: "shell"},
		}, true)
		require.NoError(t, err)

		// new-session should NOT have "--", "sh", "-c" suffix
		assert.Equal(t, []string{"new-session", "-d", "-s", "sess", "-n", "shell", "-c", "/work"}, rec.Commands[0].Args)
	})
}

func TestBuilder_CreateSession_Background(t *testing.T) {
	t.Setenv("TMUX", "")

	rec := &executil.RecordingExecutor{}
	b := NewBuilder(rec)

	err := b.CreateSession(context.Background(), "sess", "/work", []RenderedWindow{
		{Name: "agent", Command: "claude", Focus: true},
	}, true)
	require.NoError(t, err)

	// Should NOT have attach-session or switch-client
	for _, cmd := range rec.Commands {
		assert.NotContains(t, cmd.Args, "attach-session")
		assert.NotContains(t, cmd.Args, "switch-client")
	}
}

func TestBuilder_AttachOrSwitch(t *testing.T) {
	t.Run("inside tmux uses switch-client", func(t *testing.T) {
		orig := insideTmux
		insideTmux = func() bool { return true }
		defer func() { insideTmux = orig }()

		rec := &executil.RecordingExecutor{}
		b := NewBuilder(rec)

		err := b.AttachOrSwitch(context.Background(), "sess")
		require.NoError(t, err)

		require.Len(t, rec.Commands, 1)
		assert.Equal(t, []string{"switch-client", "-t", "sess"}, rec.Commands[0].Args)
	})

	t.Run("outside tmux uses attach-session", func(t *testing.T) {
		orig := insideTmux
		insideTmux = func() bool { return false }
		defer func() { insideTmux = orig }()

		rec := &executil.RecordingExecutor{}
		b := NewBuilder(rec)

		err := b.AttachOrSwitch(context.Background(), "sess")
		require.NoError(t, err)

		require.Len(t, rec.Commands, 1)
		assert.Equal(t, []string{"attach-session", "-t", "sess"}, rec.Commands[0].Args)
	})
}

func TestBuilder_OpenSession(t *testing.T) {
	t.Run("session exists attaches", func(t *testing.T) {
		orig := insideTmux
		insideTmux = func() bool { return false }
		defer func() { insideTmux = orig }()

		rec := &executil.RecordingExecutor{}
		b := NewBuilder(rec)

		err := b.OpenSession(context.Background(), "sess", "/work", []RenderedWindow{
			{Name: "agent", Focus: true},
		}, false, "")
		require.NoError(t, err)

		// has-session succeeds, so should attach
		require.Len(t, rec.Commands, 2)
		assert.Equal(t, []string{"has-session", "-t", "sess"}, rec.Commands[0].Args)
		assert.Equal(t, []string{"attach-session", "-t", "sess"}, rec.Commands[1].Args)
	})

	t.Run("session exists with target window", func(t *testing.T) {
		orig := insideTmux
		insideTmux = func() bool { return false }
		defer func() { insideTmux = orig }()

		rec := &executil.RecordingExecutor{}
		b := NewBuilder(rec)

		err := b.OpenSession(context.Background(), "sess", "/work", []RenderedWindow{
			{Name: "agent", Focus: true},
		}, false, "editor")
		require.NoError(t, err)

		// has-session, select-window, attach
		require.Len(t, rec.Commands, 3)
		assert.Equal(t, []string{"has-session", "-t", "sess"}, rec.Commands[0].Args)
		assert.Equal(t, []string{"select-window", "-t", "sess:editor"}, rec.Commands[1].Args)
		assert.Equal(t, []string{"attach-session", "-t", "sess"}, rec.Commands[2].Args)
	})

	t.Run("session exists background is noop", func(t *testing.T) {
		rec := &executil.RecordingExecutor{}
		b := NewBuilder(rec)

		err := b.OpenSession(context.Background(), "sess", "/work", []RenderedWindow{
			{Name: "agent", Focus: true},
		}, true, "")
		require.NoError(t, err)

		// has-session only, no attach
		require.Len(t, rec.Commands, 1)
		assert.Equal(t, []string{"has-session", "-t", "sess"}, rec.Commands[0].Args)
	})

	t.Run("session not exists creates", func(t *testing.T) {
		rec := &executil.RecordingExecutor{
			Errors: map[string]error{"tmux": fmt.Errorf("no session")},
		}
		b := NewBuilder(rec)

		// HasSession will fail, then CreateSession calls will also fail due to
		// the blanket error on "tmux". Test just the flow.
		err := b.OpenSession(context.Background(), "sess", "/work", []RenderedWindow{
			{Name: "agent", Focus: true},
		}, true, "")
		require.Error(t, err)

		// First command should be has-session (fails), second should be new-session
		require.GreaterOrEqual(t, len(rec.Commands), 2)
		assert.Equal(t, []string{"has-session", "-t", "sess"}, rec.Commands[0].Args)
		assert.Contains(t, rec.Commands[1].Args, "new-session")
	})
}
