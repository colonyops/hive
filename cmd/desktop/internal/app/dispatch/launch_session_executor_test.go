package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/internal/core/git"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive"
)

// fakeSessionLauncher records every LaunchSession call.
type fakeSessionLauncher struct {
	calls []LaunchSessionRequest
	path  string
	err   error
}

func (f *fakeSessionLauncher) LaunchSession(_ context.Context, req LaunchSessionRequest) (SessionExecutionOutcome, error) {
	f.calls = append(f.calls, req)
	return SessionExecutionOutcome{ID: "session-1", Name: req.Name, Slug: "slug-1", Path: f.path}, f.err
}

type fakeWorkspaceSessionLauncher struct {
	calls []LaunchWorkspaceSessionRequest
	err   error
}

func (f *fakeWorkspaceSessionLauncher) LaunchWorkspaceSession(_ context.Context, req LaunchWorkspaceSessionRequest) (SessionExecutionOutcome, error) {
	f.calls = append(f.calls, req)
	return SessionExecutionOutcome{ID: "42", Name: req.Name, Slug: "agentws-42"}, f.err
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
	exec := NewLaunchSessionExecutor(zerolog.Nop(), launcher, nil, hostEnvironment{})

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
		Raw:     json.RawMessage(`{"title":"Fix the bug","repo":"colonyops/hive"}`),
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

func TestLaunchSessionExecutor_ItemRemoteRendersTheItemsCloneURL(t *testing.T) {
	action := actions.Action{
		ID: "spawn-review", Type: "launch-session",
		Config: &actions.LaunchSessionConfig{PromptTemplate: "hi", RepoTemplate: "{{ .ItemRemote }}"},
	}

	t.Run("an item that names its repository", func(t *testing.T) {
		launcher := &fakeSessionLauncher{}
		raw := `{"repo":"colonyops/hive","url":"https://github.com/colonyops/hive/pull/7"}`
		_, err := NewLaunchSessionExecutor(zerolog.Nop(), launcher, nil, hostEnvironment{}).
			Execute(t.Context(), action, OutputData{Key: "pr-7", Raw: json.RawMessage(raw)}, ActionInvocationInput{})
		require.NoError(t, err)
		require.Len(t, launcher.calls, 1)
		assert.Equal(t, "https://github.com/colonyops/hive.git", launcher.calls[0].Repo)
	})

	t.Run("an item without one fails naming the cause", func(t *testing.T) {
		launcher := &fakeSessionLauncher{}
		_, err := NewLaunchSessionExecutor(zerolog.Nop(), launcher, nil, hostEnvironment{}).
			Execute(t.Context(), action, OutputData{Key: "alert-1", Raw: json.RawMessage(`{"title":"down"}`)}, ActionInvocationInput{})
		require.EqualError(t, err, "launch-session: repo_template rendered blank: the item names no repository")
		assert.Empty(t, launcher.calls)
	})

	t.Run("a flow node's failure names the node's own field", func(t *testing.T) {
		node := actions.Action{ID: "launch:triage/review", Type: "launch-session", Config: &LaunchNodeActionConfig{
			PromptTemplate: "hi", RepoTemplate: "{{ .ItemRemote }}",
		}}
		_, err := NewLaunchSessionExecutor(zerolog.Nop(), &fakeSessionLauncher{}, nil, hostEnvironment{}).
			Execute(t.Context(), node, OutputData{Key: "alert-1", Raw: json.RawMessage(`{"title":"down"}`)}, ActionInvocationInput{})
		require.EqualError(t, err, "launch-session: repo rendered blank: the item names no repository")
	})
}

func TestLaunchSessionExecutor_RerunUsesUniqueSessionName(t *testing.T) {
	launcher := &fakeSessionLauncher{}
	exec := NewLaunchSessionExecutor(zerolog.Nop(), launcher, nil, hostEnvironment{})
	action := actions.Action{ID: "spawn-review", Type: "launch-session", Config: &actions.LaunchSessionConfig{
		PromptTemplate: "hi", RepoTemplate: "git@github.com:colonyops/hive.git",
	}}

	_, err := exec.Execute(t.Context(), action, OutputData{Key: "item-1", CommandID: 42, IsRerun: true, Payload: map[string]any{}, Raw: json.RawMessage(`{}`)}, ActionInvocationInput{Rerun: true})
	require.NoError(t, err)
	require.Equal(t, "spawn-review-item-1-rerun-42", launcher.calls[0].Name)
}

func TestLaunchSessionExecutor_ConfiguredRepoTemplateIgnoresInteractiveOverride(t *testing.T) {
	launcher := &fakeSessionLauncher{}
	exec := NewLaunchSessionExecutor(zerolog.Nop(), launcher, nil, hostEnvironment{})
	action := actions.Action{ID: "spawn-review", Type: "launch-session", Config: &actions.LaunchSessionConfig{
		PromptTemplate: "hi", RepoTemplate: "git@github.com:colonyops/hive.git", Agent: "configured-agent",
	}}
	_, err := exec.Execute(t.Context(), action, OutputData{Key: "item-1", Payload: map[string]any{}, Raw: json.RawMessage(`{}`)}, ActionInvocationInput{Session: &SessionInvocationInput{
		Name: "frontend-name", Repository: "https://github.com/other/repo.git", Agent: "frontend-agent",
	}})
	require.NoError(t, err)
	require.Equal(t, LaunchSessionRequest{Name: "spawn-review-item-1", Prompt: "hi", Repo: "git@github.com:colonyops/hive.git", Agent: "configured-agent"}, launcher.calls[0])
}

func TestLaunchSessionExecutor_NoRepoTemplate_LeavesRepoEmpty(t *testing.T) {
	launcher := &fakeSessionLauncher{}
	exec := NewLaunchSessionExecutor(zerolog.Nop(), launcher, nil, hostEnvironment{})

	action := actions.Action{
		ID:     "spawn-review",
		Type:   "launch-session",
		Config: &actions.LaunchSessionConfig{PromptTemplate: "hi"},
	}

	_, err := exec.Execute(t.Context(), action, OutputData{Key: "item-1", Payload: map[string]any{}}, ActionInvocationInput{Session: &SessionInvocationInput{Name: "spawn-review-item-1", Repository: "git@example/repo"}})
	require.NoError(t, err)
	require.Len(t, launcher.calls, 1)
	assert.Equal(t, "git@example/repo", launcher.calls[0].Repo)
	assert.Empty(t, launcher.calls[0].CollisionSuffix, "a typed name keeps the duplicate error")
}

func TestLaunchSessionExecutor_GeneratedName(t *testing.T) {
	launcher := &fakeSessionLauncher{}
	exec := NewLaunchSessionExecutor(zerolog.Nop(), launcher, nil, hostEnvironment{})
	action := actions.Action{ID: "spawn-review", Type: "launch-session", Config: &LaunchNodeActionConfig{
		PromptTemplate: "hi", RepoTemplate: "example/repo", NameTemplate: "{{ .Payload.title }}",
	}}
	data := OutputData{Key: "item-1", Payload: map[string]any{"title": "修正 🚀"}, Raw: json.RawMessage(`{}`), CommandID: 42}

	_, err := exec.Execute(t.Context(), action, data, ActionInvocationInput{})
	require.NoError(t, err)
	require.Len(t, launcher.calls, 1)
	assert.Equal(t, "spawn-review-item-1", launcher.calls[0].Name, "a title with no letters or digits falls back to the derived name")
	assert.Equal(t, "42", launcher.calls[0].CollisionSuffix, "every retry of the command claims the same name")
}

func TestLaunchSessionExecutor_LaunchesConfiguredWorkspace(t *testing.T) {
	repositories := &fakeSessionLauncher{}
	workspaces := &fakeWorkspaceSessionLauncher{}
	exec := NewLaunchSessionExecutor(zerolog.Nop(), repositories, workspaces, hostEnvironment{})
	action := actions.Action{ID: "triage", Type: "launch-session", Config: &actions.LaunchSessionConfig{
		PromptTemplate: "Triage {{ .Payload.alert }}", Workspace: "alerts",
	}}

	result, err := exec.Execute(t.Context(), action, OutputData{
		Key: "alert-1", Payload: map[string]any{"alert": "disk full"}, Raw: json.RawMessage(`{"alert":"disk full"}`),
	}, ActionInvocationInput{})
	require.NoError(t, err)
	assert.Empty(t, repositories.calls)
	require.Equal(t, []LaunchWorkspaceSessionRequest{{Workspace: "alerts", Name: "triage-alert-1", Prompt: "Triage disk full"}}, workspaces.calls)
	require.NotNil(t, result.Outcome)
	assert.Equal(t, "agentws-42", result.Outcome.Session.Slug)
	assert.Empty(t, result.Log)
}

func TestLaunchSessionExecutor_LaunchesInteractiveWorkspace(t *testing.T) {
	workspaces := &fakeWorkspaceSessionLauncher{}
	exec := NewLaunchSessionExecutor(zerolog.Nop(), nil, workspaces, hostEnvironment{})
	action := actions.Action{ID: "triage", Type: "launch-session", Config: &actions.LaunchSessionConfig{PromptTemplate: "Triage"}}

	_, err := exec.Execute(t.Context(), action, OutputData{Key: "alert-1", Payload: map[string]any{}}, ActionInvocationInput{
		Session: &SessionInvocationInput{Name: "prod-alert", Workspace: "alerts"},
	})
	require.NoError(t, err)
	require.Equal(t, "prod-alert", workspaces.calls[0].Name)
}

func TestLaunchSessionExecutor_InteractiveTargetMustBeExclusive(t *testing.T) {
	exec := NewLaunchSessionExecutor(zerolog.Nop(), &fakeSessionLauncher{}, &fakeWorkspaceSessionLauncher{}, hostEnvironment{})
	action := actions.Action{ID: "triage", Type: "launch-session", Config: &actions.LaunchSessionConfig{PromptTemplate: "Triage"}}
	data := OutputData{Key: "alert-1", Payload: map[string]any{}}

	for name, input := range map[string]*SessionInvocationInput{
		"neither": {Name: "triage"},
		"both":    {Name: "triage", Repository: "repo", Workspace: "alerts"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := exec.Execute(t.Context(), action, data, ActionInvocationInput{Session: input})
			require.ErrorContains(t, err, "exactly one")
		})
	}
}

