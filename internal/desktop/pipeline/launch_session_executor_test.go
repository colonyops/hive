package pipeline

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/core/git"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/desktop/pipeline/actions"
	"github.com/colonyops/hive/internal/hive"
)

// fakeSessionLauncher records every LaunchSession call.
type fakeSessionLauncher struct {
	calls []LaunchSessionRequest
	err   error
}

func (f *fakeSessionLauncher) LaunchSession(_ context.Context, req LaunchSessionRequest) (SessionExecutionOutcome, error) {
	f.calls = append(f.calls, req)
	return SessionExecutionOutcome{ID: "session-1", Name: req.Name}, f.err
}

type fakeSessionCreator struct {
	calls   []hive.CreateOptions
	err     error
	options hive.SessionLaunchOptions
}

func (f *fakeSessionCreator) SessionLaunchOptions(context.Context) (hive.SessionLaunchOptions, error) {
	return f.options, nil
}

func (f *fakeSessionCreator) ResolveSessionLaunchRepository(_ context.Context, remote string) (hive.SessionLaunchRepository, error) {
	for _, repo := range f.options.Repositories {
		if git.EquivalentRemote(repo.Remote, remote) {
			return repo, nil
		}
	}
	return hive.SessionLaunchRepository{Remote: remote}, nil
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

	_, err := exec.Execute(t.Context(), action, data, ActionInvocationInput{})
	require.NoError(t, err)

	require.Len(t, launcher.calls, 1)
	got := launcher.calls[0]
	assert.Equal(t, "spawn-review-item-1", got.Name)
	assert.Equal(t, "Review Fix the bug (key=item-1)", got.Prompt)
	assert.Equal(t, "claude", got.Agent)
	assert.Equal(t, "colonyops/hive", got.Repo)
}

func TestLaunchSessionExecutor_RerunUsesUniqueSessionName(t *testing.T) {
	launcher := &fakeSessionLauncher{}
	exec := NewLaunchSessionExecutor(launcher)
	action := actions.Action{ID: "spawn-review", Type: "launch-session", Config: &actions.LaunchSessionConfig{
		PromptTemplate: "hi", RepoTemplate: "git@github.com:colonyops/hive.git",
	}}

	_, err := exec.Execute(t.Context(), action, OutputData{Key: "item-1", CommandID: 42, IsRerun: true, Payload: map[string]any{}}, ActionInvocationInput{Rerun: true})
	require.NoError(t, err)
	require.Equal(t, "spawn-review-item-1-rerun-42", launcher.calls[0].Name)
}

func TestLaunchSessionExecutor_ConfiguredRepoTemplateIgnoresInteractiveOverride(t *testing.T) {
	launcher := &fakeSessionLauncher{}
	exec := NewLaunchSessionExecutor(launcher)
	action := actions.Action{ID: "spawn-review", Type: "launch-session", Config: &actions.LaunchSessionConfig{
		PromptTemplate: "hi", RepoTemplate: "git@github.com:colonyops/hive.git", Agent: "configured-agent",
	}}
	_, err := exec.Execute(t.Context(), action, OutputData{Key: "item-1", Payload: map[string]any{}}, ActionInvocationInput{Session: &SessionInvocationInput{
		Name: "frontend-name", Repository: "https://github.com/other/repo.git", Agent: "frontend-agent",
	}})
	require.NoError(t, err)
	require.Equal(t, LaunchSessionRequest{Name: "spawn-review-item-1", Prompt: "hi", Repo: "git@github.com:colonyops/hive.git", Agent: "configured-agent"}, launcher.calls[0])
}

