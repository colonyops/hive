package pipeline

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/desktop/pipeline/actions"
)

// fakeSessionLauncher records every LaunchSession call.
type fakeSessionLauncher struct {
	calls []LaunchSessionRequest
	err   error
}

func (f *fakeSessionLauncher) LaunchSession(_ context.Context, req LaunchSessionRequest) error {
	f.calls = append(f.calls, req)
	return f.err
}

func TestLaunchSessionExecutor_RendersPromptAndRepoTemplates(t *testing.T) {
	launcher := &fakeSessionLauncher{}
	exec := NewLaunchSessionExecutor(launcher)

	action := actions.Action{
		ID:   "spawn-review",
		Type: "launch-session",
		Config: &actions.LaunchSessionConfig{
			PromptTemplate: "Review {{ .Payload.title }} (key={{ .Key }})",
			Agent:          "claude",
			RepoTemplate:   "{{ .Payload.repo }}",
			Post:           "comment",
		},
	}
	data := OutputData{
		Key:     "item-1",
		Payload: map[string]any{"title": "Fix the bug", "repo": "colonyops/hive"},
	}

	err := exec.Execute(t.Context(), action, data)
	require.NoError(t, err)

	require.Len(t, launcher.calls, 1)
	got := launcher.calls[0]
	assert.Equal(t, "Review Fix the bug (key=item-1)", got.Prompt)
	assert.Equal(t, "claude", got.Agent)
	assert.Equal(t, "colonyops/hive", got.Repo)
	assert.Equal(t, "comment", got.Post)
}

func TestLaunchSessionExecutor_NoRepoTemplate_LeavesRepoEmpty(t *testing.T) {
	launcher := &fakeSessionLauncher{}
	exec := NewLaunchSessionExecutor(launcher)

	action := actions.Action{
		ID:     "spawn-review",
		Type:   "launch-session",
		Config: &actions.LaunchSessionConfig{PromptTemplate: "hi"},
	}

	require.NoError(t, exec.Execute(t.Context(), action, OutputData{Payload: map[string]any{}}))
	require.Len(t, launcher.calls, 1)
	assert.Empty(t, launcher.calls[0].Repo)
}

func TestLaunchSessionExecutor_WrongConfigType_IsError(t *testing.T) {
	exec := NewLaunchSessionExecutor(&fakeSessionLauncher{})
	action := actions.Action{ID: "x", Type: "launch-session", Config: &actions.ShellConfig{}}

	err := exec.Execute(t.Context(), action, OutputData{})
	require.Error(t, err)
}

func TestNewLaunchSessionExecutor_NilLauncherDefaultsToLoggingStub(t *testing.T) {
	exec := NewLaunchSessionExecutor(nil)
	action := actions.Action{
		ID:     "spawn-review",
		Type:   "launch-session",
		Config: &actions.LaunchSessionConfig{PromptTemplate: "hi"},
	}
	// The logging stub returns nil without a real launcher configured.
	require.NoError(t, exec.Execute(t.Context(), action, OutputData{Payload: map[string]any{}}))
}

func TestLoggingSessionLauncher_ReturnsNil(t *testing.T) {
	l := LoggingSessionLauncher{Logger: zerolog.Nop()}
	require.NoError(t, l.LaunchSession(t.Context(), LaunchSessionRequest{Prompt: "hi"}))
}