func TestLaunchSessionExecutor_PropagatesLaunchFailure(t *testing.T) {
	launcher := &fakeSessionLauncher{err: errors.New("clone failed")}
	exec := NewLaunchSessionExecutor(zerolog.Nop(), launcher, nil, hostEnvironment{})
	action := actions.Action{ID: "spawn-review", Type: "launch-session", Config: &actions.LaunchSessionConfig{PromptTemplate: "hi"}}

	_, err := exec.Execute(t.Context(), action, OutputData{Key: "item-1", Payload: map[string]any{}}, ActionInvocationInput{Session: &SessionInvocationInput{Name: "spawn-review-item-1", Repository: "git@example/repo"}})
	require.ErrorIs(t, err, launcher.err)
	require.Len(t, launcher.calls, 1)
}

func TestLaunchSessionExecutor_WrongConfigType_IsError(t *testing.T) {
	exec := NewLaunchSessionExecutor(zerolog.Nop(), &fakeSessionLauncher{}, nil, hostEnvironment{})
	action := actions.Action{ID: "x", Type: "launch-session", Config: &actions.ShellConfig{}}

	_, err := exec.Execute(t.Context(), action, OutputData{}, ActionInvocationInput{})
	require.Error(t, err)
}

func TestLaunchSessionExecutor_NilLauncherIsError(t *testing.T) {
	exec := NewLaunchSessionExecutor(zerolog.Nop(), nil, nil, hostEnvironment{})
	for name, cfg := range map[string]*actions.LaunchSessionConfig{
		"repository": {RepoTemplate: "repo", PromptTemplate: "hi"},
		"workspace":  {Workspace: "alerts", PromptTemplate: "hi"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := exec.Execute(t.Context(), actions.Action{ID: "spawn-review", Type: "launch-session", Config: cfg}, OutputData{Key: "item-1", Raw: []byte(`{}`), Payload: map[string]any{}}, ActionInvocationInput{})
			require.ErrorContains(t, err, "no "+name+" session launcher configured")
		})
	}
}

