package pipeline

import (
	"os"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/desktop/pipeline/actions"
)

func TestShellExecutor_SuccessfulCommand(t *testing.T) {
	exec := NewShellExecutor(zerolog.Nop())
	action := actions.Action{
		ID:     "run-true",
		Type:   "shell",
		Config: &actions.ShellConfig{CommandTemplate: "true"},
	}

	err := exec.Execute(t.Context(), action, OutputData{Payload: map[string]any{}})
	require.NoError(t, err)
}

func TestShellExecutor_FailingCommand_IsError(t *testing.T) {
	exec := NewShellExecutor(zerolog.Nop())
	action := actions.Action{
		ID:     "run-false",
		Type:   "shell",
		Config: &actions.ShellConfig{CommandTemplate: "false"},
	}

	err := exec.Execute(t.Context(), action, OutputData{Payload: map[string]any{}})
	require.Error(t, err)
}

func TestShellExecutor_RendersCommandTemplateWithShq(t *testing.T) {
	dir := t.TempDir()
	outFile := dir + "/out.txt"

	exec := NewShellExecutor(zerolog.Nop())
	action := actions.Action{
		ID:   "echo-title",
		Type: "shell",
		Config: &actions.ShellConfig{
			CommandTemplate: `echo {{ .Payload.title | shq }} > ` + outFile,
		},
	}

	err := exec.Execute(t.Context(), action, OutputData{Payload: map[string]any{"title": "hello world"}})
	require.NoError(t, err)

	got, err := os.ReadFile(outFile)
	require.NoError(t, err)
	assert.Equal(t, "hello world\n", string(got))
}

func TestShellExecutor_RespectsCwd(t *testing.T) {
	dir := t.TempDir()

	exec := NewShellExecutor(zerolog.Nop())
	action := actions.Action{
		ID:   "touch-file",
		Type: "shell",
		Config: &actions.ShellConfig{
			CommandTemplate: "touch marker.txt",
			Cwd:             dir,
		},
	}

	require.NoError(t, exec.Execute(t.Context(), action, OutputData{Payload: map[string]any{}}))

	_, err := os.Stat(dir + "/marker.txt")
	require.NoError(t, err)
}

func TestShellExecutor_TimeoutKillsSlowCommand(t *testing.T) {
	exec := NewShellExecutor(zerolog.Nop())
	action := actions.Action{
		ID:   "slow",
		Type: "shell",
		Config: &actions.ShellConfig{
			CommandTemplate: "sleep 5",
			Timeout:         actions.Duration(50 * time.Millisecond),
		},
	}

	start := time.Now()
	err := exec.Execute(t.Context(), action, OutputData{Payload: map[string]any{}})
	require.Error(t, err)
	assert.Less(t, time.Since(start), 4*time.Second, "the timeout should have killed the command well before it finished")
}

func TestShellExecutor_WrongConfigType_IsError(t *testing.T) {
	exec := NewShellExecutor(zerolog.Nop())
	action := actions.Action{ID: "x", Type: "shell", Config: &actions.PublishEventConfig{}}

	err := exec.Execute(t.Context(), action, OutputData{})
	require.Error(t, err)
}
