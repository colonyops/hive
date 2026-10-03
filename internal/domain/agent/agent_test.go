package agent_test

import (
	"testing"

	"github.com/colonyops/hive/internal/domain/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKnownReturnsACopy(t *testing.T) {
	first := agent.Known()
	first[0].SkipPermissionFlags[0] = "--mutated"
	first[1].Name = "mutated"

	second := agent.Known()
	assert.Equal(t, []string{"--dangerously-skip-permissions"}, second[0].SkipPermissionFlags)
	assert.Equal(t, "opencode", second[1].Name)
}

func TestLookup(t *testing.T) {
	a, ok := agent.Lookup("opencode")
	require.True(t, ok)
	assert.Equal(t, []string{"--agent", "free-permissions-runner"}, a.SkipPermissionFlags)

	_, ok = agent.Lookup("nonexistent")
	assert.False(t, ok)
}