func TestHiveSessionLauncher_MapsRequestToSessionService(t *testing.T) {
	creator := &fakeSessionCreator{}
	launcher := NewHiveSessionLauncher(creator)

	_, err := launcher.LaunchSession(t.Context(), LaunchSessionRequest{
		Name: "review-pr-1", Prompt: "Review this", Agent: "claude", Repo: "https://example.test/repo.git", CollisionSuffix: "7",
	})
	require.NoError(t, err)
	require.Len(t, creator.calls, 1)
	require.NotNil(t, creator.calls[0].Progress, "every attempt gets its own progress writer, so a failure can name the step it died on")
	require.Equal(t, hive.CreateOptions{
		Name: "review-pr-1", Prompt: "Review this", AgentKey: "claude", Remote: "https://example.test/repo.git", Background: true, CollisionSuffix: "7",
	}, withoutProgress(creator.calls[0]))
}

// The per-attempt writer has no comparable identity, so drop it to compare the
// rest by value.
func withoutProgress(opts hive.CreateOptions) hive.CreateOptions {
	opts.Progress = nil
	return opts
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
	}, withoutProgress(creator.calls[0]))
}

func TestHiveSessionLauncher_PropagatesServiceFailure(t *testing.T) {
	creator := &fakeSessionCreator{err: errors.New("tmux unavailable")}
	_, err := NewHiveSessionLauncher(creator).LaunchSession(t.Context(), LaunchSessionRequest{Name: "review-pr-1"})
	require.ErrorIs(t, err, creator.err)
}

