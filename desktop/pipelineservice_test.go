package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/desktop/feed"
	"github.com/colonyops/hive/internal/desktop/pipeline"
	"github.com/colonyops/hive/internal/desktop/pipeline/actions"
	"github.com/colonyops/hive/internal/desktop/pipeline/pipelinedb"
)

type recordingActionExecutor struct {
	calls int
	data  pipeline.OutputData
}

func (e *recordingActionExecutor) Execute(_ context.Context, _ actions.Action, data pipeline.OutputData, _ pipeline.ActionInvocationInput) (pipeline.ExecutionResult, error) {
	e.calls++
	e.data = data
	return pipeline.ExecutionResult{}, nil
}

type recordingLaunchOptions struct{ options pipeline.SessionLaunchOptions }

func (r recordingLaunchOptions) SessionLaunchOptions(context.Context) (pipeline.SessionLaunchOptions, error) {
	return r.options, nil
}

type recordingSessionLauncher struct {
	calls []pipeline.LaunchSessionRequest
}

func (l *recordingSessionLauncher) LaunchSession(_ context.Context, req pipeline.LaunchSessionRequest) (pipeline.SessionExecutionOutcome, error) {
	l.calls = append(l.calls, req)
	return pipeline.SessionExecutionOutcome{ID: "session-1", Name: req.Name}, nil
}

func configuredActionStore(t *testing.T) *actions.ActionStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "actions.yml")
	require.NoError(t, os.WriteFile(path, []byte(`version: 1
actions:
  - id: review-pr
    label: Review PR
    type: launch-session
    show_in_detail: true
    applies_to: [pr]
    repo_template: "git@example/repo.git"
    prompt_template: "Review {{ .Payload.title }}"
  - id: triage-any
    label: Triage
    type: shell
    show_in_detail: true
    command_template: "true"
  - id: hidden
    label: Hidden
    type: shell
    show_in_detail: false
    command_template: "true"
`), 0o644))
	store := actions.NewActionStore(path)
	require.NoError(t, store.Reload())
	return store
}

