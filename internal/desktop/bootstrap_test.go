package desktop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// unsetEnv removes key for the duration of the test, restoring the original
// value (or absence) afterward. t.Setenv cannot express "unset", which the
// ApplyBootstrap precedence tests need.
func unsetEnv(t *testing.T, key string) {
	t.Helper()
	orig, had := os.LookupEnv(key)
	require.NoError(t, os.Unsetenv(key))
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(key, orig)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}

func TestLoadBootstrapMissingIsEmpty(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	b, err := LoadBootstrap()
	require.NoError(t, err)
	require.Equal(t, Bootstrap{}, b)
}

func TestBootstrapRoundtrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	want := Bootstrap{DataDir: "/custom/data", ConfigDir: "/custom/cfg"}
	require.NoError(t, SaveBootstrap(want))

	got, err := LoadBootstrap()
	require.NoError(t, err)
	require.Equal(t, want, got)

	// The file lands at the fixed anchor, independent of any config override.
	require.FileExists(t, BootstrapPath())
}

func TestApplyBootstrapSeedsEnvWhenUnset(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	unsetEnv(t, "HIVE_DATA_DIR")
	unsetEnv(t, EnvConfigPath)
	require.NoError(t, SaveBootstrap(Bootstrap{DataDir: "/custom/data", ConfigDir: "/custom/cfg"}))

	require.NoError(t, ApplyBootstrap())

	require.Equal(t, "/custom/data", os.Getenv("HIVE_DATA_DIR"))
	require.Equal(t, filepath.Join("/custom/cfg", "profiles.yaml"), os.Getenv(EnvConfigPath))
}

func TestApplyBootstrapKeepsExplicitEnv(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HIVE_DATA_DIR", "/env/data")
	t.Setenv(EnvConfigPath, "/env/cfg/profiles.yaml")
	require.NoError(t, SaveBootstrap(Bootstrap{DataDir: "/custom/data", ConfigDir: "/custom/cfg"}))

	require.NoError(t, ApplyBootstrap())

	require.Equal(t, "/env/data", os.Getenv("HIVE_DATA_DIR"))
	require.Equal(t, "/env/cfg/profiles.yaml", os.Getenv(EnvConfigPath))
}

func TestDataDirAndConfigDirDerive(t *testing.T) {
	t.Setenv("HIVE_DATA_DIR", "/root/data")
	t.Setenv(EnvConfigPath, "/root/cfg/profiles.yaml")

	require.Equal(t, "/root/data", DataDir())
	require.Equal(t, "/root/cfg", ConfigDir())
}
