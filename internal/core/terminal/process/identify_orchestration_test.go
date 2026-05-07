package process

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubReaders installs fake process-reader functions for the duration of a
// single sub-test, restoring the originals on cleanup.
func stubReaders(t *testing.T, tpgid int, tpgidErr error, comm string, argv []string, env map[string]string) {
	t.Helper()
	origTpgid, origComm, origArgv, origEnv := readTpgid, readComm, readArgv, readEnv
	readTpgid = func(int) (int, error) { return tpgid, tpgidErr }
	readComm = func(int) string { return comm }
	readArgv = func(int) ([]string, error) { return argv, nil }
	readEnv = func(int) map[string]string { return env }
	t.Cleanup(func() {
		readTpgid, readComm, readArgv, readEnv = origTpgid, origComm, origArgv, origEnv
	})
}

func TestIdentify(t *testing.T) {
	t.Run("zero pid returns nil", func(t *testing.T) {
		got, err := Identify(0)
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("negative pid returns nil", func(t *testing.T) {
		got, err := Identify(-1)
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("CLAUDECODE env wins", func(t *testing.T) {
		stubReaders(t, 1234, nil, "node", []string{"node", "/path/to/something"}, map[string]string{"CLAUDECODE": "1"})
		got, err := Identify(999)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "claude", got.Tool)
		assert.Equal(t, 1234, got.PID)
	})

	t.Run("argv with claude basename matches via looksLikeClaude", func(t *testing.T) {
		stubReaders(t, 1234, nil, "claude", []string{"/usr/local/bin/claude"}, nil)
		got, err := Identify(999)
		require.NoError(t, err)
		assert.Equal(t, "claude", got.Tool)
	})

	t.Run("argv-based codex match", func(t *testing.T) {
		stubReaders(t, 200, nil, "node", []string{"npx", "codex"}, map[string]string{"PATH": "/usr/bin"})
		got, err := Identify(999)
		require.NoError(t, err)
		assert.Equal(t, "codex", got.Tool)
	})

	t.Run("comm-based gemini match when argv missing", func(t *testing.T) {
		stubReaders(t, 200, nil, "gemini", nil, nil)
		got, err := Identify(999)
		require.NoError(t, err)
		assert.Equal(t, "gemini", got.Tool)
	})

	t.Run("unknown process returns Tool empty", func(t *testing.T) {
		// Critical: callers depend on Tool="" as the signal to fall back to
		// content-based detection. Returning "shell" here would silently
		// disable the content path for wrappers like `sh -c claude`.
		stubReaders(t, 200, nil, "sh", []string{"sh", "-c", "claude"}, nil)
		got, err := Identify(999)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Empty(t, got.Tool)
	})

	t.Run("tpgid error falls back to panePID", func(t *testing.T) {
		stubReaders(t, 0, errors.New("nope"), "claude", []string{"/usr/bin/claude"}, nil)
		got, err := Identify(999)
		require.NoError(t, err)
		// Tpgid lookup failed → PID falls back to panePID.
		assert.Equal(t, 999, got.PID)
		assert.Equal(t, "claude", got.Tool)
	})

	t.Run("tpgid <= 0 falls back to panePID", func(t *testing.T) {
		stubReaders(t, -1, nil, "claude", nil, nil)
		got, err := Identify(42)
		require.NoError(t, err)
		assert.Equal(t, 42, got.PID)
	})
}
