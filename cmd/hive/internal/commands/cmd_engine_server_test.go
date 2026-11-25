package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/ops"
)

func TestEngineCommandsTakeServer(t *testing.T) {
	for _, args := range [][]string{
		{"hc", "next", "--server", "nowhere", "hc-1"},
		{"drive", "list", "--server", "nowhere"},
		{"msg", "list", "--server", "nowhere"},
		{"todo", "list", "--server", "nowhere"},
	} {
		_, err := runRoot(t, driveRoot(testApp(t)), args...)
		require.Error(t, err, "%v", args)
		assert.Equal(t, ops.KindNotFound, ErrorKind(err), "%v: %v", args, err)
		assert.Contains(t, err.Error(), `unknown server "nowhere"`)
	}
}

func TestRemoteDriveCommandsStillRefuseSpaceFlags(t *testing.T) {
	_, err := runRoot(t, driveRoot(testApp(t)), "drive", "--shared", "put", "--server", "nowhere", "a.md", "--body", "x")

	require.Error(t, err)
	assert.Equal(t, ops.KindInvalid, ErrorKind(err))
	assert.Contains(t, err.Error(), "--shared")
}

func TestEngineCommandsReadHiveServer(t *testing.T) {
	t.Setenv("HIVE_SERVER", "nowhere")

	_, err := runRoot(t, driveRoot(testApp(t)), "hc", "list")

	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown server "nowhere"`)
}

func TestEngineCommandsOnLocalStayHere(t *testing.T) {
	_, err := runRoot(t, driveRoot(testApp(t)), "hc", "list", "--server", "local", "--caller", "global")

	require.NoError(t, err)
}
