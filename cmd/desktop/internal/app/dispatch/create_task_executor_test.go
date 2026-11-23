package dispatch

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/internal/domain/hc"
)

type taskCreatorTest struct {
	repoKey string
	input   hc.CreateItemInput
	calls   int
	err     error
}

func (c *taskCreatorTest) CreateItem(_ context.Context, repoKey string, input hc.CreateItemInput) (hc.Item, error) {
	c.calls++
	c.repoKey, c.input = repoKey, input
	if c.err != nil {
		return hc.Item{}, c.err
	}
	return hc.Item{ID: "hc-1", RepoKey: repoKey, Title: input.Title}, nil
}

func tasksOf(creator TaskCreator) func() TaskCreator {
	return func() TaskCreator { return creator }
}

func createTaskAction(cfg CreateTaskActionConfig) actions.Action {
	return actions.Action{ID: "task:triage/todo", Type: ActionTypeCreateTask, Config: &cfg}
}

func TestCreateTaskExecutor_RendersTheTaskOverTheItem(t *testing.T) {
	t.Parallel()
	creator := &taskCreatorTest{}
	action := createTaskAction(CreateTaskActionConfig{
		TitleTemplate:       "Fix {{ .Payload.title }}",
		DescriptionTemplate: "  {{ .Payload.url }}\n",
		RepoTemplate:        "{{ .Payload.repo }}",
		Parent:              "hc-epic",
	})
	data := OutputData{Payload: map[string]any{"title": "flaky\ntest", "url": "https://x/1", "repo": "acme/api"}}

	result, err := NewCreateTaskExecutor(tasksOf(creator)).Execute(t.Context(), action, data, ActionInvocationInput{})
	require.NoError(t, err)
	assert.True(t, result.Attempted)
	assert.Equal(t, "acme/api", creator.repoKey)
	assert.Equal(t, hc.CreateItemInput{Title: "Fix flaky test", Desc: "https://x/1", Type: hc.ItemTypeTask, ParentID: "hc-epic"}, creator.input)
}

func TestCreateTaskExecutor_AllowsNoRepository(t *testing.T) {
	t.Parallel()
	creator := &taskCreatorTest{}
	_, err := NewCreateTaskExecutor(tasksOf(creator)).Execute(t.Context(), createTaskAction(CreateTaskActionConfig{TitleTemplate: "t"}), OutputData{}, ActionInvocationInput{})
	require.NoError(t, err)
	assert.Empty(t, creator.repoKey)
}

func TestCreateTaskExecutor_FailsBeforeCreating(t *testing.T) {
	t.Parallel()
	for name, cfg := range map[string]CreateTaskActionConfig{
		"blank title":       {TitleTemplate: "{{ .Payload.missing }}"},
		"repo not a key":    {TitleTemplate: "t", RepoTemplate: "https://github.com/acme/api"},
		"repo with a space": {TitleTemplate: "t", RepoTemplate: "acme/my api"},
	} {
		creator := &taskCreatorTest{}
		result, err := NewCreateTaskExecutor(tasksOf(creator)).Execute(t.Context(), createTaskAction(cfg), OutputData{Payload: map[string]any{}}, ActionInvocationInput{})
		require.Errorf(t, err, "%s", name)
		assert.Falsef(t, result.Attempted, "%s", name)
		assert.Zerof(t, creator.calls, "%s", name)
	}
}

func TestCreateTaskExecutor_PropagatesCreateFailure(t *testing.T) {
	t.Parallel()
	creator := &taskCreatorTest{err: errors.New("parent not found")}
	result, err := NewCreateTaskExecutor(tasksOf(creator)).Execute(t.Context(), createTaskAction(CreateTaskActionConfig{TitleTemplate: "t"}), OutputData{}, ActionInvocationInput{})
	require.ErrorIs(t, err, creator.err)
	assert.True(t, result.Attempted)
}

func TestCreateTaskExecutor_RejectsWrongConfigOrMissingService(t *testing.T) {
	t.Parallel()
	_, err := NewCreateTaskExecutor(tasksOf(&taskCreatorTest{})).Execute(t.Context(), actions.Action{ID: "x", Config: &actions.ShellConfig{}}, OutputData{}, ActionInvocationInput{})
	require.Error(t, err)
	_, err = NewCreateTaskExecutor(nil).Execute(t.Context(), createTaskAction(CreateTaskActionConfig{TitleTemplate: "t"}), OutputData{}, ActionInvocationInput{})
	require.Error(t, err)
}
