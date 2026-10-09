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
	runner := &startupRunner{}
	client := newStartupClient(runner, 0)
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
	assert.Equal(t, []string{"new-session", "-d", "-s", "work", "-n", "shell", "-P", "-F", allocationFormat, "-c", "/repo", "--", "cat", ";", "set-option", "-t", "=work:", launchMarker, "1"}, runner.calls[0])
	assert.Contains(t, runner.calls, []string{"respawn-pane", "-k", "-t", "%0", "-c", "/repo", "--", "sh", "-c", "first"})
	assert.Contains(t, runner.calls, []string{"split-window", "-d", "-t", "@0", "-P", "-F", allocationFormat, "-h", "-l", "40%", "-c", "/repo", "--", "sh", "-c", "cat"})
	assert.Contains(t, runner.calls, []string{"respawn-pane", "-k", "-t", "%1", "-c", "/repo", "--", "sh", "-c", "second"})
	assert.Contains(t, runner.calls, []string{"new-window", "-d", "-t", "=work:", "-n", "agent", "-P", "-F", allocationFormat, "-c", "/repo/sub", "--", "cat"})
	assert.Contains(t, runner.calls, []string{"respawn-pane", "-k", "-t", "%2", "-c", "/repo/sub", "--", "sh", "-c", "pi"})
	assert.Contains(t, runner.calls, []string{"select-window", "-t", "@1"})
}

func TestCreateSessionCleansPartialSession(t *testing.T) {
	runner := &startupRunner{failure: func(_ context.Context, args []string) error {
		if args[0] == "split-window" {
			return assert.AnError
		}
		return nil
	}}
	client := newStartupClient(runner, 0)
	err := client.CreateSession(context.Background(), multiplexer.SessionSpec{
		Target: multiplexer.Target{Session: "work"}, Background: true,
		Windows: []multiplexer.WindowSpec{{Name: "one", Panes: []multiplexer.PaneSpec{{}, {}}}},
	})
	require.ErrorIs(t, err, assert.AnError)
	assert.Equal(t, []string{"kill-session", "-t", "$0"}, runner.calls[len(runner.calls)-1])
}

func TestAttachOrSwitchUsesInteractiveStreamsOutsideTmux(t *testing.T) {
	runner := &fakeRunner{}
	client := New(zerolog.Nop(), runner)
	client.getenv = func(string) string { return "" }
	target := multiplexer.Target{Session: "work", Window: "2", Pane: "1"}

	require.NoError(t, client.AttachOrSwitch(context.Background(), target, multiplexer.AttachStreams{}))
	assert.Equal(t, []string{"select-window", "-t", "=work:2"}, runner.calls[0].args)
	assert.Equal(t, []string{"select-pane", "-t", "=work:2.1"}, runner.calls[1].args)
	assert.Equal(t, "interactive", runner.calls[2].mode)
	assert.Equal(t, []string{"attach-session", "-t", "=work"}, runner.calls[2].args)
}

func TestOpenExistingSessionModes(t *testing.T) {
	t.Run("background does not attach", func(t *testing.T) {
		runner := &fakeRunner{}
		client := New(zerolog.Nop(), runner)
		spec := multiplexer.SessionSpec{Target: multiplexer.Target{Session: "work"}, Background: true}

		result, err := client.OpenSession(context.Background(), spec, multiplexer.Target{})
		require.NoError(t, err)
		assert.False(t, result.Created)
		require.Len(t, runner.calls, 3)
		assert.Equal(t, []string{"has-session", "-t", "=work"}, runner.calls[0].args)
	})

	t.Run("inside tmux switches then selects qualified pane", func(t *testing.T) {
		runner := &fakeRunner{}
		client := New(zerolog.Nop(), runner)
		client.getenv = func(string) string { return "/tmp/tmux" }
		spec := multiplexer.SessionSpec{Target: multiplexer.Target{Session: "work"}}
		selection := multiplexer.Target{Session: "work", Window: "2", Pane: "1"}

		_, err := client.OpenSession(context.Background(), spec, selection)
		require.NoError(t, err)
		assert.Equal(t, []string{"has-session", "-t", "=work"}, runner.calls[0].args)
		assert.Equal(t, []string{"switch-client", "-t", "=work"}, runner.calls[3].args)
		assert.Equal(t, []string{"select-window", "-t", "=work:2"}, runner.calls[4].args)
		assert.Equal(t, []string{"select-pane", "-t", "=work:2.1"}, runner.calls[5].args)
	})
}

func TestAttachOrSwitchInsideTmux(t *testing.T) {
	runner := &fakeRunner{}
	client := New(zerolog.Nop(), runner)
	client.getenv = func(string) string { return "/tmp/tmux" }

	require.NoError(t, client.AttachOrSwitch(context.Background(), multiplexer.Target{Session: "work"}, multiplexer.AttachStreams{}))
	require.Len(t, runner.calls, 1)
	assert.Equal(t, []string{"switch-client", "-t", "=work"}, runner.calls[0].args)
}

func TestAddWindowsPreservesDirectoriesAndFocus(t *testing.T) {
	runner := &startupRunner{}
	client := newStartupClient(runner, 0)
	windows := []multiplexer.WindowSpec{
		{Name: "shell", Command: "bash", WorkingDirectory: "/repo/shell"},
		{Name: "agent", WorkingDirectory: "/repo/agent", Focus: true, Panes: []multiplexer.PaneSpec{{Command: "pi"}, {Command: "tail", WorkingDirectory: "/repo/logs"}}},
	}

	require.NoError(t, client.AddWindows(context.Background(), multiplexer.Target{Session: "work"}, windows))
	assert.Contains(t, runner.calls, []string{"new-window", "-d", "-t", "=work:", "-n", "shell", "-P", "-F", allocationFormat, "-c", "/repo/shell", "--", "cat"})
	assert.Contains(t, runner.calls, []string{"respawn-pane", "-k", "-t", "%0", "-c", "/repo/shell", "--", "sh", "-c", "bash"})
	assert.Contains(t, runner.calls, []string{"new-window", "-d", "-t", "=work:", "-n", "agent", "-P", "-F", allocationFormat, "-c", "/repo/agent", "--", "cat"})
	assert.Contains(t, runner.calls, []string{"respawn-pane", "-k", "-t", "%1", "-c", "/repo/agent", "--", "sh", "-c", "pi"})
	assert.Contains(t, runner.calls, []string{"split-window", "-d", "-t", "@1", "-P", "-F", allocationFormat, "-v", "-c", "/repo/logs", "--", "sh", "-c", "cat"})
	assert.Contains(t, runner.calls, []string{"respawn-pane", "-k", "-t", "%2", "-c", "/repo/logs", "--", "sh", "-c", "tail"})
	assert.Contains(t, runner.calls, []string{"select-window", "-t", "@1"})
}
