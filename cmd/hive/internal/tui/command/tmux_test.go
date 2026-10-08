package command

import (
	"context"
	"testing"

	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/stretchr/testify/require"
)

func TestTmuxExecutorReportsFiniteCompletion(t *testing.T) {
	opener := &mockTmuxOpener{result: multiplexer.LaunchResult{Created: true, Completed: true}}
	executor := &TmuxExecutor{opener: opener}
	output, done, cancel := executor.Execute(t.Context())
	defer cancel()
	require.Equal(t, "Session completed successfully.", <-output)
	require.NoError(t, <-done)
}

func TestTmuxExecutorCancellationDoesNotBlockCompletionDelivery(t *testing.T) {
	opener := &mockTmuxOpener{result: multiplexer.LaunchResult{Completed: true}}
	executor := &TmuxExecutor{opener: opener}
	_, done, cancel := executor.Execute(t.Context())
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
}
