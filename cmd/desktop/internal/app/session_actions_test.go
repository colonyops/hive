package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
	"github.com/colonyops/hive/internal/domain/session"
)

func terminalCatalog(t *testing.T, yaml string) *actions.ActionStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "actions.yml")
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0o600))
	store := actions.NewActionStore(path)
	require.NoError(t, store.Reload())
	return store
}

const terminalActionsYAML = `version: 1
actions:
  - id: open-in-zed
    label: Open in Zed
    type: shell
    targets: [session]
    command_template: 'zed {{ .Session.Path }}'
  - id: interrupt
    label: Interrupt
    type: shell
    targets: [window]
    command_template: 'tmux send-keys -t {{ .Session.Slug }}:{{ .Window.ID }} C-c'
  - id: copy-path
    label: Copy path
    type: clipboard
    targets: [session, window]
    text_template: '{{ .Session.Path }}'
  - id: review-pr
    label: Review PR
    type: shell
    show_in_detail: true
    command_template: 'echo {{ .Key }}'
`

func terminalSessionsService(t *testing.T, executor dispatch.Executor) (*SessionsService, *fakeJobRunner, *hiveHarness) {
	t.Helper()
	h := newHiveHarness(t, engineOptions{})
	sess := reviewSession()
	sess.Path = "/work/review-81"
	sess.SetMeta(session.MetaWorktreeBranch, "feat/review")
	h.save(t, sess)
	runner := &fakeJobRunner{}
	dispatcher := dispatch.NewDispatcher(map[string]dispatch.Executor{
		"shell":     executor,
		"clipboard": dispatch.NewClipboardExecutor(),
	})
	svc := newSessionsService(SessionsDeps{Hive: h.engine, Tmux: &fakeSessionTmux{}, Jobs: runner, Catalog: terminalCatalog(t, terminalActionsYAML), Dispatcher: dispatcher})
	return svc, runner, h
}

func TestTerminalActionViews_ScopedToTheSurface(t *testing.T) {
	svc, _, _ := terminalSessionsService(t, &recordingActionExecutor{})

	sessionViews, err := svc.TerminalActionViews(t.Context(), actions.TargetSession)
	require.NoError(t, err)
	assert.Equal(t, []string{"open-in-zed", "copy-path"}, viewIDs(sessionViews))

	windowViews, err := svc.TerminalActionViews(t.Context(), actions.TargetWindow)
	require.NoError(t, err)
	assert.Equal(t, []string{"interrupt", "copy-path"}, viewIDs(windowViews))
}

func TestTerminalActionViews_UnknownSurfaceIsInvalid(t *testing.T) {
	svc, _, _ := terminalSessionsService(t, &recordingActionExecutor{})
	_, err := svc.TerminalActionViews(t.Context(), actions.TargetItem)
	require.Error(t, err)
	assert.Equal(t, KindInvalid, KindOf(err))
}

func TestInvokeTerminalAction_ResolvesTheSessionFromItsSlug(t *testing.T) {
	executor := &recordingActionExecutor{}
	svc, runner, _ := terminalSessionsService(t, executor)

	jobID, err := svc.InvokeTerminalAction(t.Context(), "open-in-zed", dispatch.TerminalTarget{Slug: "review-81"}, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(7), jobID)
	require.True(t, runner.ran)
	require.NoError(t, runner.err)
	assert.Equal(t, "Open in Zed", runner.label)
	assert.Equal(t, "open-in-zed", runner.actionID)

	require.NotNil(t, executor.data.Session)
	assert.Equal(t, "/work/review-81", executor.data.Session.Path)
	assert.Equal(t, "review-81", executor.data.Session.Slug)
	assert.Equal(t, "acme/site", executor.data.Session.Repo)
	assert.Equal(t, "feat/review", executor.data.Session.Branch)
	assert.Nil(t, executor.data.Window, "a session invocation carries no window")
}

func TestInvokeTerminalAction_WindowIDSelectsTheWindowSurface(t *testing.T) {
	executor := &recordingActionExecutor{}
	svc, _, _ := terminalSessionsService(t, executor)

	_, err := svc.InvokeTerminalAction(t.Context(), "interrupt", dispatch.TerminalTarget{Slug: "review-81", WindowID: "@3"}, nil)
	require.NoError(t, err)
	require.NotNil(t, executor.data.Window)
	assert.Equal(t, "@3", executor.data.Window.ID)
}