func TestPipelineService_SessionLaunchOptionsUsesNarrowDTO(t *testing.T) {
	store := configuredActionStore(t)
	db, err := pipelinedb.Open(t.TempDir(), pipelinedb.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	expected := pipeline.SessionLaunchOptions{Repositories: []pipeline.SessionLaunchRepository{{Name: "hive", Repository: "https://github.com/colonyops/hive.git"}}, DefaultRepository: "https://github.com/colonyops/hive.git", Agents: []string{"claude"}, DefaultAgent: "claude"}
	service := NewPipelineService(db, store, nil, recordingLaunchOptions{options: expected})
	got, err := service.SessionLaunchOptions()
	require.NoError(t, err)
	assert.Equal(t, expected, got)
}

func TestPipelineService_ActionViewsAndInvocationUseActionStore(t *testing.T) {
	store := configuredActionStore(t)
	db, err := pipelinedb.Open(t.TempDir(), pipelinedb.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	executor := &recordingActionExecutor{}
	worker := pipeline.NewWorker(db, store, pipeline.NewDispatcher(map[string]pipeline.Executor{
		"launch-session": executor,
	}), 0, zerolog.Nop())
	service := NewPipelineService(db, store, worker, nil)

	assert.Equal(t, []actions.View{
		{ID: "review-pr", Label: "Review PR", Type: "launch-session", ShowInDetail: true},
		{ID: "triage-any", Label: "Triage", Type: "shell", ShowInDetail: true},
	}, service.ActionViews("PR"))

	_, err = service.InvokeAction("review-pr", feed.Item{ID: "pr-1", Kind: "PR", Title: "Fix it"}, pipeline.ActionInvocationInput{})
	require.NoError(t, err)
	assert.Equal(t, 1, executor.calls)
	assert.Equal(t, "pr-1", executor.data.Key)
	assert.Equal(t, "Fix it", executor.data.Payload["title"])
	_, err = service.InvokeAction("review-pr", feed.Item{ID: "issue-1", Kind: "Issue"}, pipeline.ActionInvocationInput{})
	require.Error(t, err)
	_, err = service.InvokeAction("hidden", feed.Item{ID: "pr-2", Kind: "PR"}, pipeline.ActionInvocationInput{})
	assert.ErrorContains(t, err, "not available in the detail pane", "backend must reject a trusted caller bypassing ActionViews")
}

func TestPipelineService_AttemptedFailureReturnsPersistedActionRun(t *testing.T) {
	store := configuredActionStore(t)
	db, err := pipelinedb.Open(t.TempDir(), pipelinedb.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	// Use a dispatcher executor that records a dispatched side effect failure.
	failed := &attemptedFailureExecutor{}
	worker := pipeline.NewWorker(db, store, pipeline.NewDispatcher(map[string]pipeline.Executor{"launch-session": failed}), 0, zerolog.Nop())
	service := NewPipelineService(db, store, worker, nil)

	view, err := service.InvokeAction("review-pr", feed.Item{ID: "pr-1", Kind: "PR", Title: "Fix it"}, pipeline.ActionInvocationInput{})
	require.NoError(t, err)
	assert.Equal(t, "failed", view.Status)
	assert.Equal(t, "side effect failed", view.Error)
	assert.Equal(t, "partial stdout", view.Stdout)
	assert.Equal(t, "partial stderr", view.Stderr)

	afterNavigation, err := service.ActionRun(view.CommandID)
	require.NoError(t, err)
	assert.Equal(t, view, afterNavigation)
}

type attemptedFailureExecutor struct{}

func (attemptedFailureExecutor) Execute(context.Context, actions.Action, pipeline.OutputData, pipeline.ActionInvocationInput) (pipeline.ExecutionResult, error) {
	return pipeline.ExecutionResult{Attempted: true, Log: pipeline.ExecutionLog{Stdout: "partial stdout", Stderr: "partial stderr"}}, fmt.Errorf("side effect failed")
}

func TestPipelineService_ActionRunSurvivesDatabaseReopen(t *testing.T) {
	store := configuredActionStore(t)
	dir := t.TempDir()
	db, err := pipelinedb.Open(dir, pipelinedb.DefaultOpenOptions())
	require.NoError(t, err)
	failed := &attemptedFailureExecutor{}
	worker := pipeline.NewWorker(db, store, pipeline.NewDispatcher(map[string]pipeline.Executor{"launch-session": failed}), 0, zerolog.Nop())
	view, err := NewPipelineService(db, store, worker, nil).InvokeAction("review-pr", feed.Item{ID: "pr-1", Kind: "PR", Title: "Fix it"}, pipeline.ActionInvocationInput{})
	require.NoError(t, err)
	require.NoError(t, db.Close())

	reopened, err := pipelinedb.Open(dir, pipelinedb.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })
	afterRestart, err := NewPipelineService(reopened, store, nil, nil).ActionRun(view.CommandID)
	require.NoError(t, err)
	assert.Equal(t, view, afterRestart)
}

func TestPipelineService_ConfirmedLaunchSessionExecutesRealActionPath(t *testing.T) {
	store := configuredActionStore(t)
	db, err := pipelinedb.Open(t.TempDir(), pipelinedb.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	launcher := &recordingSessionLauncher{}
	worker := pipeline.NewWorker(db, store, pipeline.NewDispatcher(map[string]pipeline.Executor{
		"launch-session": pipeline.NewLaunchSessionExecutor(launcher),
	}), 0, zerolog.Nop())
	service := NewPipelineService(db, store, worker, nil)

	_, err = service.InvokeAction("review-pr", feed.Item{ID: "pr-1", Kind: "PR", Title: "Fix it"}, pipeline.ActionInvocationInput{})
	require.NoError(t, err)
	require.Equal(t, []pipeline.LaunchSessionRequest{{
		Name: "review-pr-pr-1", Prompt: "Review Fix it", Repo: "git@example/repo.git",
	}}, launcher.calls)

	var status string
	require.NoError(t, db.Conn().QueryRowContext(t.Context(),
		`SELECT status FROM output_command WHERE action_id = ? AND key = ?`, "review-pr", "pr-1").Scan(&status))
	assert.Equal(t, "done", status)
}
