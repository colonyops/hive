package settings

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// unsetEnv removes key for the duration of the test, restoring the original
// value (or absence) afterward. t.Setenv cannot express "unset", which the
// typed path precedence tests need.
func unsetEnv(t *testing.T, key string) {
	t.Helper()
	orig, had := os.LookupEnv(key)
	require.NoError(t, os.Unsetenv(key))
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(key, orig) //nolint:usetesting // this restore IS the cleanup t.Setenv would register
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

func TestResolvePathsUsesBootstrapWhenEnvUnset(t *testing.T) {
	unsetEnv(t, EnvDataDir)
	unsetEnv(t, EnvConfigDir)
	paths := ResolvePaths(Bootstrap{DataDir: "/custom/data", ConfigDir: "/custom/cfg"}, ResolveOptions{})
	require.Equal(t, "/custom/data", paths.DataDir)
	require.Equal(t, "/custom/cfg", paths.ConfigDir)
}

func TestResolvePathsKeepsExplicitEnv(t *testing.T) {
	t.Setenv(EnvDataDir, "/env/data")
	t.Setenv(EnvConfigDir, "/env/cfg")
	paths := ResolvePaths(Bootstrap{DataDir: "/custom/data", ConfigDir: "/custom/cfg"}, ResolveOptions{})
	require.Equal(t, "/env/data", paths.DataDir)
	require.Equal(t, "/env/cfg", paths.ConfigDir)
}

func TestDataDirAndConfigDirDerive(t *testing.T) {
	t.Setenv(EnvDataDir, "/root/data")
	t.Setenv(EnvConfigDir, "/root/cfg")

	require.Equal(t, "/root/data", DataDir())
	require.Equal(t, "/root/cfg", ConfigDir())
}