// fakeItemSessionLinker records the association the launcher persists.
type fakeItemSessionLinker struct {
	links    map[string][]models.ItemRef
	attempts []models.ItemRef
	err      error
}

func (f *fakeItemSessionLinker) Link(_ context.Context, sessionID string, ref models.ItemRef) error {
	f.attempts = append(f.attempts, ref)
	if f.err != nil {
		return f.err
	}
	if f.links == nil {
		f.links = map[string][]models.ItemRef{}
	}
	f.links[sessionID] = append(f.links[sessionID], ref)
	return nil
}

func TestHiveSessionLauncher_LinksTheCreatedSessionToItsItem(t *testing.T) {
	creator := &fakeSessionCreator{}
	linker := &fakeItemSessionLinker{}
	launcher := NewHiveSessionLauncher(creator)
	launcher.SetItemSessionLinker(linker, zerolog.Nop())
	ref := models.ItemRef{ProfileID: "p", SourceKind: "github", SourceScope: "acct", ExternalID: "acme/repo#1"}

	_, err := launcher.LaunchSession(t.Context(), LaunchSessionRequest{Name: "review-1", Prompt: "go", Repo: "r", Origins: []models.ItemRef{ref}})
	require.NoError(t, err)
	assert.Equal(t, map[string][]models.ItemRef{"session-1": {ref}}, linker.links)
	// The item id also goes on the session as a hive tag, for a reader inside
	// hive. It is presentational and never read back.
	require.Len(t, creator.calls, 1)
	assert.Equal(t, []string{"acme/repo#1"}, creator.calls[0].Tags)
}

