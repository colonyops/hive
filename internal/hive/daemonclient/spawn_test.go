package daemonclient

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpawnEnvPinsTheDaemonToTheCallersDataDirAndConfig(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	got := spawnEnv([]string{"HIVE_DATA_DIR=/old", "HIVE_CONFIG=rel/config.yaml", "X=1", "HIVE_CONFIG_EXTRA=y"}, "/new")

	assert.Equal(t, []string{
		"HIVE_CONFIG=" + filepath.Join(cwd, "rel/config.yaml"),
		"X=1",
		"HIVE_CONFIG_EXTRA=y",
		"HIVE_DATA_DIR=/new",
	}, got, "the daemon runs in the data dir, so a relative config path must be made absolute, and the caller's data dir replaces an inherited one")
}

func TestSpawnEnvKeepsAnEmptyConfig(t *testing.T) {
	assert.Equal(t, []string{"HIVE_CONFIG=", "HIVE_DATA_DIR=/d"}, spawnEnv([]string{"HIVE_CONFIG="}, "/d"))
}
