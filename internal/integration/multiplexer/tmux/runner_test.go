package tmux

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/multiplexer"
	"github.com/colonyops/hive/pkg/executil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func installFakeTmux(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "tmux")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestExecRunnerSeparatesStreamsAndForwardsInput(t *testing.T) {
	installFakeTmux(t, "cat\nprintf 'diagnostic' >&2\n")
	runner := execRunner{}

	stdout, stderr, err := runner.Input(context.Background(), strings.NewReader("input bytes"), "ignored")
	require.NoError(t, err)
	assert.Equal(t, "input bytes", string(stdout))
	assert.Equal(t, "diagnostic", string(stderr))
}

func TestExecRunnerReturnsBoundedCommandError(t *testing.T) {
	installFakeTmux(t, "printf '%0600d' 0 >&2\nexit 7\n")
	runner := execRunner{}

	_, _, err := runner.Capture(context.Background(), "ignored")
	require.Error(t, err)
	var commandErr *executil.CommandError
	require.ErrorAs(t, err, &commandErr)
	assert.Len(t, commandErr.Output, 500)
}

func TestExecRunnerHonorsCancellation(t *testing.T) {
	installFakeTmux(t, "sleep 10\n")
	runner := execRunner{}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	started := time.Now()
	_, _, err := runner.Capture(ctx, "ignored")
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(started), time.Second)
}

func TestExecRunnerInteractiveStreamsAndDiagnostics(t *testing.T) {
	installFakeTmux(t, "cat\nprintf 'interactive error' >&2\nexit 3\n")
	runner := execRunner{}
	var stdout, stderr bytes.Buffer

	err := runner.Interactive(context.Background(), multiplexer.AttachStreams{
		Stdin: strings.NewReader("interactive input"), Stdout: &stdout, Stderr: &stderr,
	}, "ignored")
	require.Error(t, err)
	assert.Equal(t, "interactive input", stdout.String())
	assert.Equal(t, "interactive error", stderr.String())
	assert.Contains(t, err.Error(), "interactive error")
	assert.NotErrorIs(t, err, context.Canceled)
}
