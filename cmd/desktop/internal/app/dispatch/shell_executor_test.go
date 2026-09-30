package dispatch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
)

func TestShellExecutor_SuccessfulCommand(t *testing.T) {
	exec := NewShellExecutor(zerolog.Nop(), hostEnvironment{})
	action := actions.Action{
		ID:     "run-true",
		Type:   "shell",
		Config: &actions.ShellConfig{CommandTemplate: "true"},
	}

	_, err := exec.Execute(t.Context(), action, OutputData{Payload: map[string]any{}}, ActionInvocationInput{})
	require.NoError(t, err)
}

func TestShellExecutor_FailingCommand_IsError(t *testing.T) {
	exec := NewShellExecutor(zerolog.Nop(), hostEnvironment{})
	action := actions.Action{
		ID:     "run-false",
		Type:   "shell",
		Config: &actions.ShellConfig{CommandTemplate: "false"},
	}

	_, err := exec.Execute(t.Context(), action, OutputData{Payload: map[string]any{}}, ActionInvocationInput{})
	require.Error(t, err)
}

func TestShellExecutor_RendersCommandTemplateWithShq(t *testing.T) {
	dir := t.TempDir()
	outFile := dir + "/out.txt"

	exec := NewShellExecutor(zerolog.Nop(), hostEnvironment{})
	action := actions.Action{
		ID:   "echo-title",
		Type: "shell",
		Config: &actions.ShellConfig{
			CommandTemplate: `echo {{ .Payload.title | shq }} > ` + outFile,
		},
	}

	_, err := exec.Execute(t.Context(), action, OutputData{Payload: map[string]any{"title": "hello world"}}, ActionInvocationInput{})
	require.NoError(t, err)

	got, err := os.ReadFile(outFile)
	require.NoError(t, err)
	assert.Equal(t, "hello world\n", string(got))
}

func TestShellExecutor_RespectsCwd(t *testing.T) {
	dir := t.TempDir()

	exec := NewShellExecutor(zerolog.Nop(), hostEnvironment{})
	action := actions.Action{
		ID:   "touch-file",
		Type: "shell",
		Config: &actions.ShellConfig{
			CommandTemplate: "touch marker.txt",
			Cwd:             dir,
		},
	}

	_, err := exec.Execute(t.Context(), action, OutputData{Payload: map[string]any{}}, ActionInvocationInput{})
	require.NoError(t, err)

	_, err = os.Stat(dir + "/marker.txt")
	require.NoError(t, err)
}

// A terminal action runs in the session's own checkout, which is what makes
// `mise run test` a complete action rather than one that has to say where.
func TestShellExecutor_TerminalTargetDefaultsToTheSessionCheckout(t *testing.T) {
	dir := t.TempDir()

	exec := NewShellExecutor(zerolog.Nop(), hostEnvironment{})
	action := actions.Action{
		ID:     "touch-file",
		Type:   "shell",
		Config: &actions.ShellConfig{CommandTemplate: "touch marker.txt"},
	}

	_, err := exec.Execute(t.Context(), action, OutputData{Session: &SessionTarget{Path: dir}}, ActionInvocationInput{})
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dir, "marker.txt"))
	require.NoError(t, err)
}

func TestShellExecutor_ConfiguredCwdWinsOverTheSessionCheckout(t *testing.T) {
	session, configured := t.TempDir(), t.TempDir()

	exec := NewShellExecutor(zerolog.Nop(), hostEnvironment{})
	action := actions.Action{
		ID:     "touch-file",
		Type:   "shell",
		Config: &actions.ShellConfig{CommandTemplate: "touch marker.txt", Cwd: configured},
	}

	_, err := exec.Execute(t.Context(), action, OutputData{Session: &SessionTarget{Path: session}}, ActionInvocationInput{})
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(configured, "marker.txt"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(session, "marker.txt"))
	require.Error(t, err)
}