// A window id is what makes an invocation a window invocation, so an action
// that only declares the session surface must not run from a window row.
func TestInvokeTerminalAction_RefusesAnActionTheSurfaceDoesNotOffer(t *testing.T) {
	svc, runner, _ := terminalSessionsService(t, &recordingActionExecutor{})

	_, err := svc.InvokeTerminalAction(t.Context(), "open-in-zed", dispatch.TerminalTarget{Slug: "review-81", WindowID: "@3"}, nil)
	require.Error(t, err)
	assert.Equal(t, KindInvalid, KindOf(err))
	assert.False(t, runner.ran)
}

func TestInvokeTerminalAction_RefusesAnItemAction(t *testing.T) {
	svc, _, _ := terminalSessionsService(t, &recordingActionExecutor{})

	_, err := svc.InvokeTerminalAction(t.Context(), "review-pr", dispatch.TerminalTarget{Slug: "review-81"}, nil)
	require.Error(t, err)
	assert.Equal(t, KindInvalid, KindOf(err))
}

func TestInvokeTerminalAction_RefusesAClipboardAction(t *testing.T) {
	svc, _, _ := terminalSessionsService(t, &recordingActionExecutor{})

	_, err := svc.InvokeTerminalAction(t.Context(), "copy-path", dispatch.TerminalTarget{Slug: "review-81"}, nil)
	require.Error(t, err)
	assert.Equal(t, KindInvalid, KindOf(err))
}

func TestInvokeTerminalAction_RefusesANonActiveSession(t *testing.T) {
	svc, _, h := terminalSessionsService(t, &recordingActionExecutor{})
	sess := h.session(t, "s1")
	sess.State = session.StateRecycled
	h.save(t, sess)

	_, err := svc.InvokeTerminalAction(t.Context(), "open-in-zed", dispatch.TerminalTarget{Slug: "review-81"}, nil)
	require.Error(t, err)
	assert.Equal(t, KindConflict, KindOf(err))
}

func TestInvokeTerminalAction_UnknownSlugIsNotFound(t *testing.T) {
	svc, _, _ := terminalSessionsService(t, &recordingActionExecutor{})

	_, err := svc.InvokeTerminalAction(t.Context(), "open-in-zed", dispatch.TerminalTarget{Slug: "nope"}, nil)
	require.Error(t, err)
	assert.Equal(t, KindNotFound, KindOf(err))
}

// A terminal action has no durable command row holding its streams, so the
// stderr tail is the only diagnostic the jobs UI ever sees.
func TestInvokeTerminalAction_FailureCarriesTheStderrTail(t *testing.T) {
	executor := &recordingActionExecutor{
		err:    errors.New("shell: command failed: exit status 1"),
		result: dispatch.ExecutionResult{Attempted: true, Log: dispatch.ExecutionLog{Stderr: "zed: command not found\n"}},
	}
	svc, runner, _ := terminalSessionsService(t, executor)

	_, err := svc.InvokeTerminalAction(t.Context(), "open-in-zed", dispatch.TerminalTarget{Slug: "review-81"}, nil)
	require.NoError(t, err, "the job reports the failure, the call reports that it started")
	require.Error(t, runner.err)
	assert.Contains(t, runner.err.Error(), "exit status 1")
	assert.Contains(t, runner.err.Error(), "zed: command not found")
}

func TestRenderTerminalClipboardAction_RendersTheSessionContext(t *testing.T) {
	svc, runner, _ := terminalSessionsService(t, &recordingActionExecutor{})

	text, err := svc.RenderTerminalClipboardAction(t.Context(), "copy-path", dispatch.TerminalTarget{Slug: "review-81"}, nil)
	require.NoError(t, err)
	assert.Equal(t, "/work/review-81", text)
	assert.False(t, runner.ran, "copying renders text; it never enqueues a job")
}

func TestRenderTerminalClipboardAction_RefusesANonClipboardAction(t *testing.T) {
	svc, _, _ := terminalSessionsService(t, &recordingActionExecutor{})

	_, err := svc.RenderTerminalClipboardAction(t.Context(), "open-in-zed", dispatch.TerminalTarget{Slug: "review-81"}, nil)
	require.Error(t, err)
	assert.Equal(t, KindInvalid, KindOf(err))
}

func viewIDs(views []actions.View) []string {
	ids := make([]string, 0, len(views))
	for _, view := range views {
		ids = append(ids, view.ID)
	}
	return ids
}
