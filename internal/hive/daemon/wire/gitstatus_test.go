package wire

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/hive/gitstatus"
)

func TestGitStatusesKeepFailedReadsAcrossTheWire(t *testing.T) {
	got := GitStatusesFromProto(GitStatusesToProto(map[string]gitstatus.Status{
		"a": {Branch: "main", Resolved: true, Additions: 3, Err: errors.New("git diff: killed"), UnpushedErr: errors.New("no upstream")},
		"b": {Branch: "dev", Resolved: true},
	}))

	assert.Equal(t, "main", got["a"].Branch)
	assert.Equal(t, 3, got["a"].Additions)
	require.EqualError(t, got["a"].Err, "git diff: killed")
	require.EqualError(t, got["a"].UnpushedErr, "no upstream")
	assert.NoError(t, got["b"].Err)
	assert.NoError(t, got["b"].UnpushedErr)
}
