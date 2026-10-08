package tmuxexec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type startupRunner struct {
	mu          sync.Mutex
	calls       [][]string
	nextPane    int
	nextWindow  int
	paneWindows map[int]int
	listPanes   func(call int) string
	listCalls   int
	capture     string
	options     map[string]string
	failure     func(context.Context, []string) error
}

func (r *startupRunner) Available() bool { return true }

func (r *startupRunner) Capture(ctx context.Context, args ...string) ([]byte, []byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, slices.Clone(args))
	if r.failure != nil {
		if err := r.failure(ctx, args); err != nil {
			return nil, nil, err
		}
	}
	switch args[0] {
	case "new-session", "new-window", "split-window":
		if r.paneWindows == nil {
			r.paneWindows = make(map[int]int)
		}
		window := r.nextWindow
		if args[0] == "split-window" {
			for i, arg := range args {
				if arg == "-t" {
					window, _ = strconv.Atoi(strings.TrimPrefix(args[i+1], "@"))
					break
				}
			}
		} else {
			r.nextWindow++
		}
		pane := r.nextPane
		r.nextPane++
		r.paneWindows[pane] = window
		return fmt.Appendf(nil, "$0 @%d %%%d\n", window, pane), nil, nil
	case "list-panes":
		r.listCalls++
		if r.listPanes != nil {
			return []byte(r.listPanes(r.listCalls)), nil, nil
		}
		var out strings.Builder
		for id := 0; id < r.nextPane; id++ {
			fmt.Fprintf(&out, "%%%d|0||\n", id)
		}
		return []byte(out.String()), nil, nil
	case "capture-pane":
		return []byte(r.capture), nil, nil
	case "list-sessions":
		return []byte("$0\n"), nil, nil
	case "list-windows":
		if slices.Contains(args, "-a") {
			var output strings.Builder
			for i := range r.nextWindow {
				fmt.Fprintf(&output, "@%d\n", i)
			}
			return []byte(output.String()), nil, nil
		}
	case "show-options":
		if slices.Contains(args, "-A") {
			return []byte("off\n"), nil, nil
		}
		for i, arg := range args {
			if arg == "-t" {
				if value, ok := r.options[args[i+1]+":"+args[len(args)-1]]; ok {
					return []byte(value + "\n"), nil, nil
				}
			}
		}
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

func (r *startupRunner) find(command string) [][]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var calls [][]string
	for _, call := range r.calls {
		if call[0] == command {
			calls = append(calls, call)
		}
	}
	return calls
}

func newStartupClient(runner *startupRunner, grace time.Duration) *Client {
	client := New(zerolog.Nop(), runner)
	client.startupGrace = grace
	return client
}

func startupSpec() multiplexer.SessionSpec {
	return multiplexer.SessionSpec{Target: multiplexer.Target{Session: "work"}, Background: true, Windows: []multiplexer.WindowSpec{{Name: "agent", Command: "agent"}}}
}

func TestCreateSessionReportsCommandThatExitsDuringStartup(t *testing.T) {
	runner := &startupRunner{listPanes: func(int) string { return "%0|1|127|\n%1|0||\n" }, capture: "sh: missing-agent: command not found\n  detail\n\n"}
	spec := startupSpec()
	spec.Windows[0].Command = "missing-agent --prompt hi"
	spec.Windows = append(spec.Windows, multiplexer.WindowSpec{Name: "shell"})
	err := newStartupClient(runner, 0).CreateSession(t.Context(), spec)
	require.ErrorIs(t, err, ErrCommandExited)
	var exitErr *CommandExitedError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, "work", exitErr.Session)
	assert.Equal(t, "agent", exitErr.Window)
	assert.Equal(t, "%0", exitErr.Pane)
	assert.Equal(t, 127, exitErr.Status)
	assert.True(t, exitErr.NotFound())
	assert.Equal(t, "sh: missing-agent: command not found\n  detail", exitErr.Output)
	assert.Equal(t, [][]string{{"kill-session", "-t", "$0"}}, runner.find("kill-session"))
	assert.Empty(t, runner.find("select-window"))
	var launchErr *LaunchError
	require.ErrorAs(t, err, &launchErr)
	assert.Equal(t, LaunchPhaseObserving, launchErr.Phase)
}

