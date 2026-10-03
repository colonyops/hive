package app

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/platform/execenv"
)

// toolDir installs an executable named tool that prints marker, and returns the
// directory holding it — a stand-in for the package-manager prefix a desktop
// launch's own PATH does not contain.
func toolDir(t *testing.T, marker string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tool"), []byte("#!/bin/sh\necho "+marker+"\n"), 0o755))
	return dir
}

func resolverFor(dir string) *execenv.Resolver {
	return execenv.NewResolver(execenv.Options{Shell: "/bin/sh", Probe: func(context.Context, string) (map[string]string, error) {
		return map[string]string{"PATH": dir + ":/usr/bin:/bin"}, nil
	}})
}

// The bug this exists for: a hook is `sh -c <user command>`, so the command it
// runs is only found if the child's PATH is the resolved one.
func TestEnvExecutorRunsCommandsWithTheResolvedPath(t *testing.T) {
	dir := toolDir(t, "found")
	exec := newEnvExecutor(resolverFor(dir))
	t.Setenv("PATH", "/nonexistent")

	out, err := exec.Run(t.Context(), "sh", "-c", "tool")
	require.NoError(t, err)
	assert.Equal(t, "found", strings.TrimSpace(string(out)))

	out, err = exec.RunDir(t.Context(), t.TempDir(), "sh", "-c", "tool")
	require.NoError(t, err)
	assert.Equal(t, "found", strings.TrimSpace(string(out)))

	var streamed strings.Builder
	require.NoError(t, exec.RunStream(t.Context(), &streamed, io.Discard, "sh", "-c", "tool"))
	assert.Equal(t, "found", strings.TrimSpace(streamed.String()))

	streamed.Reset()
	require.NoError(t, exec.RunDirStream(t.Context(), t.TempDir(), &streamed, io.Discard, "sh", "-c", "tool"))
	assert.Equal(t, "found", strings.TrimSpace(streamed.String()))
}

func TestEnvExecutorRunsInTheGivenDirectory(t *testing.T) {
	dir := t.TempDir()
	exec := newEnvExecutor(resolverFor(toolDir(t, "unused")))

	require.NoError(t, exec.RunDirStream(t.Context(), dir, io.Discard, io.Discard, "sh", "-c", "touch marker"))
	_, err := os.Stat(filepath.Join(dir, "marker"))
	require.NoError(t, err)
}

// Hive streams hook output to io.Discard, so without this a missing command
// reaches the jobs list as a bare exit status naming nothing.
func TestEnvExecutorErrorNamesWhatTheShellCouldNotFind(t *testing.T) {
	exec := newEnvExecutor(resolverFor(t.TempDir()))

	err := exec.RunDirStream(t.Context(), t.TempDir(), io.Discard, io.Discard, "sh", "-c", "definitely-not-installed")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "definitely-not-installed")
	assert.Contains(t, err.Error(), "not found")
}

func TestEnvExecutorStillStreamsToItsCaller(t *testing.T) {
	exec := newEnvExecutor(resolverFor(t.TempDir()))

	var stderr strings.Builder
	err := exec.RunStream(t.Context(), io.Discard, &stderr, "sh", "-c", "echo complaint >&2; exit 3")
	require.Error(t, err)
	assert.Equal(t, "complaint", strings.TrimSpace(stderr.String()))
}

// hive clones with `_, err := e.exec.Run(...)`, so git's own reason sat in a
// return value nobody read.
func TestEnvExecutorCarriesAFailedCommandsOutputInTheError(t *testing.T) {
	exec := newEnvExecutor(resolverFor(t.TempDir()))

	_, err := exec.Run(t.Context(), "sh", "-c", "echo 'Repository not found' >&2; exit 128")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Repository not found")
	assert.Contains(t, err.Error(), "exit status 128")

	_, err = exec.RunDir(t.Context(), t.TempDir(), "sh", "-c", "echo 'fatal: not a git repository' >&2; exit 1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fatal: not a git repository")
}

func TestEnvExecutorLeavesTheErrorAloneWhenACommandSaidNothing(t *testing.T) {
	exec := newEnvExecutor(resolverFor(t.TempDir()))

	_, err := exec.Run(t.Context(), "sh", "-c", "exit 1")
	require.Error(t, err)
	assert.Equal(t, "exec sh: exit status 1", err.Error(), "a silent failure gains no trailing colon")
}

func TestEnvExecutorBoundsTheOutputItCarries(t *testing.T) {
	exec := newEnvExecutor(resolverFor(t.TempDir()))

	_, err := exec.Run(t.Context(), "sh", "-c", "head -c 100000 /dev/zero | tr '\\0' 'x'; exit 1")
	require.Error(t, err)
	assert.Less(t, len(err.Error()), maxDiagnosticBytes*2, "a wall of output must not become the job record")
}

// The incident's actual command, against real git. A local path keeps the test
// off the network.
func TestEnvExecutorCarriesGitsOwnReasonForARefusedClone(t *testing.T) {
	exec := newEnvExecutor(resolverFor(t.TempDir()))
	missing := filepath.Join(t.TempDir(), "no-such-repo")

	_, err := exec.Run(t.Context(), "git", "clone", missing, filepath.Join(t.TempDir(), "dest"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist", "git's own words, not just its exit status")
	assert.Contains(t, err.Error(), missing)
}