func TestShellExecutor_TimeoutKillsSlowCommand(t *testing.T) {
	exec := NewShellExecutor(zerolog.Nop(), hostEnvironment{})
	action := actions.Action{
		ID:   "slow",
		Type: "shell",
		Config: &actions.ShellConfig{
			CommandTemplate: "sleep 5",
			Timeout:         actions.Duration(50 * time.Millisecond),
		},
	}

	start := time.Now()
	_, err := exec.Execute(t.Context(), action, OutputData{Payload: map[string]any{}}, ActionInvocationInput{})
	require.Error(t, err)
	assert.Less(t, time.Since(start), 4*time.Second, "the timeout should have killed the command well before it finished")
}

func TestShellExecutor_BoundsAndDrainsNoisyStreams(t *testing.T) {
	exec := NewShellExecutor(zerolog.Nop(), hostEnvironment{})
	action := actions.Action{
		ID:     "noisy",
		Type:   "shell",
		Config: &actions.ShellConfig{CommandTemplate: "yes stdout | head -c 70000; yes stderr | head -c 70000 >&2"},
	}

	result, err := exec.Execute(t.Context(), action, OutputData{Payload: map[string]any{}}, ActionInvocationInput{})
	require.NoError(t, err)
	assert.True(t, result.Attempted)
	assert.Len(t, result.Log.Stdout, maxExecutionStreamBytes)
	assert.Len(t, result.Log.Stderr, maxExecutionStreamBytes)
	assert.True(t, strings.HasSuffix(result.Log.Stdout, truncatedStreamMarker))
	assert.True(t, strings.HasSuffix(result.Log.Stderr, truncatedStreamMarker))
}

func TestBoundedExecutionWriterNeverBuffersMoreThanLimit(t *testing.T) {
	writer := &boundedExecutionWriter{}
	stream := []byte(strings.Repeat("x", maxExecutionStreamBytes*2))
	n, err := writer.Write(stream)
	require.NoError(t, err)
	assert.Equal(t, len(stream), n, "all child output must be drained")
	assert.LessOrEqual(t, writer.buf.Len(), maxExecutionStreamBytes)
	assert.Len(t, writer.String(), maxExecutionStreamBytes)
	assert.True(t, strings.HasSuffix(writer.String(), truncatedStreamMarker))
}

func TestShellExecutor_WrongConfigType_IsError(t *testing.T) {
	exec := NewShellExecutor(zerolog.Nop(), hostEnvironment{})
	action := actions.Action{ID: "x", Type: "shell", Config: &actions.PublishMessageConfig{}}

	_, err := exec.Execute(t.Context(), action, OutputData{}, ActionInvocationInput{})
	require.Error(t, err)
}

type hostEnvironment struct{}

func (hostEnvironment) Environ(context.Context) []string { return os.Environ() }

type stubExecEnvironment struct{ path string }

func (s stubExecEnvironment) Environ(context.Context) []string {
	return append(os.Environ(), "PATH="+s.path)
}

// A shell action's command is the user's own, so it has the same problem a
// session hook does: the PATH a desktop launch inherits is not the one the
// command was written against.
func TestShellExecutor_RunsWithTheResolvedEnvironment(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tool"), []byte("#!/bin/sh\necho found\n"), 0o755))

	exec := NewShellExecutor(zerolog.Nop(), stubExecEnvironment{path: dir + ":/usr/bin:/bin"})
	action := actions.Action{
		ID:     "run-tool",
		Type:   "shell",
		Config: &actions.ShellConfig{CommandTemplate: "tool", Env: map[string]string{"MARKER": "set"}},
	}

	result, err := exec.Execute(t.Context(), action, OutputData{Payload: map[string]any{}}, ActionInvocationInput{})
	require.NoError(t, err)
	assert.Equal(t, "found", strings.TrimSpace(result.Log.Stdout))

	action.Config = &actions.ShellConfig{CommandTemplate: `printf %s "$MARKER"`, Env: map[string]string{"MARKER": "set"}}
	result, err = exec.Execute(t.Context(), action, OutputData{Payload: map[string]any{}}, ActionInvocationInput{})
	require.NoError(t, err)
	assert.Equal(t, "set", result.Log.Stdout, "the action's own env still overlays the resolved one")
}