// A session with no item behind it — the blank New Session form, an action run
// from a terminal target — must not produce a link, or every such session
// would share one.
func TestHiveSessionLauncher_LinksOneCreatedSessionToEveryUniqueOrigin(t *testing.T) {
	creator := &fakeSessionCreator{}
	linker := &fakeItemSessionLinker{}
	launcher := NewHiveSessionLauncher(creator)
	launcher.SetItemSessionLinker(linker, zerolog.Nop())
	first := models.ItemRef{ProfileID: "p", SourceKind: "github", SourceScope: "acct", ExternalID: "acme/repo#1"}
	second := models.ItemRef{ProfileID: "p", SourceKind: "github", SourceScope: "acct", ExternalID: "acme/repo#2"}

	_, err := launcher.LaunchSession(t.Context(), LaunchSessionRequest{Name: "review", Origins: []models.ItemRef{first, second, first}})
	require.NoError(t, err)
	require.Len(t, creator.calls, 1)
	assert.Equal(t, []string{"acme/repo#1", "acme/repo#2"}, creator.calls[0].Tags)
	assert.Equal(t, []models.ItemRef{first, second}, linker.links["session-1"])
}

func TestHiveSessionLauncher_LinksNothingWithoutAnOrigin(t *testing.T) {
	creator := &fakeSessionCreator{}
	linker := &fakeItemSessionLinker{}
	launcher := NewHiveSessionLauncher(creator)
	launcher.SetItemSessionLinker(linker, zerolog.Nop())

	_, err := launcher.LaunchSession(t.Context(), LaunchSessionRequest{Name: "review-1", Prompt: "go", Repo: "r"})
	require.NoError(t, err)
	assert.Empty(t, linker.links)
	require.Len(t, creator.calls, 1)
	assert.Empty(t, creator.calls[0].Tags)
}

// The session exists either way, so reporting the launch as failed would be a
// lie — and would invite a retry that creates a second session.
func TestHiveSessionLauncher_ReportsSuccessWhenTheLinkCannotBeWritten(t *testing.T) {
	launcher := NewHiveSessionLauncher(&fakeSessionCreator{})
	linker := &fakeItemSessionLinker{err: errors.New("disk full")}
	launcher.SetItemSessionLinker(linker, zerolog.Nop())
	origins := []models.ItemRef{
		{ProfileID: "p", SourceKind: "github", ExternalID: "acme/repo#1"},
		{ProfileID: "p", SourceKind: "github", ExternalID: "acme/repo#2"},
	}

	outcome, err := launcher.LaunchSession(t.Context(), LaunchSessionRequest{Name: "review-1", Prompt: "go", Repo: "r", Origins: origins})
	require.NoError(t, err)
	assert.Equal(t, "session-1", outcome.ID)
	assert.Equal(t, origins, linker.attempts)
}

// The flow-fired path: a command the engine enqueued carries the item it was
// routed from, and the executor hands it to the launcher.
func TestLaunchSessionExecutor_CarriesTheCommandsOriginToTheLauncher(t *testing.T) {
	launcher := &fakeSessionLauncher{}
	exec := NewLaunchSessionExecutor(zerolog.Nop(), launcher, nil, hostEnvironment{})
	action := actions.Action{ID: "spawn-review", Type: "launch-session", Config: &actions.LaunchSessionConfig{
		PromptTemplate: "review", RepoTemplate: "acme/site",
	}}
	ref := models.ItemRef{ProfileID: "p", SourceKind: "github", SourceScope: "acct", ExternalID: "acme/site#81"}

	_, err := exec.Execute(t.Context(), action, OutputData{
		Key: "oc-1", Raw: json.RawMessage(`{}`), Payload: map[string]any{}, Origin: ref,
	}, ActionInvocationInput{})
	require.NoError(t, err)
	require.Len(t, launcher.calls, 1)
	assert.Equal(t, []models.ItemRef{ref}, launcher.calls[0].Origins)
}

