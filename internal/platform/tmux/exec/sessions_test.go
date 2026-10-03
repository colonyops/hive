package tmuxexec

import (
	"context"
	"testing"

	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateSessionCreatesPanesAndFocus(t *testing.T) {
	runner := &fakeRunner{}
	client := New(runner, zerolog.Nop())
	client.getenv = func(string) string { return "" }
	spec := multiplexer.SessionSpec{
		Target:           multiplexer.Target{Session: "work"},
		WorkingDirectory: "/repo",
		Background:       true,
		Windows: []multiplexer.WindowSpec{
			{Name: "shell", Panes: []multiplexer.PaneSpec{{Command: "first"}, {Command: "second", Split: multiplexer.SplitHorizontal, Size: "40%"}}},
			{Name: "agent", Command: "pi", WorkingDirectory: "/repo/sub", Focus: true},
		},
	}

	require.NoError(t, client.CreateSession(context.Background(), spec))
	assert.Equal(t, []string{"new-session", "-d", "-s", "work", "-n", "shell", "-c", "/repo", "--", "sh", "-c", "first"}, runner.calls[0].args)
	assert.True(t, hasCall(runner.calls, []string{"split-window", "-t", "work:shell", "-h", "-l", "40%", "-c", "/repo", "--", "sh", "-c", "second"}))
	assert.True(t, hasCall(runner.calls, []string{"new-window", "-t", "work", "-n", "agent", "-c", "/repo/sub", "--", "sh", "-c", "pi"}))
	assert.True(t, hasCall(runner.calls, []string{"select-window", "-t", "work:agent"}))
}

func TestCreateSessionCleansPartialSession(t *testing.T) {
	runner := &fakeRunner{results: []runnerResult{
		{},                    // new-session
		{},                    // tag
		{},                    // first hook
		{},                    // second hook
		{err: assert.AnError}, // split
		{},                    // cleanup
	}}
	client := New(runner, zerolog.Nop())
	err := client.CreateSession(context.Background(), multiplexer.SessionSpec{
		Target: multiplexer.Target{Session: "work"}, Background: true,
		Windows: []multiplexer.WindowSpec{{Name: "one", Panes: []multiplexer.PaneSpec{{}, {}}}},
	})
	require.ErrorIs(t, err, assert.AnError)
	assert.Equal(t, []string{"kill-session", "-t", "work"}, runner.calls[len(runner.calls)-1].args)
}

func TestAttachOrSwitchUsesInteractiveStreamsOutsideTmux(t *testing.T) {
	runner := &fakeRunner{}
	client := New(runner, zerolog.Nop())
	client.getenv = func(string) string { return "" }
	target := multiplexer.Target{Session: "work", Window: "2", Pane: "1"}

	require.NoError(t, client.AttachOrSwitch(context.Background(), target, multiplexer.AttachStreams{}))
	assert.Equal(t, []string{"select-window", "-t", "work:2"}, runner.calls[0].args)
	assert.Equal(t, []string{"select-pane", "-t", "work:2.1"}, runner.calls[1].args)
	assert.Equal(t, "interactive", runner.calls[2].mode)
	assert.Equal(t, []string{"attach-session", "-t", "work"}, runner.calls[2].args)
}

func TestOpenExistingSessionModes(t *testing.T) {
	t.Run("background does not attach", func(t *testing.T) {
		runner := &fakeRunner{}
		client := New(runner, zerolog.Nop())
		spec := multiplexer.SessionSpec{Target: multiplexer.Target{Session: "work"}, Background: true}

		require.NoError(t, client.OpenSession(context.Background(), spec, multiplexer.Target{}))
		require.Len(t, runner.calls, 1)
		assert.Equal(t, []string{"has-session", "-t", "work"}, runner.calls[0].args)
	})

	t.Run("inside tmux switches then selects qualified pane", func(t *testing.T) {
		runner := &fakeRunner{}
		client := New(runner, zerolog.Nop())
		client.getenv = func(string) string { return "/tmp/tmux" }
		spec := multiplexer.SessionSpec{Target: multiplexer.Target{Session: "work"}}
		selection := multiplexer.Target{Session: "work", Window: "2", Pane: "1"}

		require.NoError(t, client.OpenSession(context.Background(), spec, selection))
		assert.Equal(t, []string{"has-session", "-t", "work"}, runner.calls[0].args)
		assert.Equal(t, []string{"switch-client", "-t", "work"}, runner.calls[1].args)
		assert.Equal(t, []string{"select-window", "-t", "work:2"}, runner.calls[2].args)
		assert.Equal(t, []string{"select-pane", "-t", "work:2.1"}, runner.calls[3].args)
	})
}

func TestAttachOrSwitchInsideTmux(t *testing.T) {
	runner := &fakeRunner{}
	client := New(runner, zerolog.Nop())
	client.getenv = func(string) string { return "/tmp/tmux" }

	require.NoError(t, client.AttachOrSwitch(context.Background(), multiplexer.Target{Session: "work"}, multiplexer.AttachStreams{}))
	require.Len(t, runner.calls, 1)
	assert.Equal(t, []string{"switch-client", "-t", "work"}, runner.calls[0].args)
}

func TestAddWindowsPreservesDirectoriesAndFocus(t *testing.T) {
	runner := &fakeRunner{}
	client := New(runner, zerolog.Nop())
	windows := []multiplexer.WindowSpec{
		{Name: "shell", Command: "bash", WorkingDirectory: "/repo/shell"},
		{Name: "agent", WorkingDirectory: "/repo/agent", Focus: true, Panes: []multiplexer.PaneSpec{{Command: "pi"}, {Command: "tail", WorkingDirectory: "/repo/logs"}}},
	}

	require.NoError(t, client.AddWindows(context.Background(), multiplexer.Target{Session: "work"}, windows))
	assert.True(t, hasCall(runner.calls, []string{"new-window", "-t", "work", "-n", "shell", "-c", "/repo/shell", "--", "sh", "-c", "bash"}))
	assert.True(t, hasCall(runner.calls, []string{"new-window", "-t", "work", "-n", "agent", "-c", "/repo/agent", "--", "sh", "-c", "pi"}))
	assert.True(t, hasCall(runner.calls, []string{"split-window", "-t", "work:agent", "-v", "-c", "/repo/logs", "--", "sh", "-c", "tail"}))
	assert.True(t, hasCall(runner.calls, []string{"select-window", "-t", "work:agent"}))
}

func hasCall(calls []runnerCall, want []string) bool {
	for _, call := range calls {
		if assert.ObjectsAreEqual(want, call.args) {
			return true
		}
	}
	return false
}