func TestCreateSessionReportsFailedSplitCommand(t *testing.T) {
	runner := &startupRunner{listPanes: func(int) string { return "%0|0||\n%1|1|1|\n" }}
	spec := startupSpec()
	spec.Windows[0] = multiplexer.WindowSpec{Name: "editor", Panes: []multiplexer.PaneSpec{{}, {Command: "missing-editor", Split: multiplexer.SplitHorizontal}}}
	err := newStartupClient(runner, 0).CreateSession(t.Context(), spec)
	var exitErr *CommandExitedError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, "editor", exitErr.Window)
	assert.Equal(t, "missing-editor", exitErr.Command)
	assert.Equal(t, 1, exitErr.Status)
	assert.NotContains(t, runner.find("new-session")[0], "cat")
	assert.Contains(t, runner.find("set-option"), []string{"set-option", "-p", "-t", "%1", "remain-on-exit", "on"})
	assert.Contains(t, runner.find("respawn-pane"), []string{"respawn-pane", "-k", "-t", "%1", "--", "sh", "-c", "missing-editor"})
}

func TestCreateSessionHealthyStartupRestoresEachPaneOption(t *testing.T) {
	runner := &startupRunner{options: map[string]string{"%0:remain-on-exit": "failed", "%0:remain-on-exit-format": ""}}
	spec := startupSpec()
	spec.Windows[0].Panes = []multiplexer.PaneSpec{{Command: "first"}, {Command: "second"}}
	require.NoError(t, newStartupClient(runner, 0).CreateSession(t.Context(), spec))
	assert.Contains(t, runner.find("set-option"), []string{"set-option", "-p", "-t", "%0", "remain-on-exit", "failed"})
	assert.Contains(t, runner.find("set-option"), []string{"set-option", "-p", "-t", "%0", "remain-on-exit-format", ""})
	assert.Contains(t, runner.find("set-option"), []string{"set-option", "-p", "-t", "%1", "-u", "remain-on-exit"})
	assert.Empty(t, runner.find("kill-session"))
}

func TestCreateSessionWaitsForExitMetadata(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		runner := &startupRunner{listPanes: func(call int) string {
			if call == 1 {
				return "%0|1||\n"
			}
			return "%0|1|127|\n"
		}}
		err := newStartupClient(runner, 100*time.Millisecond).CreateSession(t.Context(), startupSpec())
		require.ErrorIs(t, err, ErrCommandExited)
		assert.Equal(t, 2, runner.listCalls)
	})
}

func TestCreateSessionObservesDefaultShellWithoutRespawningIt(t *testing.T) {
	runner := &startupRunner{}
	spec := startupSpec()
	spec.Windows[0].Command = ""
	require.NoError(t, newStartupClient(runner, 0).CreateSession(t.Context(), spec))
	assert.Equal(t, 2, runner.listCalls)
	assert.Empty(t, runner.find("respawn-pane"))
}

func TestSuccessfulCommandDoesNotRollBackLivePanes(t *testing.T) {
	runner := &startupRunner{listPanes: func(int) string { return "%0|1|0|\n%1|0||\n" }}
	spec := startupSpec()
	spec.Windows = append(spec.Windows, multiplexer.WindowSpec{Name: "shell"})
	require.NoError(t, newStartupClient(runner, 0).CreateSession(t.Context(), spec))
	assert.Empty(t, runner.find("kill-session"))
	assert.Equal(t, [][]string{{"kill-pane", "-t", "%0"}}, runner.find("kill-pane"))
	assert.Equal(t, [][]string{{"select-window", "-t", "@1"}}, runner.find("select-window"))
}

type absentSessionError struct{}

func (absentSessionError) Error() string { return "session does not exist" }
func (absentSessionError) ExitCode() int { return 1 }

func TestEntirelyFiniteLaunchDoesNotAttach(t *testing.T) {
	runner := &startupRunner{listPanes: func(int) string { return "%0|1|0|\n" }, failure: func(_ context.Context, args []string) error {
		if args[0] == "has-session" {
			return absentSessionError{}
		}
		return nil
	}}
	spec := startupSpec()
	spec.Background = false
	result, err := newStartupClient(runner, 0).OpenSession(t.Context(), spec, multiplexer.Target{})
	require.NoError(t, err)
	assert.True(t, result.Created)
	assert.True(t, result.Completed)
	assert.Empty(t, runner.find("attach-session"))
	assert.Empty(t, runner.find("select-window"))
}

