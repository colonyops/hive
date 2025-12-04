package executil

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
