package executil

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunSh_StderrCappedAtMaxLen(t *testing.T) {
	ctx := context.Background()

	// Write twice the cap to stderr; only the first maxStderrLen bytes should appear in the error.
	longStderr := strings.Repeat("A", maxStderrLen*2)
	cmd := fmt.Sprintf("printf '%%s' '%s' >&2; exit 1", longStderr)

	err := RunSh(ctx, "", cmd)
	require.Error(t, err)

	errMsg := err.Error()
	// Error format: "<stderr prefix>: exit status 1"
	// The stderr portion must not exceed maxStderrLen bytes.
	assert.LessOrEqual(t, len(errMsg), maxStderrLen+20, "error message should be capped")
	assert.Equal(t, strings.Repeat("A", maxStderrLen), errMsg[:maxStderrLen], "first %d bytes should be the capped stderr", maxStderrLen)
}

func TestRunSh_PreservesExitError(t *testing.T) {
	ctx := context.Background()

	// Command that writes to stderr and exits non-zero.
	err := RunSh(ctx, "", "echo 'error message' >&2; exit 1")
	require.Error(t, err)

	var exitErr *exec.ExitError
	assert.ErrorAs(t, err, &exitErr, "original ExitError should be preserved via wrapping")
}

func TestRunSh_NoStderrReturnsExitError(t *testing.T) {
	ctx := context.Background()

	// Command that exits non-zero with no stderr output.
	err := RunSh(ctx, "", "exit 2")
	require.Error(t, err)

	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 2, exitErr.ExitCode())
}

func TestRealExecutor_Run(t *testing.T) {
	exec := &RealExecutor{}
	ctx := context.Background()

	t.Run("successful command", func(t *testing.T) {
		out, err := exec.Run(ctx, "echo", "hello")
		require.NoError(t, err)
		assert.Equal(t, "hello\n", string(out))
	})

	t.Run("command not found", func(t *testing.T) {
		_, err := exec.Run(ctx, "nonexistent-command-12345")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exec nonexistent-command-12345")
	})

	t.Run("command fails", func(t *testing.T) {
		_, err := exec.Run(ctx, "false")
		require.Error(t, err)
	})
}

func TestRealExecutor_RunDir(t *testing.T) {
	exec := &RealExecutor{}
	ctx := context.Background()

	t.Run("runs in specified directory", func(t *testing.T) {
		out, err := exec.RunDir(ctx, "/tmp", "pwd")
		require.NoError(t, err)
		assert.Contains(t, string(out), "/tmp")
	})

	t.Run("invalid directory", func(t *testing.T) {
		_, err := exec.RunDir(ctx, "/nonexistent-dir-12345", "pwd")
		require.Error(t, err)
	})
}

func TestRecordingExecutor_Run(t *testing.T) {
	t.Run("records commands", func(t *testing.T) {
		exec := &RecordingExecutor{}
		ctx := context.Background()

		_, _ = exec.Run(ctx, "git", "clone", "url")
		_, _ = exec.Run(ctx, "git", "checkout", "main")

		require.Len(t, exec.Commands, 2)
		assert.Equal(t, "git", exec.Commands[0].Cmd)
		assert.Equal(t, []string{"clone", "url"}, exec.Commands[0].Args)
		assert.Empty(t, exec.Commands[0].Dir)
	})

	t.Run("records directory", func(t *testing.T) {
		exec := &RecordingExecutor{}
		ctx := context.Background()

		_, _ = exec.RunDir(ctx, "/tmp/repo", "git", "status")

		require.Len(t, exec.Commands, 1)
		assert.Equal(t, "/tmp/repo", exec.Commands[0].Dir)
	})

	t.Run("returns configured output", func(t *testing.T) {
		exec := &RecordingExecutor{
			Outputs: map[string][]byte{
				"git": []byte("output"),
			},
		}
		ctx := context.Background()

		out, err := exec.Run(ctx, "git", "status")
		require.NoError(t, err)
		assert.Equal(t, []byte("output"), out)
	})

	t.Run("returns configured error", func(t *testing.T) {
		expectedErr := errors.New("command failed")
		exec := &RecordingExecutor{
			Errors: map[string]error{
				"git": expectedErr,
			},
		}
		ctx := context.Background()

		_, err := exec.Run(ctx, "git", "status")
		assert.Equal(t, expectedErr, err)
	})

	t.Run("reset clears commands", func(t *testing.T) {
		exec := &RecordingExecutor{}
		ctx := context.Background()

		_, _ = exec.Run(ctx, "echo", "hello")
		require.Len(t, exec.Commands, 1)

		exec.Reset()
		assert.Empty(t, exec.Commands)
	})
}
