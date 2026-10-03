//go:build !server

package tmuxcc

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeSessionCommands is a self-contained one-shot fake for this file's
// tests, separate from fakeTmuxCommands in manager_test.go: it answers
// new-session, capture-pane, list-sessions and has-session, which that fake
// does not need to know about.
type fakeSessionCommands struct {
	calls   [][]string
	envs    [][]string
	present map[string]bool
	capture []string
	names   []string
	failure error
}

func (f *fakeSessionCommands) run(_ context.Context, _ string, env []string, args ...string) ([]string, error) {
	f.calls = append(f.calls, args)
	f.envs = append(f.envs, env)
	if len(args) == 0 {
		return nil, nil
	}
	switch args[0] {
	case "has-session":
		if f.present[args[2]] {
			return nil, nil
		}
		return nil, errors.New("can't find session")
	case "new-session":
		if f.failure != nil {
			return nil, f.failure
		}
		if f.present == nil {
			f.present = map[string]bool{}
		}
		f.present[args[3]] = true
		return nil, nil
	case "capture-pane":
		return f.capture, f.failure
	case "list-sessions":
		return f.names, f.failure
	}
	return nil, nil
}

func TestManagerNewSessionCreatesADetachedSession(t *testing.T) {
	t.Parallel()

	cmds := &fakeSessionCommands{}
	m := newTestManager(t, nil, ManagerOptions{runTmux: cmds.run})

	require.NoError(t, m.NewSession(t.Context(), "agentws-1", "/work/dir", "claude --resume 'abc'", nil))
	require.Equal(t, [][]string{
		{"has-session", "-t", "agentws-1"},
		{"new-session", "-d", "-s", "agentws-1", "-c", "/work/dir", "--", resolveLoginShell(), "-l", "-c", "claude --resume 'abc'"},
	}, cmds.calls)
}

func TestManagerNewSessionOpensAnInteractiveShellForAnEmptyCommand(t *testing.T) {
	t.Parallel()

	cmds := &fakeSessionCommands{}
	m := newTestManager(t, nil, ManagerOptions{runTmux: cmds.run})

	require.NoError(t, m.NewSession(t.Context(), "agentws-1", "/work/dir", "", nil))
	require.Equal(t, [][]string{
		{"has-session", "-t", "agentws-1"},
		{"new-session", "-d", "-s", "agentws-1", "-c", "/work/dir", "--", resolveLoginShell(), "-l"},
	}, cmds.calls)
}

// A session's pane inherits the environment of the client that created it, so
// the resolved PATH has to be on the new-session command itself: the login
// shell it execs is non-interactive and never reads the file the PATH is
// usually set in (ADR tmux-runs-in-the-resolved-environment).
func TestManagerNewSessionRunsWithTheResolvedEnvironment(t *testing.T) {
	t.Parallel()

	cmds := &fakeSessionCommands{}
	resolved := []string{"PATH=/opt/homebrew/bin:/usr/bin", "HOME=/Users/agent"}
	m := newTestManager(t, nil, ManagerOptions{
		runTmux: cmds.run,
		Environ: func(context.Context) []string { return resolved },
	})

	require.NoError(t, m.NewSession(t.Context(), "agentws-1", "/work/dir", "claude", nil))
	require.Len(t, cmds.envs, 2)
	for _, env := range cmds.envs {
		require.Equal(t, resolved, env)
	}
}

func TestManagerNewSessionSetsSessionEnvironment(t *testing.T) {
	t.Parallel()

	cmds := &fakeSessionCommands{}
	m := newTestManager(t, nil, ManagerOptions{runTmux: cmds.run})

	env := []string{"HIVE_AGENT_SESSION=7", "HIVE_AGENT_WORKSPACE=/work/dir"}
	require.NoError(t, m.NewSession(t.Context(), "agentws-7", "/work/dir", "claude", env))
	require.Equal(t, [][]string{
		{"has-session", "-t", "agentws-7"},
		{
			"new-session", "-d", "-s", "agentws-7", "-c", "/work/dir",
			"-e", "HIVE_AGENT_SESSION=7", "-e", "HIVE_AGENT_WORKSPACE=/work/dir",
			"--", resolveLoginShell(), "-l", "-c", "claude",
		},
	}, cmds.calls)
}

func TestManagerNewSessionRejectsAnExistingName(t *testing.T) {
	t.Parallel()

	cmds := &fakeSessionCommands{present: map[string]bool{"agentws-1": true}}
	m := newTestManager(t, nil, ManagerOptions{runTmux: cmds.run})

	err := m.NewSession(t.Context(), "agentws-1", "/work/dir", "claude", nil)
	require.ErrorIs(t, err, ErrSessionExists)
}

func TestManagerNewSessionRejectsAnEmptyName(t *testing.T) {
	t.Parallel()

	cmds := &fakeSessionCommands{}
	m := newTestManager(t, nil, ManagerOptions{runTmux: cmds.run})

	err := m.NewSession(t.Context(), "", "/work/dir", "claude", nil)
	require.ErrorIs(t, err, ErrInvalidName)
	require.Empty(t, cmds.calls, "an invalid name is rejected before any tmux call")
}

func TestManagerCapturePaneJoinsWrappedLines(t *testing.T) {
	t.Parallel()

	cmds := &fakeSessionCommands{capture: []string{"line one", "line two"}}
	m := newTestManager(t, nil, ManagerOptions{runTmux: cmds.run})

	screen, err := m.CapturePane(t.Context(), "agentws-1")
	require.NoError(t, err)
	require.Equal(t, "line one\nline two", screen)
	require.Equal(t, [][]string{{"capture-pane", "-t", "agentws-1", "-p", "-J"}}, cmds.calls)
}

func TestManagerSessionNamesFiltersByPrefix(t *testing.T) {
	t.Parallel()

	cmds := &fakeSessionCommands{names: []string{"agentws-1", "agentws-2", "hive-demo"}}
	m := newTestManager(t, nil, ManagerOptions{runTmux: cmds.run})

	names, err := m.SessionNames(t.Context(), "agentws-")
	require.NoError(t, err)
	require.Equal(t, []string{"agentws-1", "agentws-2"}, names)
}

func TestManagerSessionNamesTreatsADeadServerAsEmpty(t *testing.T) {
	t.Parallel()

	cmds := &fakeSessionCommands{failure: errors.New("no server running")}
	m := newTestManager(t, nil, ManagerOptions{runTmux: cmds.run})

	names, err := m.SessionNames(t.Context(), "agentws-")
	require.NoError(t, err)
	require.Empty(t, names)
}
