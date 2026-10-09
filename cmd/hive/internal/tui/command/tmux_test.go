package command

import (
	"testing"

	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/stretchr/testify/require"
)

func TestTmuxExecutorReportsFiniteCompletion(t *testing.T) {
	opener := &mockTmuxOpener{result: multiplexer.LaunchResult{Created: true, Completed: true}}
	executor := &TmuxExecutor{opener: opener}
	require.NoError(t, ExecuteSync(t.Context(), executor))
	require.Equal(t, "Session completed successfully.", executor.ResultMessage())
}

func TestTmuxExecutorHasNoMessageForALiveSession(t *testing.T) {
	opener := &mockTmuxOpener{result: multiplexer.LaunchResult{Created: true}}
	executor := &TmuxExecutor{opener: opener}
	require.NoError(t, ExecuteSync(t.Context(), executor))
	require.Empty(t, executor.ResultMessage())
}
