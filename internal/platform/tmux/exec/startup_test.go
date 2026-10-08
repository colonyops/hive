package tmuxexec

import (
	"context"
	"io"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type startupRunner struct {
	mu        sync.Mutex
	calls     [][]string
	nextPane  int
	listPanes func(call int) string
	listCalls int
	capture   string
}

func (r *startupRunner) Available() bool { return true }

func (r *startupRunner) Capture(_ context.Context, args ...string) ([]byte, []byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, slices.Clone(args))

	switch args[0] {
	case "new-session", "new-window", "split-window":
		if slices.Contains(args, "-P") {
			id := r.nextPane
			r.nextPane++
			return []byte("%" + strconv.Itoa(id) + "\n"), nil, nil
		}
	case "list-panes":
		r.listCalls++
		if r.listPanes != nil {
			return []byte(r.listPanes(r.listCalls)), nil, nil
		}
	case "capture-pane":
		return []byte(r.capture), nil, nil
	}
	return nil, nil, nil
}

func (r *startupRunner) Input(ctx context.Context, _ io.Reader, args ...string) ([]byte, []byte, error) {
	return r.Capture(ctx, args...)
}

func (r *startupRunner) Interactive(ctx context.Context, _ multiplexer.AttachStreams, args ...string) error {
	_, _, err := r.Capture(ctx, args...)
	return err
}

func (r *startupRunner) find(subcommand string) [][]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var matches [][]string
	for _, call := range r.calls {
		if call[0] == subcommand {
			matches = append(matches, call)
		}
	}
	return matches
}

func newStartupClient(runner *startupRunner, grace time.Duration) *Client {
	client := New(zerolog.Nop(), runner)
	client.startupGrace = grace
	return client
}

func TestCreateSessionReportsCommandThatExitsDuringStartup(t *testing.T) {
	runner := &startupRunner{
		listPanes: func(int) string { return "%0 1 127\n%1 0 \n" },
		capture:   "sh: missing-agent: command not found\n\nPane is dead (status 127)\n",
	}
	client := newStartupClient(runner, 0)

	err := client.CreateSession(t.Context(), multiplexer.SessionSpec{
		Target: multiplexer.Target{Session: "work"}, WorkingDirectory: "/repo", Background: true,
		Windows: []multiplexer.WindowSpec{
			{Name: "agent", Command: "missing-agent --prompt hi", Focus: true},
			{Name: "shell"},
		},
	})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrCommandExited)
	var exitErr *CommandExitedError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, "work", exitErr.Session)
	assert.Equal(t, "agent", exitErr.Window)
	assert.Equal(t, "missing-agent --prompt hi", exitErr.Command)
	assert.Equal(t, 127, exitErr.Status)
	assert.True(t, exitErr.NotFound())
	assert.Equal(t, "sh: missing-agent: command not found", exitErr.Output)
	assert.Contains(t, err.Error(), "command not found")
	assert.Equal(t, [][]string{{"kill-session", "-t", "=work"}}, runner.find("kill-session"))
	assert.Empty(t, runner.find("select-window"))
}

func TestCreateSessionReportsFailedSplitCommand(t *testing.T) {
	runner := &startupRunner{listPanes: func(int) string { return "%0 0 \n%1 1 1\n" }}
	client := newStartupClient(runner, 0)

	err := client.CreateSession(t.Context(), multiplexer.SessionSpec{
		Target: multiplexer.Target{Session: "work"}, Background: true,
		Windows: []multiplexer.WindowSpec{{
			Name: "editor",
			Panes: []multiplexer.PaneSpec{
				{},
				{Command: "missing-editor", Split: multiplexer.SplitHorizontal},
			},
		}},
	})

	var exitErr *CommandExitedError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, "editor", exitErr.Window)
	assert.Equal(t, "missing-editor", exitErr.Command)
	assert.Equal(t, 1, exitErr.Status)
	newSession := runner.find("new-session")
	require.Len(t, newSession, 1)
	assert.NotContains(t, newSession[0], "cat", "a commandless first pane must remain an interactive shell")
	assert.Contains(t, runner.find("split-window")[0], "cat")
	assert.Contains(t, runner.find("set-option"), []string{"set-option", "-w", "-t", "%1", "remain-on-exit", "on"})
	assert.Contains(t, runner.find("respawn-pane"), []string{"respawn-pane", "-k", "-t", "%1", "--", "sh", "-c", "missing-editor"})
}

func TestCreateSessionHealthyStartupClearsRemainOnExit(t *testing.T) {
	runner := &startupRunner{listPanes: func(int) string { return "%0 0 \n" }}
	client := newStartupClient(runner, 0)

	err := client.CreateSession(t.Context(), multiplexer.SessionSpec{
		Target: multiplexer.Target{Session: "work"}, WorkingDirectory: "/repo", Background: true,
		Windows: []multiplexer.WindowSpec{{Name: "agent", Command: "agent", Focus: true}},
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"new-session", "-d", "-s", "work", "-n", "agent", "-P", "-F", "#{pane_id}", "-c", "/repo", "--", "cat"}, runner.find("new-session")[0])
	assert.Equal(t, []string{"respawn-pane", "-k", "-t", "%0", "-c", "/repo", "--", "sh", "-c", "agent"}, runner.find("respawn-pane")[0])
	assert.Contains(t, runner.find("set-option"), []string{"set-option", "-w", "-u", "-t", "%0", "remain-on-exit"})
	assert.Empty(t, runner.find("kill-session"))
}

func TestCreateSessionWatchesUntilGraceEnds(t *testing.T) {
	runner := &startupRunner{listPanes: func(call int) string {
		if call == 1 {
			return "%0 0 \n"
		}
		return "%0 1 127\n"
	}}
	client := newStartupClient(runner, 100*time.Millisecond)

	err := client.CreateSession(t.Context(), multiplexer.SessionSpec{
		Target:     multiplexer.Target{Session: "work"},
		Background: true,
		Windows:    []multiplexer.WindowSpec{{Name: "agent", Command: "missing-agent"}},
	})
	require.ErrorIs(t, err, ErrCommandExited)
	assert.Equal(t, 2, runner.listCalls)
}

func TestCreateSessionSkipsStartupWatchWithoutCommands(t *testing.T) {
	runner := &startupRunner{}
	client := newStartupClient(runner, time.Hour)

	err := client.CreateSession(t.Context(), multiplexer.SessionSpec{
		Target:     multiplexer.Target{Session: "work"},
		Background: true,
		Windows:    []multiplexer.WindowSpec{{Name: "shell"}},
	})
	require.NoError(t, err)
	assert.Zero(t, runner.listCalls)
	for _, call := range runner.find("set-option") {
		assert.NotContains(t, call, "remain-on-exit")
	}
}

func TestCommandExitedErrorMessage(t *testing.T) {
	err := &CommandExitedError{Session: "s", Window: "w", Command: "x", Status: -1}
	assert.Equal(t, `tmux session "s": command "x" exited in window "w" during startup`, err.Error())
	assert.ErrorIs(t, err, ErrCommandExited)
}
