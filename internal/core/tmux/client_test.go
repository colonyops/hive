package tmux

import (
	"context"
	"fmt"
	"testing"

	"github.com/colonyops/hive/pkg/executil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_HasSession(t *testing.T) {
	t.Run("exists", func(t *testing.T) {
		rec := &executil.RecordingExecutor{}
		c := New(rec)

		assert.True(t, c.HasSession(context.Background(), "my-session"))
		require.Len(t, rec.Commands, 1)
		assert.Equal(t, "tmux", rec.Commands[0].Cmd)
		assert.Equal(t, []string{"has-session", "-t", "my-session"}, rec.Commands[0].Args)
	})

	t.Run("not exists", func(t *testing.T) {
		rec := &executil.RecordingExecutor{
			Errors: map[string]error{"tmux": fmt.Errorf("session not found")},
		}
		c := New(rec)

		assert.False(t, c.HasSession(context.Background(), "missing"))
	})
}

func TestClient_CreateSession(t *testing.T) {
	t.Run("single window", func(t *testing.T) {
		rec := &executil.RecordingExecutor{}
		c := New(rec)

		err := c.CreateSession(context.Background(), "sess", "/work", []RenderedWindow{
			{Name: "agent", Command: "claude", Focus: true},
		}, true)
		require.NoError(t, err)

		require.Len(t, rec.Commands, 2) // new-session + select-window
		assert.Equal(t, []string{"new-session", "-d", "-s", "sess", "-n", "agent", "-c", "/work", "--", "sh", "-c", "claude"}, rec.Commands[0].Args)
		assert.Equal(t, []string{"select-window", "-t", "sess:agent"}, rec.Commands[1].Args)
	})

	t.Run("two windows", func(t *testing.T) {
		rec := &executil.RecordingExecutor{}
		c := New(rec)

		err := c.CreateSession(context.Background(), "sess", "/work", []RenderedWindow{
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
		c := New(rec)

		err := c.CreateSession(context.Background(), "sess", "/work", []RenderedWindow{
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
		c := New(rec)

		err := c.CreateSession(context.Background(), "sess", "/work", []RenderedWindow{
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
		c := New(rec)

		err := c.CreateSession(context.Background(), "sess", "/work", []RenderedWindow{
			{Name: "first"},
			{Name: "second"},
		}, true)
		require.NoError(t, err)

		last := rec.Commands[len(rec.Commands)-1]
		assert.Equal(t, []string{"select-window", "-t", "sess:first"}, last.Args)
	})

	t.Run("no windows returns error", func(t *testing.T) {
		rec := &executil.RecordingExecutor{}
		c := New(rec)

		err := c.CreateSession(context.Background(), "sess", "/work", nil, true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least one window")
	})

	t.Run("window without command gets no sh -c", func(t *testing.T) {
		rec := &executil.RecordingExecutor{}
		c := New(rec)

		err := c.CreateSession(context.Background(), "sess", "/work", []RenderedWindow{
			{Name: "shell"},
		}, true)
		require.NoError(t, err)

		// new-session should NOT have "--", "sh", "-c" suffix
		assert.Equal(t, []string{"new-session", "-d", "-s", "sess", "-n", "shell", "-c", "/work"}, rec.Commands[0].Args)
	})
}

func TestClient_CreateSession_Background(t *testing.T) {
	t.Setenv("TMUX", "")

	rec := &executil.RecordingExecutor{}
	c := New(rec)

	err := c.CreateSession(context.Background(), "sess", "/work", []RenderedWindow{
		{Name: "agent", Command: "claude", Focus: true},
	}, true)
	require.NoError(t, err)

	// Should NOT have attach-session or switch-client
	for _, cmd := range rec.Commands {
		assert.NotContains(t, cmd.Args, "attach-session")
		assert.NotContains(t, cmd.Args, "switch-client")
	}
}

func TestClient_AttachOrSwitch(t *testing.T) {
	t.Run("inside tmux uses switch-client", func(t *testing.T) {
		orig := insideTmux
		insideTmux = func() bool { return true }
		defer func() { insideTmux = orig }()

		rec := &executil.RecordingExecutor{}
		c := New(rec)

		err := c.AttachOrSwitch(context.Background(), "sess")
		require.NoError(t, err)

		require.Len(t, rec.Commands, 1)
		assert.Equal(t, []string{"switch-client", "-t", "sess"}, rec.Commands[0].Args)
	})

	t.Run("outside tmux uses attach-session", func(t *testing.T) {
		orig := insideTmux
		insideTmux = func() bool { return false }
		defer func() { insideTmux = orig }()

		rec := &executil.RecordingExecutor{}
		c := New(rec)

		err := c.AttachOrSwitch(context.Background(), "sess")
		require.NoError(t, err)

		require.Len(t, rec.Commands, 1)
		assert.Equal(t, []string{"attach-session", "-t", "sess"}, rec.Commands[0].Args)
	})
}

func TestClient_OpenSession(t *testing.T) {
	t.Run("session exists attaches", func(t *testing.T) {
		orig := insideTmux
		insideTmux = func() bool { return false }
		defer func() { insideTmux = orig }()

		rec := &executil.RecordingExecutor{}
		c := New(rec)

		err := c.OpenSession(context.Background(), "sess", "/work", []RenderedWindow{
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
		c := New(rec)

		err := c.OpenSession(context.Background(), "sess", "/work", []RenderedWindow{
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
		c := New(rec)

		err := c.OpenSession(context.Background(), "sess", "/work", []RenderedWindow{
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
		c := New(rec)

		// HasSession will fail, then CreateSession calls will also fail due to
		// the blanket error on "tmux". Test just the flow.
		err := c.OpenSession(context.Background(), "sess", "/work", []RenderedWindow{
			{Name: "agent", Focus: true},
		}, true, "")
		require.Error(t, err)

		// First command should be has-session (fails), second should be new-session
		require.GreaterOrEqual(t, len(rec.Commands), 2)
		assert.Equal(t, []string{"has-session", "-t", "sess"}, rec.Commands[0].Args)
		assert.Contains(t, rec.Commands[1].Args, "new-session")
	})
}

func TestClient_AddPanes(t *testing.T) {
	t.Run("vertical pane", func(t *testing.T) {
		rec := &executil.RecordingExecutor{}
		c := New(rec)

		err := c.CreateSession(context.Background(), "sess", "/work", []RenderedWindow{
			{Name: "agent", Command: "claude", Panes: []RenderedPane{
				{Command: "tail -f dev.log"},
			}},
		}, true)
		require.NoError(t, err)

		// new-session, split-window, select-window
		require.Len(t, rec.Commands, 3)
		splitArgs := rec.Commands[1].Args
		assert.Equal(t, []string{"split-window", "-t", "sess:agent", "-c", "/work", "--", "sh", "-c", "tail -f dev.log"}, splitArgs)
	})

	t.Run("horizontal pane", func(t *testing.T) {
		rec := &executil.RecordingExecutor{}
		c := New(rec)

		err := c.CreateSession(context.Background(), "sess", "/work", []RenderedWindow{
			{Name: "agent", Command: "claude", Panes: []RenderedPane{
				{Command: "watch git status", Horizontal: true},
			}},
		}, true)
		require.NoError(t, err)

		splitArgs := rec.Commands[1].Args
		assert.Equal(t, []string{"split-window", "-t", "sess:agent", "-h", "-c", "/work", "--", "sh", "-c", "watch git status"}, splitArgs)
	})

	t.Run("focused pane calls select-pane", func(t *testing.T) {
		rec := &executil.RecordingExecutor{}
		c := New(rec)

		err := c.CreateSession(context.Background(), "sess", "/work", []RenderedWindow{
			{Name: "agent", Command: "claude", Panes: []RenderedPane{
				{Command: "tail -f dev.log", Focus: true},
			}},
		}, true)
		require.NoError(t, err)

		// new-session, split-window, select-pane, select-window
		require.Len(t, rec.Commands, 4)
		assert.Equal(t, []string{"select-pane", "-t", "sess:agent.1"}, rec.Commands[2].Args)
	})

	t.Run("explicit pane dir used over session workDir", func(t *testing.T) {
		rec := &executil.RecordingExecutor{}
		c := New(rec)

		err := c.CreateSession(context.Background(), "sess", "/work", []RenderedWindow{
			{Name: "agent", Command: "claude", Panes: []RenderedPane{
				{Command: "ls", Dir: "/custom"},
			}},
		}, true)
		require.NoError(t, err)

		splitArgs := rec.Commands[1].Args
		assert.Contains(t, splitArgs, "/custom")
	})

	t.Run("empty pane dir inherits window dir", func(t *testing.T) {
		rec := &executil.RecordingExecutor{}
		c := New(rec)

		err := c.CreateSession(context.Background(), "sess", "/work", []RenderedWindow{
			{Name: "agent", Command: "claude", Dir: "/window-dir", Panes: []RenderedPane{
				{Command: "tail -f app.log"},
			}},
		}, true)
		require.NoError(t, err)

		splitArgs := rec.Commands[1].Args
		assert.Contains(t, splitArgs, "/window-dir")
		assert.NotContains(t, splitArgs, "/work")
	})

	t.Run("no panes produces no split-window", func(t *testing.T) {
		rec := &executil.RecordingExecutor{}
		c := New(rec)

		err := c.CreateSession(context.Background(), "sess", "/work", []RenderedWindow{
			{Name: "agent", Command: "claude"},
		}, true)
		require.NoError(t, err)

		for _, cmd := range rec.Commands {
			assert.NotContains(t, cmd.Args, "split-window")
		}
	})

	t.Run("two panes second focused", func(t *testing.T) {
		rec := &executil.RecordingExecutor{}
		c := New(rec)

		err := c.CreateSession(context.Background(), "sess", "/work", []RenderedWindow{
			{Name: "agent", Command: "claude", Panes: []RenderedPane{
				{Command: "tail -f dev.log"},
				{Command: "watch git status", Focus: true},
			}},
		}, true)
		require.NoError(t, err)

		// new-session, split-window x2, select-pane (idx 2), select-window
		require.Len(t, rec.Commands, 5)
		assert.Equal(t, []string{"split-window", "-t", "sess:agent", "-c", "/work", "--", "sh", "-c", "tail -f dev.log"}, rec.Commands[1].Args)
		assert.Equal(t, []string{"split-window", "-t", "sess:agent", "-c", "/work", "--", "sh", "-c", "watch git status"}, rec.Commands[2].Args)
		assert.Equal(t, []string{"select-pane", "-t", "sess:agent.2"}, rec.Commands[3].Args)
	})
}

func TestClient_AddWindows(t *testing.T) {
	t.Run("adds windows without focus", func(t *testing.T) {
		rec := &executil.RecordingExecutor{}
		c := New(rec)

		err := c.AddWindows(context.Background(), "sess", "/work", []RenderedWindow{
			{Name: "w1", Command: "claude"},
			{Name: "w2"},
		})
		require.NoError(t, err)

		require.Len(t, rec.Commands, 2)
		assert.Equal(t, []string{"new-window", "-t", "sess", "-n", "w1", "-c", "/work", "--", "sh", "-c", "claude"}, rec.Commands[0].Args)
		assert.Equal(t, []string{"new-window", "-t", "sess", "-n", "w2", "-c", "/work"}, rec.Commands[1].Args)
	})

	t.Run("selects focused window", func(t *testing.T) {
		rec := &executil.RecordingExecutor{}
		c := New(rec)

		err := c.AddWindows(context.Background(), "sess", "/work", []RenderedWindow{
			{Name: "w1"},
			{Name: "w2", Focus: true},
		})
		require.NoError(t, err)

		// 2 new-window + 1 select-window
		require.Len(t, rec.Commands, 3)
		assert.Equal(t, []string{"select-window", "-t", "sess:w2"}, rec.Commands[2].Args)
	})

	t.Run("window dir overrides session dir", func(t *testing.T) {
		rec := &executil.RecordingExecutor{}
		c := New(rec)

		err := c.AddWindows(context.Background(), "sess", "/work", []RenderedWindow{
			{Name: "w1", Dir: "/custom"},
		})
		require.NoError(t, err)

		require.Len(t, rec.Commands, 1)
		assert.Contains(t, rec.Commands[0].Args, "/custom")
	})

	t.Run("window with panes creates splits", func(t *testing.T) {
		rec := &executil.RecordingExecutor{}
		c := New(rec)

		err := c.AddWindows(context.Background(), "sess", "/work", []RenderedWindow{
			{Name: "w1", Command: "claude", Panes: []RenderedPane{
				{Command: "tail -f dev.log"},
			}},
		})
		require.NoError(t, err)

		// new-window + split-window
		require.Len(t, rec.Commands, 2)
		assert.Equal(t, []string{"new-window", "-t", "sess", "-n", "w1", "-c", "/work", "--", "sh", "-c", "claude"}, rec.Commands[0].Args)
		assert.Equal(t, []string{"split-window", "-t", "sess:w1", "-c", "/work", "--", "sh", "-c", "tail -f dev.log"}, rec.Commands[1].Args)
	})
}
