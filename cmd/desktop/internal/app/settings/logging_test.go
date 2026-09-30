package settings

import (
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestResolveLogLevel(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		t.Setenv(EnvLogLevel, "")
		level, err := ResolveLogLevel()
		require.NoError(t, err)
		require.Equal(t, zerolog.InfoLevel, level)
	})
	t.Run("valid", func(t *testing.T) {
		t.Setenv(EnvLogLevel, "debug")
		level, err := ResolveLogLevel()
		require.NoError(t, err)
		require.Equal(t, zerolog.DebugLevel, level)
	})
	t.Run("invalid", func(t *testing.T) {
		t.Setenv(EnvLogLevel, "verbose")
		_, err := ResolveLogLevel()
		require.ErrorContains(t, err, EnvLogLevel)
	})
}
