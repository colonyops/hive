package executil

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithoutEnv(t *testing.T) {
	environ := []string{"PATH=/bin", "TMUX=/tmp/s,1,0", "TMUX_PANE=%1", "TMUXX=keep", "EMPTY=", "NOEQUALS"}

	got := WithoutEnv(environ, "TMUX", "TMUX_PANE", "NOEQUALS")

	assert.Equal(t, []string{"PATH=/bin", "TMUXX=keep", "EMPTY="}, got)
	assert.Len(t, environ, 6, "the input is not modified")
}

func TestWithoutEnvNilMeansThisProcess(t *testing.T) {
	t.Setenv("HIVE_WITHOUT_ENV_KEEP", "1")
	t.Setenv("HIVE_WITHOUT_ENV_DROP", "1")

	got := WithoutEnv(nil, "HIVE_WITHOUT_ENV_DROP")

	assert.Contains(t, got, "HIVE_WITHOUT_ENV_KEEP=1")
	assert.NotContains(t, got, "HIVE_WITHOUT_ENV_DROP=1")
}

func TestRealExecutorEnvReachesTheChild(t *testing.T) {
	e := &RealExecutor{Env: func(context.Context) []string {
		return append(os.Environ(), "HIVE_EXECUTIL_PROBE=seen")
	}}

	out, err := e.Run(t.Context(), "sh", "-c", "printf %s \"$HIVE_EXECUTIL_PROBE\"")
	require.NoError(t, err)
	assert.Equal(t, "seen", string(out))

	out, err = e.RunDir(t.Context(), t.TempDir(), "sh", "-c", "printf %s \"$HIVE_EXECUTIL_PROBE\"")
	require.NoError(t, err)
	assert.Equal(t, "seen", string(out))

	var streamed strings.Builder
	require.NoError(t, e.RunStream(t.Context(), &streamed, io.Discard, "sh", "-c", "printf %s \"$HIVE_EXECUTIL_PROBE\""))
	assert.Equal(t, "seen", streamed.String())
}

func TestRealExecutorLookPathResolvesOutsideThisProcessPath(t *testing.T) {
	dir := t.TempDir()
	tool := filepath.Join(dir, "hive-executil-tool")
	require.NoError(t, os.WriteFile(tool, []byte("#!/bin/sh\necho found\n"), 0o755))
	e := &RealExecutor{LookPath: func(_ context.Context, file string) (string, error) {
		return filepath.Join(dir, file), nil
	}}

	out, err := e.Run(t.Context(), "hive-executil-tool")
	require.NoError(t, err)
	assert.Equal(t, "found\n", string(out))
}

func TestRealExecutorStreamDiagnosticBytes(t *testing.T) {
	script := "echo complaint >&2; exit 3"

	err := (&RealExecutor{}).RunStream(t.Context(), io.Discard, io.Discard, "sh", "-c", script)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "complaint", "off by default")

	err = (&RealExecutor{StreamDiagnosticBytes: 4}).RunStream(t.Context(), io.Discard, nil, "sh", "-c", script)
	require.Error(t, err)
	assert.Equal(t, "exec sh: exit status 3: comp", err.Error())
}
