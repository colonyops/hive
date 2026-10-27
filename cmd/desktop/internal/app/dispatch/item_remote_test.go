package dispatch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
)

// sshRemotes stands in for the engine's clone policy with a fixed SSH answer.
func sshRemotes(repo, _ string) string {
	if repo == "" {
		return ""
	}
	return "git@github.com:" + repo + ".git"
}

const itemRemotePayload = `{"repo":"acme/site","title":"Fix it","url":"https://github.com/acme/site/pull/7"}`

func TestOutputData_ItemRemote(t *testing.T) {
	payload := map[string]any{"repo": "acme/site", "url": "https://github.com/acme/site/pull/7"}

	t.Run("a nil generator builds HTTPS", func(t *testing.T) {
		assert.Equal(t, "https://github.com/acme/site.git", OutputData{Payload: payload}.ItemRemote())
	})
	t.Run("a nil generator passes an explicit remote through", func(t *testing.T) {
		explicit := map[string]any{"repo": " git@example.com:acme/site.git "}
		assert.Equal(t, "git@example.com:acme/site.git", OutputData{Payload: explicit}.ItemRemote())
	})
	t.Run("a nil generator yields blank when the item has no host", func(t *testing.T) {
		assert.Empty(t, OutputData{Payload: map[string]any{"repo": "acme/site"}}.ItemRemote())
	})
	t.Run("the generator decides", func(t *testing.T) {
		assert.Equal(t, "git@github.com:acme/site.git", OutputData{Payload: payload, remotes: sshRemotes}.ItemRemote())
	})
}

func TestRenderRepoTarget_UsesTheGenerator(t *testing.T) {
	action := probeLaunchSessionAction("review", "{{ .ItemRemote }}", "pr")

	repo, err := RenderRepoTarget(action, "pr-7", []byte(itemRemotePayload), nil, sshRemotes)
	require.NoError(t, err)
	assert.Equal(t, "git@github.com:acme/site.git", repo)
}

func TestActionApplicability_ItemRemoteFollowsTheGenerator(t *testing.T) {
	action := probeLaunchSessionAction("review", "{{ .ItemRemote }}", "pr")
	item := DecodedActionItem{ID: "pr-7", Kind: "PR", Payload: []byte(itemRemotePayload)}

	ok, _ := ActionApplicability(action, item, sshRemotes)
	assert.True(t, ok)

	ok, reason := ActionApplicability(action, item, func(string, string) string { return "" })
	assert.False(t, ok)
	assert.Contains(t, reason, "rendered blank")
}

func TestRenderClipboardText_ItemRemoteUsesTheGenerator(t *testing.T) {
	text, err := RenderClipboardText(clipboardAction("git clone {{ .ItemRemote }}"), "pr-7", []byte(itemRemotePayload), nil, sshRemotes)
	require.NoError(t, err)
	assert.Equal(t, "git clone git@github.com:acme/site.git", text)
}

func TestRenderSessionDraft_UsesTheGenerator(t *testing.T) {
	draft, err := RenderSessionDraft("Fix it", "https://github.com/acme/site/pull/7", []byte(itemRemotePayload), sshRemotes)
	require.NoError(t, err)
	assert.Equal(t, "git@github.com:acme/site.git", draft.Repository)
}

func TestDispatcher_GivesExecutorsTheGenerator(t *testing.T) {
	launcher := &fakeSessionLauncher{}
	dispatcher := NewDispatcher(map[string]Executor{
		"launch-session": NewLaunchSessionExecutor(zerolog.Nop(), launcher, nil, hostEnvironment{}),
	}, sshRemotes)
	action := actions.Action{
		ID: "review", Type: "launch-session",
		Config: &actions.LaunchSessionConfig{PromptTemplate: "hi", RepoTemplate: "{{ .ItemRemote }}"},
	}
	data := OutputData{Key: "pr-7", Payload: map[string]any{"repo": "acme/site"}, Raw: json.RawMessage(itemRemotePayload)}

	_, err := dispatcher.Execute(t.Context(), action, data, ActionInvocationInput{})
	require.NoError(t, err)
	require.Len(t, launcher.calls, 1)
	assert.Equal(t, "git@github.com:acme/site.git", launcher.calls[0].Repo)
}

func TestLaunchSessionExecutor_PostHookAndLogReportTheStoredRemote(t *testing.T) {
	dir := t.TempDir()
	launcher := &fakeSessionLauncher{path: dir, remote: "https://github.com/acme/site.git"}
	exec := NewLaunchSessionExecutor(zerolog.Nop(), launcher, nil, hostEnvironment{})
	action := launchWithPostHook("echo {{ .Session.Repo }} > repo", 0)
	cfg, ok := action.Config.(*actions.LaunchSessionConfig)
	require.True(t, ok)
	cfg.RepoTemplate = "git@github.com:acme/site.git"

	result, err := exec.Execute(t.Context(), action, reviewItem(), ActionInvocationInput{})
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/acme/site.git", result.Outcome.Session.Remote)

	written, err := os.ReadFile(filepath.Join(dir, "repo"))
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/acme/site.git", strings.TrimSpace(string(written)))
	assert.Contains(t, launchedLine(*result.Outcome.Session), "from https://github.com/acme/site.git")
}

func TestNotifyTerminal_ItemRemoteUsesTheGenerator(t *testing.T) {
	ctx := t.Context()
	db := openTestPipelineDB(t)
	flows := flowListerTest{flows: []flow.Flow{{
		ID: "triage", Enabled: true,
		Nodes: []flow.Node{{ID: "tell-me", Type: "notify", Name: "Tell me", Config: &flow.NotifyConfig{
			Title: "{{ .ItemRemote }}", Body: "body",
		}}},
	}}}
	notifier := &notifierTest{}
	dispatcher := NewDispatcher(map[string]Executor{
		ActionTypeNotify: NewNotifyExecutor(notifier, openGate(), stores.New(db, stores.Options{}).InboxItems, zerolog.Nop()),
	}, sshRemotes)
	worker := NewWorker(testOutputCommands(db), NewFlowActions(flows, actionListerTest{}), dispatcher, DefaultOutputWorkerInterval, zerolog.Nop())

	require.NoError(t, stores.New(db, stores.Options{}).EventLog.Commit(ctx, models.CommitBatch{
		Consumer: "triage", UpToOffset: 1,
		Outputs: []models.Output{{
			Sink:          models.Sink{Kind: models.SinkKindNotify, TargetID: "triage/tell-me"},
			Key:           "acme/site#7",
			OccurrenceKey: "acme/site#7:open",
			SourceKind:    "github",
			SourceScope:   "src",
			SourceTopic:   "source:triage/src",
			Payload:       []byte(itemRemotePayload),
		}},
	}))
	worker.Tick(ctx)
	waitForWorker(t, worker)

	require.Len(t, notifier.sent, 1)
	assert.Equal(t, "git@github.com:acme/site.git", notifier.sent[0].Title)
}
