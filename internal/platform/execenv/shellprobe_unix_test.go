//go:build darwin || linux

package execenv

import (
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestShellEnvironmentStartsASeparateSession(t *testing.T) {
	shell := filepath.Join(t.TempDir(), "shell")
	require.NoError(t, os.WriteFile(shell, []byte(
		"#!/bin/sh\n"+
			"/bin/sleep 10 >/dev/null 2>&1 &\n"+
			"printf 'PATH=/usr/bin\\nHIVE_PROBE_PID=%s\\nHIVE_PROBE_CHILD_PID=%s\\n' \"$$\" \"$!\"\n"), 0o755))

	env, err := shellEnvironment(t.Context(), shell)
	require.NoError(t, err)
	probePID, err := strconv.Atoi(env["HIVE_PROBE_PID"])
	require.NoError(t, err)
	childPID, err := strconv.Atoi(env["HIVE_PROBE_CHILD_PID"])
	require.NoError(t, err)
	t.Cleanup(func() { _ = syscall.Kill(childPID, syscall.SIGKILL) })

	sid, err := unix.Getsid(childPID)
	require.NoError(t, err)
	assert.Equal(t, probePID, sid, "an interactive probe must not share the caller's controlling terminal")
}
