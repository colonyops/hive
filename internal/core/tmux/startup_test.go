package tmux

import (
	"context"
	"io"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// paneExec fakes tmux for startup checks: it hands out pane ids for commands
// run with -P and answers list-panes and capture-pane from scripted handlers.
type paneExec struct {
	mu        sync.Mutex
	calls     [][]string
	nextPane  int
	listPanes func(call int) string
	listCalls int
	capture   string
}

func (e *paneExec) Run(_ context.Context, _ string, args ...string) ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls = append(e.calls, args)

	switch args[0] {
	case "new-session", "new-window", "split-window":
		if slices.Contains(args, "-P") {
			id := e.nextPane
			e.nextPane++
			return []byte("%" + strconv.Itoa(id) + "\n"), nil
		}
	case "list-panes":
		e.listCalls++
		if e.listPanes != nil {
			return []byte(e.listPanes(e.listCalls)), nil
		}
	case "capture-pane":
		return []byte(e.capture), nil
	}
	return nil, nil
}

func (e *paneExec) RunDir(ctx context.Context, _, cmd string, args ...string) ([]byte, error) {
	return e.Run(ctx, cmd, args...)
}

func (e *paneExec) RunStream(ctx context.Context, _, _ io.Writer, cmd string, args ...string) error {
	_, err := e.Run(ctx, cmd, args...)
	return err
}

func (e *paneExec) RunDirStream(ctx context.Context, _ string, _, _ io.Writer, cmd string, args ...string) error {
	_, err := e.Run(ctx, cmd, args...)
	return err
}

func (e *paneExec) subcommands() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]string, len(e.calls))
	for i, c := range e.calls {
		out[i] = c[0]
	}
	return out
}

func (e *paneExec) find(sub string) [][]string {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out [][]string
	for _, c := range e.calls {
		if c[0] == sub {
			out = append(out, c)
		}
	}
	return out
}

func newStartupClient(exec *paneExec, grace time.Duration) *Client {
	c := New(exec, nopLog)
	c.startupGrace = grace
	return c
}

func TestClient_CreateSession_CommandExitsAtStartup(t *testing.T) {
	exec := &paneExec{
		listPanes: func(int) string { return "%0 1 127\n%1 0 \n" },
		capture:   "sh: pi: command not found\n\nPane is dead (status 127, Fri Sep 25 15:19:41 2026)\n",
	}
	c := newStartupClient(exec, 0)

	err := c.CreateSession(context.Background(), "sess", "/work", []RenderedWindow{
		{Name: "pi", Command: "pi --yolo", Focus: true},
		{Name: "shell"},
	}, true)
	require.Error(t, err)

	require.ErrorIs(t, err, ErrCommandExited)
	var exitErr *CommandExitedError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, "sess", exitErr.Session)
	assert.Equal(t, "pi", exitErr.Window)
	assert.Equal(t, "pi --yolo", exitErr.Command)
	assert.Equal(t, 127, exitErr.Status)
	assert.True(t, exitErr.NotFound())
	assert.Equal(t, "sh: pi: command not found", exitErr.Output)
	assert.Contains(t, err.Error(), "command not found")

	assert.Equal(t, [][]string{{"kill-session", "-t", "=sess"}}, exec.find("kill-session"))
	assert.Empty(t, exec.find("select-window"), "must not select a window in a failed session")
}

func TestClient_CreateSession_DeadSplitPaneInLaterWindow(t *testing.T) {
	// %0 agent, %1 editor initial pane (no command), %2 editor split running nvim.
	exec := &paneExec{
		listPanes: func(int) string { return "%0 0 \n%1 0 \n%2 1 1\n" },
	}
	c := newStartupClient(exec, 0)

	err := c.CreateSession(context.Background(), "sess", "/work", []RenderedWindow{
		{Name: "agent", Command: "claude"},
		{Name: "editor", Panes: []RenderedPane{{}, {Command: "nvim ."}}},
	}, true)

	var exitErr *CommandExitedError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, "editor", exitErr.Window)
	assert.Equal(t, "nvim .", exitErr.Command)
	assert.Equal(t, 1, exitErr.Status)
	assert.False(t, exitErr.NotFound())

	// The editor window has no initial command but its split does, so the
	// window still gets remain-on-exit before the split is created.
	assert.Contains(t, exec.find("set-option"), []string{"set-option", "-w", "-t", "%1", "remain-on-exit", "on"})
}

func TestClient_CreateSession_HealthyStartupClearsRemainOnExit(t *testing.T) {
	exec := &paneExec{
		listPanes: func(int) string { return "%0 0 \n%1 0 \n" },
	}
	c := newStartupClient(exec, 0)

	err := c.CreateSession(context.Background(), "sess", "/work", []RenderedWindow{
		{Name: "agent", Command: "claude", Focus: true},
		{Name: "shell"},
	}, true)
	require.NoError(t, err)

	assert.Equal(t, []string{"respawn-pane", "-k", "-t", "%0", "-c", "/work", "--", "sh", "-c", "claude"}, exec.find("respawn-pane")[0])
	assert.Contains(t, exec.find("set-option"), []string{"set-option", "-w", "-u", "-t", "%0", "remain-on-exit"})
	assert.Empty(t, exec.find("kill-session"))

	subs := exec.subcommands()
	assert.Equal(t, "select-window", subs[len(subs)-1], "startup check runs before the focused window is selected")
}

func TestClient_CreateSession_WatchesUntilGraceEnds(t *testing.T) {
	// The pane is still alive on the first poll and dead on the second, as
	// tmux reports right after respawn-pane for a command that fails at once.
	exec := &paneExec{
		listPanes: func(call int) string {
			if call == 1 {
				return "%0 0 \n"
			}
			return "%0 1 127\n"
		},
	}
	c := newStartupClient(exec, time.Second)

	err := c.CreateSession(context.Background(), "sess", "/work", []RenderedWindow{
		{Name: "agent", Command: "missing-agent"},
	}, true)
	require.ErrorIs(t, err, ErrCommandExited)
	assert.Equal(t, 2, exec.listCalls)
}

func TestClient_CreateSession_SkipsWatchWithoutCommands(t *testing.T) {
	exec := &paneExec{}
	c := newStartupClient(exec, time.Hour)

	err := c.CreateSession(context.Background(), "sess", "/work", []RenderedWindow{{Name: "shell"}}, true)
	require.NoError(t, err)
	assert.Zero(t, exec.listCalls)
	for _, call := range exec.find("set-option") {
		assert.NotContains(t, call, "remain-on-exit", "no remain-on-exit for windows without commands")
	}
}

func TestCommandExitedError_Message(t *testing.T) {
	err := &CommandExitedError{Session: "s", Window: "w", Command: "x", Status: -1}
	assert.Equal(t, `tmux session "s": window "w": command "x" exited during startup`, err.Error())
	assert.ErrorIs(t, err, ErrCommandExited)
}