func launchWithPostHook(hook string, timeout actions.Duration) actions.Action {
	return actions.Action{
		ID:   "spawn-review",
		Type: "launch-session",
		Config: &actions.LaunchSessionConfig{
			PromptTemplate:  "Review {{ .Payload.title }}",
			RepoTemplate:    "{{ .Payload.repo }}",
			PostHook:        hook,
			PostHookTimeout: timeout,
		},
	}
}

func reviewItem() OutputData {
	return OutputData{
		Key:     "pr-42",
		Payload: map[string]any{"title": "Fix it", "repo": "example/repo", "num": 42},
		Raw:     json.RawMessage(`{"title":"Fix it","repo":"example/repo","num":42}`),
	}
}

func TestLaunchSessionExecutor_PostHookRunsInTheNewCheckout(t *testing.T) {
	dir := t.TempDir()
	launcher := &fakeSessionLauncher{path: dir}
	exec := NewLaunchSessionExecutor(zerolog.Nop(), launcher, nil, hostEnvironment{})

	action := launchWithPostHook("pwd && echo {{ .Payload.num }} {{ .Session.Slug }} > checked-out", 0)

	result, err := exec.Execute(t.Context(), action, reviewItem(), ActionInvocationInput{})
	require.NoError(t, err)
	require.NotNil(t, result.Outcome.Session)
	assert.Equal(t, dir, result.Outcome.Session.Path)

	written, readErr := os.ReadFile(filepath.Join(dir, "checked-out"))
	require.NoError(t, readErr)
	assert.Equal(t, "42 slug-1", strings.TrimSpace(string(written)))

	cwd, readErr := filepath.EvalSymlinks(strings.TrimSpace(result.Log.Stdout))
	require.NoError(t, readErr)
	resolved, readErr := filepath.EvalSymlinks(dir)
	require.NoError(t, readErr)
	assert.Equal(t, resolved, cwd)
}

func TestLaunchSessionExecutor_FailingPostHookKeepsTheLaunchSuccessful(t *testing.T) {
	launcher := &fakeSessionLauncher{path: t.TempDir()}
	exec := NewLaunchSessionExecutor(zerolog.Nop(), launcher, nil, hostEnvironment{})

	action := launchWithPostHook("echo nope >&2; exit 3", 0)

	result, err := exec.Execute(t.Context(), action, reviewItem(), ActionInvocationInput{})
	require.NoError(t, err)
	require.NotNil(t, result.Outcome.Session)
	assert.Equal(t, "session-1", result.Outcome.Session.ID)
	assert.Contains(t, result.Log.Stderr, "post_hook: ")
	assert.Contains(t, result.Log.Stderr, "exit status 3")
	assert.Contains(t, result.Log.Stderr, "nope")
}

func TestLaunchSessionExecutor_PostHookTimeoutDoesNotFailTheLaunch(t *testing.T) {
	launcher := &fakeSessionLauncher{path: t.TempDir()}
	exec := NewLaunchSessionExecutor(zerolog.Nop(), launcher, nil, hostEnvironment{})

	action := launchWithPostHook("sleep 30", actions.Duration(50*time.Millisecond))

	result, err := exec.Execute(t.Context(), action, reviewItem(), ActionInvocationInput{})
	require.NoError(t, err)
	require.NotNil(t, result.Outcome.Session)
	assert.Contains(t, result.Log.Stderr, "post_hook: ")
}

func TestLaunchSessionExecutor_PostHookTemplateErrorIsReportedNotReturned(t *testing.T) {
	launcher := &fakeSessionLauncher{path: t.TempDir()}
	exec := NewLaunchSessionExecutor(zerolog.Nop(), launcher, nil, hostEnvironment{})

	action := launchWithPostHook("echo {{ .Payload.missing }}", 0)

	result, err := exec.Execute(t.Context(), action, reviewItem(), ActionInvocationInput{})
	require.NoError(t, err)
	require.NotNil(t, result.Outcome.Session)
	assert.Contains(t, result.Log.Stderr, "post_hook: ")
}

