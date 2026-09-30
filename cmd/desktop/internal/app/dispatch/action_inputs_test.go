package dispatch

import (
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
)

func inputAction(id, actionType string, cfg actions.ActionConfig, inputs ...actions.InputSpec) actions.Action {
	return actions.Action{ID: id, Label: id, Type: actionType, ShowInDetail: true, Inputs: inputs, Config: cfg}
}

// The whole point of the feature: a value the user typed reaches the shell
// command, quoted like any other interpolated value.
func TestShellExecutor_RendersCollectedInputs(t *testing.T) {
	t.Parallel()

	action := inputAction("ignore", "shell",
		&actions.ShellConfig{CommandTemplate: "printf %s {{ .Inputs.reason | shq }}"},
		actions.InputSpec{Name: "reason", Type: actions.InputTypeText, Required: true})

	result, err := NewShellExecutor(zerolog.Nop(), hostEnvironment{}).Execute(t.Context(), action,
		OutputData{Payload: map[string]any{}, Inputs: map[string]string{"reason": "flapping; rm -rf /"}},
		ActionInvocationInput{})
	require.NoError(t, err)
	assert.Equal(t, "flapping; rm -rf /", result.Log.Stdout)
}

func TestRenderClipboardText_RendersCollectedInputs(t *testing.T) {
	t.Parallel()

	action := inputAction("ignore", "clipboard",
		&actions.ClipboardConfig{TextTemplate: "flux suspend hr {{ .Payload.name }} # {{ .Inputs.reason }}"},
		actions.InputSpec{Name: "reason", Type: actions.InputTypeText, Required: true})

	text, err := RenderClipboardText(action, "k", []byte(`{"name":"grafana"}`), map[string]string{"reason": "flapping"})
	require.NoError(t, err)
	assert.Equal(t, "flux suspend hr grafana # flapping", text)
}

// Resolution fills a key for every declared input, blank ones included, so a
// template only ever hits the renderer's missingkey=error for a name the
// action does not declare.
func TestRenderClipboardText_UndeclaredInputIsARenderError(t *testing.T) {
	t.Parallel()

	action := inputAction("copy", "clipboard", &actions.ClipboardConfig{TextTemplate: "echo {{ .Inputs.reason }}"})
	_, err := RenderClipboardText(action, "k", []byte(`{}`), nil)
	require.ErrorContains(t, err, "reason")

	declared := inputAction("copy", "clipboard", &actions.ClipboardConfig{TextTemplate: "echo {{ .Inputs.reason }}"},
		actions.InputSpec{Name: "reason", Type: actions.InputTypeText})
	resolved, err := declared.ResolveInputs(nil)
	require.NoError(t, err)
	text, err := RenderClipboardText(declared, "k", []byte(`{}`), resolved)
	require.NoError(t, err)
	assert.Equal(t, "echo", text)
}

// The automatic path collects nothing, so the worker is where a flow-fired
// command picks up the declared defaults.
func TestWorker_AutomaticRunResolvesDeclaredDefaults(t *testing.T) {
	t.Parallel()
	db := openTestPipelineDB(t)
	enqueueTestCommand(t, db, "notify-oncall", "item-1", `{"title":"Fix bug"}`)

	action := inputAction("notify-oncall", "publish-message",
		&actions.PublishMessageConfig{MessageTemplate: "{{ .Inputs.severity }}", Topic: "oncall"},
		actions.InputSpec{Name: "severity", Type: actions.InputTypeSelect, Default: "page", Options: []string{"page", "fyi"}})

	exec := &fakeExecutor{}
	worker := NewWorker(testOutputCommands(db), fakeActionLister{"notify-oncall": action},
		NewDispatcher(map[string]Executor{"publish-message": exec}), 0, zerolog.Nop())
	worker.Tick(t.Context())

	require.Equal(t, 1, exec.callCount())
	assert.Equal(t, map[string]string{"severity": "page"}, exec.calls[0].Inputs)
}

func TestWorker_ConfirmThreadsCollectedInputsToTheExecutor(t *testing.T) {
	t.Parallel()
	db := openTestPipelineDB(t)

	action := inputAction("ignore", "shell",
		&actions.ShellConfig{CommandTemplate: "true"},
		actions.InputSpec{Name: "reason", Type: actions.InputTypeText, Required: true})

	exec := &fakeExecutor{}
	worker := NewWorker(testOutputCommands(db), fakeActionLister{"ignore": action},
		NewDispatcher(map[string]Executor{"shell": exec}), 0, zerolog.Nop())

	view, err := worker.Confirm(t.Context(), "ignore", "item-1", []byte(`{"title":"Fix bug"}`), models.ItemRef{},
		ActionInvocationInput{Inputs: map[string]string{"reason": "flapping"}})
	require.NoError(t, err)
	assert.Equal(t, "done", view.Status)
	require.Equal(t, 1, exec.callCount())
	assert.Equal(t, map[string]string{"reason": "flapping"}, exec.calls[0].Inputs)
}

func TestWorker_MissingRequiredInputFailsWithoutDispatching(t *testing.T) {
	t.Parallel()
	db := openTestPipelineDB(t)

	action := inputAction("ignore", "shell",
		&actions.ShellConfig{CommandTemplate: "true"},
		actions.InputSpec{Name: "reason", Type: actions.InputTypeText, Required: true})

	exec := &fakeExecutor{}
	worker := NewWorker(testOutputCommands(db), fakeActionLister{"ignore": action},
		NewDispatcher(map[string]Executor{"shell": exec}), 0, zerolog.Nop())

	_, err := worker.Confirm(t.Context(), "ignore", "item-1", []byte(`{}`), models.ItemRef{}, ActionInvocationInput{})
	require.ErrorContains(t, err, `input "reason" is required`)
	assert.Equal(t, 0, exec.callCount())
}

// A required input makes an action interactive, which is what keeps the
// applicability probe from rendering repo_template against values nobody has
// supplied yet.
func TestActionApplicability_RepoProbeUsesDeclaredDefaults(t *testing.T) {
	t.Parallel()
	item := DecodedActionItem{ID: "item-1", Kind: "pr", Payload: []byte(`{"repo":"acme/app"}`)}

	withDefault := inputAction("launch", "launch-session",
		&actions.LaunchSessionConfig{PromptTemplate: "go", RepoTemplate: "https://github.com/{{ .Payload.repo }}.git#{{ .Inputs.branch }}"},
		actions.InputSpec{Name: "branch", Type: actions.InputTypeText, Default: "main"})
	ok, reason := ActionApplicability(withDefault, item)
	assert.True(t, ok, reason)

	inputs, err := withDefault.ResolveInputs(nil)
	require.NoError(t, err)
	repo, err := RenderRepoTarget(withDefault, item.ID, item.Payload, inputs)
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/acme/app.git#main", repo)

	required := inputAction("launch", "launch-session",
		&actions.LaunchSessionConfig{PromptTemplate: "go", RepoTemplate: "https://github.com/{{ .Payload.repo }}.git"},
		actions.InputSpec{Name: "reason", Type: actions.InputTypeText, Required: true})
	ok, reason = ActionApplicability(required, item)
	assert.True(t, ok, reason)
	assert.False(t, required.HeadlessCapable())
}