func TestLaunchSessionExecutor_NoRepoTemplate_LeavesRepoEmpty(t *testing.T) {
	launcher := &fakeSessionLauncher{}
	exec := NewLaunchSessionExecutor(launcher)

	action := actions.Action{
		ID:     "spawn-review",
		Type:   "launch-session",
		Config: &actions.LaunchSessionConfig{PromptTemplate: "hi"},
	}

	_, err := exec.Execute(t.Context(), action, OutputData{Key: "item-1", Payload: map[string]any{}}, ActionInvocationInput{Session: &SessionInvocationInput{Name: "spawn-review-item-1", Repository: "git@example/repo"}})
	require.NoError(t, err)
	require.Len(t, launcher.calls, 1)
	assert.Equal(t, "git@example/repo", launcher.calls[0].Repo)
}

func TestLaunchSessionExecutor_PropagatesLaunchFailure(t *testing.T) {
	launcher := &fakeSessionLauncher{err: errors.New("clone failed")}
	exec := NewLaunchSessionExecutor(launcher)
	action := actions.Action{ID: "spawn-review", Type: "launch-session", Config: &actions.LaunchSessionConfig{PromptTemplate: "hi"}}

	_, err := exec.Execute(t.Context(), action, OutputData{Key: "item-1", Payload: map[string]any{}}, ActionInvocationInput{Session: &SessionInvocationInput{Name: "spawn-review-item-1", Repository: "git@example/repo"}})
	require.ErrorIs(t, err, launcher.err)
	require.Len(t, launcher.calls, 1)
}

func TestLaunchSessionExecutor_WrongConfigType_IsError(t *testing.T) {
	exec := NewLaunchSessionExecutor(&fakeSessionLauncher{})
	action := actions.Action{ID: "x", Type: "launch-session", Config: &actions.ShellConfig{}}

	_, err := exec.Execute(t.Context(), action, OutputData{}, ActionInvocationInput{})
	require.Error(t, err)
}

func TestLaunchSessionExecutor_NilLauncherIsError(t *testing.T) {
	exec := NewLaunchSessionExecutor(nil)
	action := actions.Action{ID: "spawn-review", Type: "launch-session", Config: &actions.LaunchSessionConfig{PromptTemplate: "hi"}}
	_, err := exec.Execute(t.Context(), action, OutputData{Key: "item-1", Payload: map[string]any{}}, ActionInvocationInput{})
	require.Error(t, err)
}

func TestHiveSessionLauncher_MapsRequestToSessionService(t *testing.T) {
	creator := &fakeSessionCreator{}
	launcher := NewHiveSessionLauncher(creator)

	_, err := launcher.LaunchSession(t.Context(), LaunchSessionRequest{
		Name: "review-pr-1", Prompt: "Review this", Agent: "claude", Repo: "https://example.test/repo.git",
	})
	require.NoError(t, err)
	require.Equal(t, []hive.CreateOptions{{
		Name: "review-pr-1", Prompt: "Review this", AgentKey: "claude", Remote: "https://example.test/repo.git", Background: true,
	}}, creator.calls)
}

func TestHiveSessionLauncher_PrefersEquivalentConfiguredCheckout(t *testing.T) {
	creator := &fakeSessionCreator{options: hive.SessionLaunchOptions{Repositories: []hive.SessionLaunchRepository{{
		Name: "hive", Remote: "git@github.com:colonyops/hive.git", Source: "/work/hive",
	}}}}
	_, err := NewHiveSessionLauncher(creator).LaunchSession(t.Context(), LaunchSessionRequest{
		Name: "review-pr-1", Prompt: "Review this", Agent: "claude", Repo: "https://github.com/colonyops/hive.git",
	})
	require.NoError(t, err)
	require.Equal(t, hive.CreateOptions{
		Name: "review-pr-1", Prompt: "Review this", AgentKey: "claude", Remote: "git@github.com:colonyops/hive.git", Source: "/work/hive", Background: true,
	}, creator.calls[0])
}

func TestHiveSessionLauncher_PropagatesServiceFailure(t *testing.T) {
	creator := &fakeSessionCreator{err: errors.New("tmux unavailable")}
	_, err := NewHiveSessionLauncher(creator).LaunchSession(t.Context(), LaunchSessionRequest{Name: "review-pr-1"})
	require.ErrorIs(t, err, creator.err)
}
