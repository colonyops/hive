package app

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/multiplexer"
	tmuxcc "github.com/colonyops/hive/internal/platform/tmux/control"
)

type recordingRunner struct {
	calls  [][]string
	stdout string
	stderr string
	err    error
}

func (r *recordingRunner) Available() bool { return true }

func (r *recordingRunner) Capture(_ context.Context, args ...string) ([]byte, []byte, error) {
	r.calls = append(r.calls, args)
	return []byte(r.stdout), []byte(r.stderr), r.err
}

func (r *recordingRunner) Input(context.Context, io.Reader, ...string) ([]byte, []byte, error) {
	return nil, nil, errors.New("unused")
}

func (r *recordingRunner) Interactive(context.Context, multiplexer.AttachStreams, ...string) error {
	return errors.New("unused")
}

func TestNewWindowArgsStartsDetachedInTheCheckout(t *testing.T) {
	assert.Equal(t,
		[]string{"new-window", "-d", "-t", "=fix-login:", "-c", "/work/site", "-P", "-F", "#{window_id}"},
		newWindowArgs("fix-login", "/work/site", "", ""))
	assert.Equal(t,
		[]string{
			"new-window", "-d", "-t", "=fix-login:", "-c", "/work/site", "-n", "tests", "-P", "-F", "#{window_id}",
			"--", "sh", "-c", "make test # watch\nexec \"${SHELL:-sh}\" -l",
		},
		newWindowArgs("fix-login", "/work/site", "tests", "make test # watch"),
		"the exec sits on its own line so a trailing comment in the command cannot hide it")
}

func TestTmuxWindowsNewWindowReadsTheWindowID(t *testing.T) {
	runner := &recordingRunner{stdout: "@7\n"}
	w := &tmuxWindows{runner: runner}

	id, err := w.NewWindow(t.Context(), "fix-login", "/work/site", "logs", "tail -f x.log")
	require.NoError(t, err)
	assert.Equal(t, "@7", id)
	require.Len(t, runner.calls, 1)

	runner.stdout = "oops"
	_, err = w.NewWindow(t.Context(), "fix-login", "/work/site", "", "")
	require.Error(t, err, "output that is not a window id")

	runner.stdout, runner.stderr, runner.err = "", "can't find session", errors.New("exit status 1")
	_, err = w.NewWindow(t.Context(), "fix-login", "/work/site", "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "can't find session")
}

func TestTmuxWindowsFindWindow(t *testing.T) {
	windows := []tmuxcc.Window{{ID: "@1", Name: "claude"}, {ID: "@3", Name: "logs"}, {ID: "@5", Name: "dup"}, {ID: "@6", Name: "dup"}}
	w := &tmuxWindows{listWindows: func(context.Context, string) ([]tmuxcc.Window, error) { return windows, nil }}

	for name, tc := range map[string]struct{ in, want string }{
		"by id":   {"@3", "@3"},
		"by name": {"logs", "@3"},
	} {
		got, err := w.FindWindow(t.Context(), "s", tc.in)
		require.NoError(t, err, name)
		assert.Equal(t, tc.want, got, name)
	}

	_, err := w.FindWindow(t.Context(), "s", "ghost")
	assert.Equal(t, KindNotFound, KindOf(err))
	_, err = w.FindWindow(t.Context(), "s", "dup")
	assert.Equal(t, KindInvalid, KindOf(err), "a name two windows share")
	_, err = w.FindWindow(t.Context(), "s", "log")
	assert.Equal(t, KindNotFound, KindOf(err), "names match exactly")
}
