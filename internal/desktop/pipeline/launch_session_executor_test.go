package pipeline

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/desktop/pipeline/actions"
	"github.com/colonyops/hive/internal/hive"
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

type fakeSessionCreator struct {
	calls []hive.CreateOptions
	err   error
}

func (f *fakeSessionCreator) CreateSession(_ context.Context, opts hive.CreateOptions) (*session.Session, error) {
	f.calls = append(f.calls, opts)
	if f.err != nil {
		return nil, f.err
	}
	return &session.Session{ID: "session-1"}, nil
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
			RepoTemplate:   " {{ .Payload.repo }} ",
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
	assert.Equal(t, "spawn-review-item-1", got.Name)
	assert.Equal(t, "Review Fix the bug (key=item-1)", got.Prompt)
	assert.Equal(t, "claude", got.Agent)
	assert.Equal(t, "colonyops/hive", got.Repo)
}

func TestLaunchSessionExecutor_NoRepoTemplate_LeavesRepoEmpty(t *testing.T) {
	launcher := &fakeSessionLauncher{}
	exec := NewLaunchSessionExecutor(launcher)

	action := actions.Action{
		ID:     "spawn-review",
		Type:   "launch-session",
		Config: &actions.LaunchSessionConfig{PromptTemplate: "hi"},
	}

	require.NoError(t, exec.Execute(t.Context(), action, OutputData{Key: "item-1", Payload: map[string]any{}}))
	require.Len(t, launcher.calls, 1)
	assert.Empty(t, launcher.calls[0].Repo)
}

func TestLaunchSessionExecutor_PropagatesLaunchFailure(t *testing.T) {
	launcher := &fakeSessionLauncher{err: errors.New("clone failed")}
	exec := NewLaunchSessionExecutor(launcher)
	action := actions.Action{ID: "spawn-review", Type: "launch-session", Config: &actions.LaunchSessionConfig{PromptTemplate: "hi"}}

	err := exec.Execute(t.Context(), action, OutputData{Key: "item-1", Payload: map[string]any{}})
	require.ErrorIs(t, err, launcher.err)
	require.Len(t, launcher.calls, 1)
}

func TestLaunchSessionExecutor_WrongConfigType_IsError(t *testing.T) {
	exec := NewLaunchSessionExecutor(&fakeSessionLauncher{})
	action := actions.Action{ID: "x", Type: "launch-session", Config: &actions.ShellConfig{}}

	err := exec.Execute(t.Context(), action, OutputData{})
	require.Error(t, err)
}

func TestLaunchSessionExecutor_NilLauncherIsError(t *testing.T) {
	exec := NewLaunchSessionExecutor(nil)
	action := actions.Action{ID: "spawn-review", Type: "launch-session", Config: &actions.LaunchSessionConfig{PromptTemplate: "hi"}}
	require.Error(t, exec.Execute(t.Context(), action, OutputData{Key: "item-1", Payload: map[string]any{}}))
}

func TestHiveSessionLauncher_MapsRequestToSessionService(t *testing.T) {
	creator := &fakeSessionCreator{}
	launcher := NewHiveSessionLauncher(creator)

	require.NoError(t, launcher.LaunchSession(t.Context(), LaunchSessionRequest{
		Name: "review-pr-1", Prompt: "Review this", Agent: "claude", Repo: "https://example.test/repo.git",
	}))
	require.Equal(t, []hive.CreateOptions{{
		Name: "review-pr-1", Prompt: "Review this", AgentKey: "claude", Remote: "https://example.test/repo.git", Background: true,
	}}, creator.calls)
}

func TestHiveSessionLauncher_PropagatesServiceFailure(t *testing.T) {
	creator := &fakeSessionCreator{err: errors.New("tmux unavailable")}
	err := NewHiveSessionLauncher(creator).LaunchSession(t.Context(), LaunchSessionRequest{Name: "review-pr-1"})
	require.ErrorIs(t, err, creator.err)
}
