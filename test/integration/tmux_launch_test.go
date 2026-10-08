//go:build integration

package integration

import (
	"context"
	"errors"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/colonyops/hive/internal/domain/multiplexer"
	tmuxexec "github.com/colonyops/hive/internal/platform/tmux/exec"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func launchSpec(name string, windows ...multiplexer.WindowSpec) multiplexer.SessionSpec {
	return multiplexer.SessionSpec{Target: multiplexer.Target{Session: name}, Background: true, Windows: windows}
}

func TestTmuxLaunchFailurePreservesDiagnostics(t *testing.T) {
	NewHarness(t)
	name := "launch-diagnostics"
	cleanupTmuxSession(t, name)
	client := tmuxexec.NewDefault(zerolog.Nop())
	err := client.CreateSession(t.Context(), launchSpec(name, multiplexer.WindowSpec{Name: "agent", Command: "printf 'failure\\n\\n  indented detail\\n'; exit 1"}))
	var report *tmuxexec.CommandExitedError
	require.ErrorAs(t, err, &report)
	assert.Equal(t, 1, report.Status)
	assert.Equal(t, "failure\n\n  indented detail", report.Output)
	assert.NotContains(t, report.Output, "Pane is dead")
	exists, probeErr := client.HasSession(t.Context(), multiplexer.Target{Session: name})
	require.NoError(t, probeErr)
	assert.False(t, exists)
}

func TestTmuxSuccessfulCommandKeepsHealthySibling(t *testing.T) {
	NewHarness(t)
	name := "launch-successful-sibling"
	cleanupTmuxSession(t, name)
	client := tmuxexec.NewDefault(zerolog.Nop())
	require.NoError(t, client.CreateSession(t.Context(), launchSpec(name, multiplexer.WindowSpec{Name: "setup", Command: "exit 0"}, multiplexer.WindowSpec{Name: "agent", Command: "sleep 600"})))
	assertTmuxWindowNames(t, name, []string{"agent"})
}

func TestTmuxFiniteLaunchReportsCompletionWithoutAttach(t *testing.T) {
	NewHarness(t)
	name := "launch-finite"
	cleanupTmuxSession(t, name)
	client := tmuxexec.NewDefault(zerolog.Nop())
	spec := launchSpec(name, multiplexer.WindowSpec{Name: "setup", Command: "exit 0"})
	spec.Background = false
	result, err := client.OpenSession(t.Context(), spec, multiplexer.Target{})
	require.NoError(t, err)
	assert.True(t, result.Created)
	assert.True(t, result.Completed)
	exists, err := client.HasSession(t.Context(), spec.Target)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestTmuxAddWindowsFailureRollsBackOnlyNewWindows(t *testing.T) {
	NewHarness(t)
	name := "launch-add-rollback"
	cleanupTmuxSession(t, name)
	client := tmuxexec.NewDefault(zerolog.Nop())
	require.NoError(t, client.CreateSession(t.Context(), launchSpec(name, multiplexer.WindowSpec{Name: "original"})))
	err := client.AddWindows(t.Context(), multiplexer.Target{Session: name}, []multiplexer.WindowSpec{{Name: "healthy", Command: "sleep 600"}, {Name: "failed", Command: "exit 1"}})
	require.ErrorIs(t, err, tmuxexec.ErrCommandExited)
	assertTmuxWindowNames(t, name, []string{"original"})
}

func TestTmuxLaunchRestoresPaneOptionInheritance(t *testing.T) {
	NewHarness(t)
	name := "launch-option-restore"
	cleanupTmuxSession(t, name)
	client := tmuxexec.NewDefault(zerolog.Nop())
	require.NoError(t, client.CreateSession(t.Context(), launchSpec(name, multiplexer.WindowSpec{Name: "agent", Panes: []multiplexer.PaneSpec{{Command: "sleep 600"}, {Command: "sleep 600"}}})))
	ids, err := exec.Command("tmux", "list-panes", "-t", name, "-F", "#{pane_id}").Output()
	require.NoError(t, err)
	for _, id := range strings.Fields(string(ids)) {
		for _, option := range []string{"remain-on-exit", "remain-on-exit-format"} {
			out, err := exec.Command("tmux", "show-options", "-p", "-t", id, option).Output()
			require.NoError(t, err)
			assert.Empty(t, out, "%s on %s must inherit its original setting", option, id)
		}
	}
}

type respawnFailureRunner struct{ tmuxexec.Runner }

func (r respawnFailureRunner) Capture(ctx context.Context, args ...string) ([]byte, []byte, error) {
	if args[0] == "respawn-pane" {
		return nil, nil, errors.New("injected respawn failure")
	}
	return r.Runner.Capture(ctx, args...)
}

func TestTmuxFirstRespawnFailureLeavesNoPlaceholder(t *testing.T) {
	NewHarness(t)
	name := "launch-respawn-failure"
	cleanupTmuxSession(t, name)
	client := tmuxexec.New(zerolog.Nop(), respawnFailureRunner{tmuxexec.NewExecRunner(tmuxexec.ExecRunnerOptions{})})
	err := client.CreateSession(t.Context(), launchSpec(name, multiplexer.WindowSpec{Name: "agent", Command: "sleep 600"}))
	require.ErrorContains(t, err, "injected respawn failure")
	exists, err := client.HasSession(t.Context(), multiplexer.Target{Session: name})
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestTmuxOpenRecoversInterruptedLaunch(t *testing.T) {
	NewHarness(t)
	name := "launch-interrupted"
	cleanupTmuxSession(t, name)
	out, err := exec.Command("tmux", "new-session", "-d", "-s", name, "-n", "placeholder", "--", "cat").CombinedOutput()
	require.NoError(t, err, string(out))
	out, err = exec.Command("tmux", "set-option", "-t", name, tmuxexec.LaunchPendingOption, "1").CombinedOutput()
	require.NoError(t, err, string(out))
	client := tmuxexec.NewDefault(zerolog.Nop())
	result, err := client.OpenSession(t.Context(), launchSpec(name, multiplexer.WindowSpec{Name: "agent", Command: "sleep 600"}), multiplexer.Target{})
	require.NoError(t, err)
	assert.True(t, result.Created)
	assertTmuxWindowNames(t, name, []string{"agent"})
}

func TestTmuxConcurrentOpenCreatesOneSession(t *testing.T) {
	NewHarness(t)
	name := "launch-concurrent"
	cleanupTmuxSession(t, name)
	var wg sync.WaitGroup
	results := make([]multiplexer.LaunchResult, 2)
	failures := make([]error, 2)
	for i := range results {
		wg.Go(func() {
			client := tmuxexec.NewDefault(zerolog.Nop())
			results[i], failures[i] = client.OpenSession(t.Context(), launchSpec(name, multiplexer.WindowSpec{Name: "agent", Command: "sleep 600"}), multiplexer.Target{})
		})
	}
	wg.Wait()
	for _, err := range failures {
		require.NoError(t, err)
	}
	assert.Equal(t, 1, len(slices.DeleteFunc(slices.Clone(results), func(result multiplexer.LaunchResult) bool { return !result.Created })))
	assertTmuxWindowNames(t, name, []string{"agent"})
}
