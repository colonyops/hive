//go:build !server

package tmuxexec_test

import (
	"context"
	"testing"

	"github.com/colonyops/hive/internal/domain/multiplexer"
	tmuxcc "github.com/colonyops/hive/internal/platform/tmux/control"
	tmuxexec "github.com/colonyops/hive/internal/platform/tmux/exec"
	"github.com/colonyops/hive/internal/platform/tmuxtest"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestSessionNamesDoNotMatchPrefixes(t *testing.T) {
	socket := tmuxtest.Private(t)
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
	runner := tmuxexec.NewExecRunner(tmuxexec.ExecRunnerOptions{
		PrepareArgs: func(args []string) []string {
			return append([]string{"-S", socket, "-f", "/dev/null"}, args...)
		},
	})
	client := tmuxexec.New(zerolog.Nop(), runner)
	manager := tmuxcc.NewManager(t.Context(), tmuxcc.ManagerOptions{Logger: zerolog.Nop()})
	t.Cleanup(func() { require.NoError(t, manager.Stop(context.WithoutCancel(t.Context()))) })
	short := multiplexer.Target{Session: "pbac"}
	long := multiplexer.Target{Session: "pbac-through-lbac-plumbing"}
	spec := func(target multiplexer.Target, dir, window string) multiplexer.SessionSpec {
		return multiplexer.SessionSpec{
			Target: target, WorkingDirectory: dir, Background: true,
			Windows: []multiplexer.WindowSpec{{Name: window, Command: "sleep 300"}},
		}
	}
	require.NoError(t, client.CreateSession(t.Context(), spec(long, t.TempDir(), "long")))

	present, err := client.HasSession(t.Context(), short)
	require.NoError(t, err)
	require.False(t, present)
	present, err = manager.HasSession(t.Context(), short.Session)
	require.NoError(t, err)
	require.False(t, present)
	_, err = manager.Attach(t.Context(), short.Session, 80, 24)
	require.Error(t, err)
	_, err = manager.CurrentPath(t.Context(), short.Session)
	require.ErrorIs(t, err, tmuxcc.ErrNotAttached)
	_, err = manager.CapturePane(t.Context(), short.Session)
	require.Error(t, err)
	_, err = manager.NewWindow(t.Context(), short.Session)
	require.ErrorIs(t, err, tmuxcc.ErrNotAttached)
	_, err = manager.NewCommandWindow(t.Context(), short.Session, t.TempDir(), "agent", "sleep 300")
	require.ErrorIs(t, err, tmuxcc.ErrNotAttached)
	renamed, err := manager.RenameSessionIfPresent(t.Context(), short.Session, "renamed")
	require.NoError(t, err)
	require.False(t, renamed)
	killed, err := manager.KillSession(t.Context(), short.Session)
	require.NoError(t, err)
	require.False(t, killed)
	require.Error(t, client.RenameSession(t.Context(), short, "renamed"))
	require.Error(t, client.KillSession(t.Context(), short))
	require.Error(t, client.AddWindows(t.Context(), short, []multiplexer.WindowSpec{{Name: "extra"}}))

	_, err = client.OpenSession(t.Context(), spec(short, t.TempDir(), "short"), multiplexer.Target{})
	require.NoError(t, err)
	shortWindows, err := manager.Attach(t.Context(), short.Session, 80, 24)
	require.NoError(t, err)
	require.Len(t, shortWindows, 1)
	require.Equal(t, "short", shortWindows[0].Name)
	longWindows, err := manager.Attach(t.Context(), long.Session, 80, 24)
	require.NoError(t, err)
	require.Len(t, longWindows, 1)
	require.Equal(t, "long", longWindows[0].Name)
	require.NotEqual(t, shortWindows[0].ID, longWindows[0].ID)

	_, err = manager.NewCommandWindow(t.Context(), short.Session, t.TempDir(), "extra", "sleep 300")
	require.NoError(t, err)
	renamed, err = manager.RenameSessionIfPresent(t.Context(), short.Session, "renamed")
	require.NoError(t, err)
	require.True(t, renamed)
	killed, err = manager.KillSession(t.Context(), "renamed")
	require.NoError(t, err)
	require.True(t, killed)
	present, err = client.HasSession(t.Context(), long)
	require.NoError(t, err)
	require.True(t, present)
	names, err := manager.SessionNames(t.Context(), "")
	require.NoError(t, err)
	require.Equal(t, []string{long.Session}, names)
}