func TestLaunchFailureRollsBackEveryMutationBoundary(t *testing.T) {
	for _, boundary := range []string{"mark-window", "show-options", "arm-format", "arm-retention", "respawn-pane", "list-panes", "restore-option", "select-window", "clear-marker"} {
		t.Run(boundary, func(t *testing.T) {
			runner := &startupRunner{failure: func(_ context.Context, args []string) error {
				matches := args[0] == boundary
				if args[0] == "set-option" {
					matches = matches || (boundary == "mark-window" && slices.Contains(args, "@0") && !slices.Contains(args, "-u")) || (boundary == "arm-format" && args[len(args)-2] == "remain-on-exit-format") || (boundary == "arm-retention" && args[len(args)-1] == "on") || (boundary == "restore-option" && slices.Contains(args, "-p") && slices.Contains(args, "-u")) || (boundary == "clear-marker" && slices.Contains(args, "-u") && args[len(args)-1] == launchMarker)
				}
				if matches {
					return assert.AnError
				}
				return nil
			}}
			err := newStartupClient(runner, 0).CreateSession(t.Context(), startupSpec())
			require.ErrorIs(t, err, assert.AnError)
			assert.Equal(t, [][]string{{"kill-session", "-t", "$0"}}, runner.find("kill-session"))
		})
	}
}

func TestAddWindowsFailurePreservesExistingSession(t *testing.T) {
	runner := &startupRunner{listPanes: func(int) string { return "%0|0||\n%1|1|1|\n" }}
	err := newStartupClient(runner, 0).AddWindows(t.Context(), multiplexer.Target{Session: "work"}, []multiplexer.WindowSpec{{Name: "healthy", Command: "agent"}, {Name: "failed", Command: "bad"}})
	require.ErrorIs(t, err, ErrCommandExited)
	assert.Empty(t, runner.find("kill-session"))
	assert.Equal(t, [][]string{{"kill-window", "-t", "@0"}, {"kill-window", "-t", "@1"}}, runner.find("kill-window"))
}

func TestCleanupFailureDoesNotReplaceCommandFailure(t *testing.T) {
	cleanupErr := errors.New("cleanup failure")
	runner := &startupRunner{listPanes: func(int) string { return "%0|1|127|\n" }, failure: func(_ context.Context, args []string) error {
		if args[0] == "kill-session" {
			return cleanupErr
		}
		return nil
	}}
	err := newStartupClient(runner, 0).CreateSession(t.Context(), startupSpec())
	require.ErrorIs(t, err, ErrCommandExited)
	require.ErrorIs(t, err, cleanupErr)
	var report *CleanupError
	require.ErrorAs(t, err, &report)
	assert.Equal(t, []string{"$0"}, report.Resources)
	assert.Contains(t, err.Error(), "cleanup did not complete")
}

func TestCancellationUsesIndependentBoundedCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var cleaned bool
	runner := &startupRunner{failure: func(ctx context.Context, args []string) error {
		if args[0] == "respawn-pane" {
			cancel()
			return ctx.Err()
		}
		if args[0] == "kill-session" {
			cleaned = true
			require.NoError(t, ctx.Err())
			_, bounded := ctx.Deadline()
			assert.True(t, bounded)
		}
		return nil
	}}
	err := newStartupClient(runner, 0).CreateSession(ctx, startupSpec())
	require.ErrorIs(t, err, context.Canceled)
	assert.True(t, cleaned)
}

func TestStartupTerminationDetails(t *testing.T) {
	for _, test := range []struct {
		name, row string
		signal    string
		lost      bool
	}{{"signal", "%0|1||9\n", "9", false}, {"unknown", "%0|1||\n", "", false}, {"lost", "", "", true}} {
		t.Run(test.name, func(t *testing.T) {
			runner := &startupRunner{listPanes: func(int) string { return test.row }}
			err := newStartupClient(runner, 0).CreateSession(t.Context(), startupSpec())
			var report *CommandExitedError
			require.ErrorAs(t, err, &report)
			assert.Equal(t, test.signal, report.Signal)
			assert.Equal(t, test.lost, report.Lost)
			assert.Equal(t, -1, report.Status)
			if test.signal == "" {
				assert.Contains(t, err.Error(), "No exit details are available.")
			}
		})
	}
}

func TestCaptureFailurePreservesExitAndUserOutput(t *testing.T) {
	runner := &startupRunner{listPanes: func(int) string { return "%0|1|1|\n" }, failure: func(_ context.Context, args []string) error {
		if args[0] == "capture-pane" {
			return assert.AnError
		}
		return nil
	}}
	err := newStartupClient(runner, 0).CreateSession(t.Context(), startupSpec())
	var report *CommandExitedError
	require.ErrorAs(t, err, &report)
	assert.Equal(t, assert.AnError, report.CaptureError)
	assert.Contains(t, err.Error(), "Terminal output unavailable")
	runner = &startupRunner{listPanes: func(int) string { return "%0|1|1|\n" }, capture: "Pane is dead is application text\n\n  detail\n"}
	err = newStartupClient(runner, 0).CreateSession(t.Context(), startupSpec())
	require.ErrorAs(t, err, &report)
	assert.Equal(t, "Pane is dead is application text\n\n  detail", report.Output)
}
