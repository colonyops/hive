package main

import (
	"context"
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

func (e *recordingActionExecutor) Execute(_ context.Context, _ actions.Action, data pipeline.OutputData) error {
	e.calls++
	e.data = data
	return nil
}

type recordingSessionLauncher struct {
	calls []pipeline.LaunchSessionRequest
}

func (l *recordingSessionLauncher) LaunchSession(_ context.Context, req pipeline.LaunchSessionRequest) error {
	l.calls = append(l.calls, req)
	return nil
}

func configuredActionStore(t *testing.T) *actions.ActionStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "actions.yml")
	require.NoError(t, os.WriteFile(path, []byte(`version: 1
actions:
  - id: review-pr
    label: Review PR
    type: launch-session
    applies_to: [pr]
    prompt_template: "Review {{ .Payload.title }}"
  - id: triage-any
    label: Triage
    type: shell
    command_template: "true"
`), 0o644))
	store := actions.NewActionStore(path)
	require.NoError(t, store.Reload())
	return store
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
	service := NewPipelineService(db, store, worker)

	assert.Equal(t, []actions.View{
		{ID: "review-pr", Label: "Review PR", Type: "launch-session", AutoApply: false},
		{ID: "triage-any", Label: "Triage", Type: "shell", AutoApply: false},
	}, service.ActionViews("PR"))

	require.NoError(t, service.InvokeAction("review-pr", feed.Item{ID: "pr-1", Kind: "PR", Title: "Fix it"}))
	assert.Equal(t, 1, executor.calls)
	assert.Equal(t, "pr-1", executor.data.Key)
	assert.Equal(t, "Fix it", executor.data.Payload["title"])
	assert.Error(t, service.InvokeAction("review-pr", feed.Item{ID: "issue-1", Kind: "Issue"}))
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
	service := NewPipelineService(db, store, worker)

	require.NoError(t, service.InvokeAction("review-pr", feed.Item{ID: "pr-1", Kind: "PR", Title: "Fix it"}))
	require.Equal(t, []pipeline.LaunchSessionRequest{{
		Name: "review-pr-pr-1", Prompt: "Review Fix it",
	}}, launcher.calls)

	var status string
	require.NoError(t, db.Conn().QueryRowContext(t.Context(),
		`SELECT status FROM output_command WHERE action_id = ? AND key = ?`, "review-pr", "pr-1").Scan(&status))
	assert.Equal(t, "done", status)
}