func TestLaunchSessionExecutor_NoPostHookRunsNothing(t *testing.T) {
	launcher := &fakeSessionLauncher{path: t.TempDir()}
	exec := NewLaunchSessionExecutor(zerolog.Nop(), launcher, nil, hostEnvironment{})

	result, err := exec.Execute(t.Context(), launchWithPostHook("", 0), reviewItem(), ActionInvocationInput{})
	require.NoError(t, err)
	assert.Equal(t, ExecutionLog{}, result.Log)
}

func TestLaunchSessionExecutor_RendersTheNameTemplate(t *testing.T) {
	launcher := &fakeSessionLauncher{}
	exec := NewLaunchSessionExecutor(zerolog.Nop(), launcher, nil, hostEnvironment{})
	action := actions.Action{ID: "launch:triage/review", Type: "launch-session", Config: &LaunchNodeActionConfig{
		PromptTemplate: "hi", RepoTemplate: "{{ .Payload.repo }}",
		NameTemplate: "Review #{{ .Payload.num }}",
	}}

	_, err := exec.Execute(t.Context(), action, reviewItem(), ActionInvocationInput{})
	require.NoError(t, err)
	require.Len(t, launcher.calls, 1)
	assert.Equal(t, "review-42", launcher.calls[0].Name)
}

func TestLaunchSessionExecutor_BlankNameTemplateIsError(t *testing.T) {
	launcher := &fakeSessionLauncher{}
	exec := NewLaunchSessionExecutor(zerolog.Nop(), launcher, nil, hostEnvironment{})
	action := actions.Action{ID: "launch:triage/review", Type: "launch-session", Config: &LaunchNodeActionConfig{
		PromptTemplate: "hi", RepoTemplate: "{{ .Payload.repo }}",
		NameTemplate: "{{ .Payload.missing }}",
	}}

	_, err := exec.Execute(t.Context(), action, reviewItem(), ActionInvocationInput{})
	require.Error(t, err)
	assert.Empty(t, launcher.calls)
}

func TestLaunchSessionExecutor_CarriesTheCommandsOriginToTheWorkspaceLauncher(t *testing.T) {
	workspaces := &fakeWorkspaceSessionLauncher{}
	exec := NewLaunchSessionExecutor(zerolog.Nop(), nil, workspaces, hostEnvironment{})
	action := actions.Action{ID: "triage", Type: "launch-session", Config: &actions.LaunchSessionConfig{
		PromptTemplate: "triage", Workspace: "alerts",
	}}
	ref := models.ItemRef{ProfileID: "p", SourceKind: "grafana", ExternalID: "alert-1"}

	_, err := exec.Execute(t.Context(), action, OutputData{
		Key: "alert-1", Raw: json.RawMessage(`{}`), Payload: map[string]any{}, Origin: ref,
	}, ActionInvocationInput{})
	require.NoError(t, err)
	require.Len(t, workspaces.calls, 1)
	assert.Equal(t, []models.ItemRef{ref}, workspaces.calls[0].Origins)
}

func TestLaunchSessionExecutor_RunsALaunchNodeProjection(t *testing.T) {
	launcher := &fakeSessionLauncher{}
	exec := NewLaunchSessionExecutor(zerolog.Nop(), launcher, nil, hostEnvironment{})
	action, ok := NewFlowActions(launchFlows(), nil).Get(models.LaunchActionID("triage/review"))
	require.True(t, ok)

	_, err := exec.Execute(t.Context(), action, reviewItem(), ActionInvocationInput{})
	require.NoError(t, err)
	require.Equal(t, []LaunchSessionRequest{{Name: "review-42", Prompt: "Review 42", Agent: "claude", Repo: "example/repo"}}, launcher.calls)
}
