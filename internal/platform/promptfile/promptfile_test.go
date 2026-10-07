package promptfile

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareLeavesPromptAtThresholdInline(t *testing.T) {
	prompt := strings.Repeat("a", ThresholdBytes)

	prepared, err := Prepare(prompt)
	require.NoError(t, err)
	assert.Equal(t, prompt, prepared.Prompt)
	assert.Empty(t, prepared.Path)
}

func TestPrepareWritesOversizedPromptToPrivateTempFile(t *testing.T) {
	prompt := strings.Repeat("context\n", ThresholdBytes/8+1)

	prepared, err := Prepare(prompt)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, prepared.Remove()) })

	assert.NotEqual(t, prompt, prepared.Prompt)
	assert.Contains(t, prepared.Prompt, prepared.Path)
	assert.Contains(t, prepared.Prompt, "delete the file")

	written, err := os.ReadFile(prepared.Path)
	require.NoError(t, err)
	assert.Equal(t, prompt, string(written))

	info, err := os.Stat(prepared.Path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestPreparedRemoveIsIdempotent(t *testing.T) {
	prepared, err := Prepare(strings.Repeat("x", ThresholdBytes+1))
	require.NoError(t, err)

	require.NoError(t, prepared.Remove())
	require.NoError(t, prepared.Remove())
}
