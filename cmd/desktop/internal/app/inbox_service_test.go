package app

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
)

func newTestWorker(db *queries.DB, actionStore dispatch.ActionLister, d *dispatch.Dispatcher, interval time.Duration, logger zerolog.Logger) *dispatch.Worker {
	return dispatch.NewWorker(stores.New(db, stores.Options{}).OutputCommands, actionStore, d, interval, logger)
}

func waitForTestWorker(t *testing.T, worker *dispatch.Worker) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	require.NoError(t, worker.WaitIdle(ctx))
}

func newTestInboxService(db *queries.DB, actionStore *actions.ActionStore, worker *dispatch.Worker) *InboxService {
	st := stores.New(db, stores.Options{})
	return newInboxService(InboxDeps{Items: st.InboxItems, Commands: st.OutputCommands, NodeRuns: st.NodeRuns, Catalog: actionStore, Worker: worker})
}

// recordingActionExecutor captures the context an action was dispatched with —
// what its templates would have rendered over — and returns whatever outcome
// the test staged.
type recordingActionExecutor struct {
	calls  int
	data   dispatch.OutputData
	result dispatch.ExecutionResult
	err    error
}

func (e *recordingActionExecutor) Execute(_ context.Context, _ actions.Action, data dispatch.OutputData, _ dispatch.ActionInvocationInput) (dispatch.ExecutionResult, error) {
	e.calls++
	e.data = data
	return e.result, e.err
}

type stubExecEnv struct{}

func (stubExecEnv) Environ(context.Context) []string { return os.Environ() }

type recordingSessionLauncher struct {
	calls []dispatch.LaunchSessionRequest
}

func (l *recordingSessionLauncher) LaunchSession(_ context.Context, req dispatch.LaunchSessionRequest) (dispatch.SessionExecutionOutcome, error) {
	l.calls = append(l.calls, req)
	return dispatch.SessionExecutionOutcome{ID: "session-1", Name: req.Name}, nil
}

func insertActionItem(t *testing.T, db *queries.DB, id, kind, title string) int64 {
	t.Helper()
	return insertActionItemSource(t, db, "github", id, kind, title, nil)
}

// insertActionItemSource inserts an inbox row with the given source kind and
// an arbitrary payload merged over the canonical id/kind/title fields — used
// by tests that need a non-GitHub source or extra payload fields (e.g. repo).
func insertActionItemSource(t *testing.T, db *queries.DB, sourceKind, id, kind, title string, extra map[string]any) int64 {
	t.Helper()
	fields := map[string]any{"id": id, "kind": kind, "title": title}
	maps.Copy(fields, extra)
	payload, err := json.Marshal(fields)
	require.NoError(t, err)
	row, err := stores.NewSeed(db).InboxItem(t.Context(), stores.InboxItem{ProfileID: "p", SourceKind: sourceKind, ExternalID: id, Title: title, Payload: payload, Lifecycle: "active"})
	require.NoError(t, err)
	return row.ID
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
  - id: deploy-repo
    label: Deploy
    type: launch-session
    show_in_detail: true
    repo_template: "{{ .Payload.repo }}"
    prompt_template: "Deploy {{ .Payload.title }}"
  - id: launch-interactive
    label: Launch
    type: launch-session
    show_in_detail: true
    prompt_template: "Launch {{ .Payload.title }}"
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
  - id: copy-checkout
    label: Copy checkout command
    type: clipboard
    show_in_detail: true
    applies_to: [pr]
    text_template: "gh pr checkout {{ .Payload.num }} -R {{ .Payload.repo }}"
`), 0o644))
	actionStore := actions.NewActionStore(path)
	require.NoError(t, actionStore.Reload())
	return actionStore
}

func TestPipelineService_ActionViewsAndInvocationUseActionStore(t *testing.T) {
	actionStore := configuredActionStore(t)
	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	executor := &recordingActionExecutor{}
	worker := newTestWorker(db, actionStore, dispatch.NewDispatcher(map[string]dispatch.Executor{
		"launch-session": executor,
	}), 0, zerolog.Nop())
	service := newTestInboxService(db, actionStore, worker)
	prID := insertActionItem(t, db, "pr-1", "PR", "Fix it")
	issueID := insertActionItem(t, db, "issue-1", "Issue", "")
	hiddenID := insertActionItem(t, db, "pr-2", "PR", "")

	// deploy-repo (repo_template) is absent: pr-1's payload has no "repo"
	// field, so the repo_template render errors and the action is
	// inapplicable. launch-interactive has no repo_template, so it always
	// applies.
	views, err := service.ActionViews(t.Context(), prID)
	require.NoError(t, err)
	// Views arrive in actions.yml order — the user-controlled presentation
	// order — not sorted by id.
	assert.Equal(t, []actions.View{
		{ID: "review-pr", Label: "Review PR", Type: "launch-session", ShowInDetail: true},
		{ID: "launch-interactive", Label: "Launch", Type: "launch-session", ShowInDetail: true, RequiresSessionInput: true},
		{ID: "triage-any", Label: "Triage", Type: "shell", ShowInDetail: true},
		{ID: "copy-checkout", Label: "Copy checkout command", Type: "clipboard", ShowInDetail: true},
	}, views)

	_, err = service.InvokeAction(t.Context(), InvokeActionRequest{ActionID: "review-pr", ItemID: prID, Input: dispatch.ActionInvocationInput{}})
	require.NoError(t, err)
	waitForTestWorker(t, worker)
	assert.Equal(t, 1, executor.calls)
	assert.Equal(t, "pr-1", executor.data.Key)
	assert.Equal(t, "Fix it", executor.data.Payload["title"])
	duplicate, err := service.InvokeAction(t.Context(), InvokeActionRequest{ActionID: "review-pr", ItemID: prID, Input: dispatch.ActionInvocationInput{}})
	require.NoError(t, err)
	assert.True(t, duplicate.ConfirmationRequired)
	assert.Equal(t, 1, executor.calls)
	rerun, err := service.InvokeAction(t.Context(), InvokeActionRequest{ActionID: "review-pr", ItemID: prID, Input: dispatch.ActionInvocationInput{Rerun: true}})
	require.NoError(t, err)
	assert.False(t, rerun.ConfirmationRequired)
	waitForTestWorker(t, worker)
	assert.Equal(t, 2, executor.calls)
	_, err = service.InvokeAction(t.Context(), InvokeActionRequest{ActionID: "review-pr", ItemID: issueID, Input: dispatch.ActionInvocationInput{}})
	require.Error(t, err)
	assert.Equal(t, KindInvalid, KindOf(err), "an inapplicable action is a bad request, not a fault")
	_, err = service.InvokeAction(t.Context(), InvokeActionRequest{ActionID: "hidden", ItemID: hiddenID, Input: dispatch.ActionInvocationInput{}})
	require.ErrorContains(t, err, "not available in the detail pane", "backend must reject a trusted caller bypassing ActionViews")
	assert.Equal(t, KindInvalid, KindOf(err))

	_, err = service.InvokeAction(t.Context(), InvokeActionRequest{ActionID: "does-not-exist", ItemID: prID, Input: dispatch.ActionInvocationInput{}})
	require.Error(t, err)
	assert.Equal(t, KindNotFound, KindOf(err), "an unknown action id is not found, not invalid")
}

// TestPipelineService_RenderClipboardActionIsRenderOnlyAndRepeatable pins the
// clipboard action's non-durable path: RenderClipboardAction renders the text
// with no output_command row, so copying the same item again just renders
// again with no rerun prompt, and InvokeAction refuses a clipboard action so it
// can never enqueue a durable command.
func TestPipelineService_RenderClipboardActionIsRenderOnlyAndRepeatable(t *testing.T) {
	actionStore := configuredActionStore(t)
	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	// A dispatcher with no clipboard executor: the render path must never route
	// a clipboard action through the worker.
	worker := newTestWorker(db, actionStore, dispatch.NewDispatcher(map[string]dispatch.Executor{}), 0, zerolog.Nop())
	service := newTestInboxService(db, actionStore, worker)
	prID := insertActionItemSource(t, db, "github", "pr-9", "PR", "Title", map[string]any{"num": 9, "repo": "acme/app"})

	text, err := service.RenderClipboardAction(t.Context(), "copy-checkout", []int64{prID}, nil)
	require.NoError(t, err)
	assert.Equal(t, "gh pr checkout 9 -R acme/app", text)

	// Repeatable: no durable command to confirm, so a second copy renders the
	// same text with no error or rerun prompt.
	again, err := service.RenderClipboardAction(t.Context(), "copy-checkout", []int64{prID}, nil)
	require.NoError(t, err)
	assert.Equal(t, text, again)

	// InvokeAction refuses a clipboard action rather than enqueueing it.
	_, err = service.InvokeAction(t.Context(), InvokeActionRequest{ActionID: "copy-checkout", ItemID: prID})
	require.Error(t, err)
	assert.Equal(t, KindInvalid, KindOf(err))
	require.ErrorContains(t, err, "clipboard action")

	// The render path refuses a non-clipboard action.
	_, err = service.RenderClipboardAction(t.Context(), "review-pr", []int64{prID}, nil)
	require.Error(t, err)
	assert.Equal(t, KindInvalid, KindOf(err))
}

func TestPipelineService_RenderClipboardActionAppliesToWholeSelection(t *testing.T) {
	actionStore := configuredActionStore(t)
	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	worker := newTestWorker(db, actionStore, dispatch.NewDispatcher(map[string]dispatch.Executor{}), 0, zerolog.Nop())
	service := newTestInboxService(db, actionStore, worker)
	firstID := insertActionItemSource(t, db, "github", "pr-1", "PR", "First", map[string]any{"num": 1, "repo": "acme/app"})
	secondID := insertActionItemSource(t, db, "github", "pr-2", "PR", "Second", map[string]any{"num": 2, "repo": "acme/api"})
	issueID := insertActionItemSource(t, db, "github", "issue-3", "Issue", "Third", map[string]any{"num": 3, "repo": "acme/app"})

	text, err := service.RenderClipboardAction(t.Context(), "copy-checkout", []int64{secondID, firstID}, nil)
	require.NoError(t, err)
	assert.Equal(t, "gh pr checkout 2 -R acme/api\n\ngh pr checkout 1 -R acme/app", text)

	_, err = service.RenderClipboardAction(t.Context(), "copy-checkout", []int64{firstID, issueID}, nil)
	require.Error(t, err)
	assert.Equal(t, KindInvalid, KindOf(err))
	require.ErrorContains(t, err, "does not apply to item")
}

// TestPipelineService_ActionViewsAndInvokeAreCapabilityGatedNotSourceGated
// covers the Phase 2 matrix: capability gating replaces the old
// SourceKind == "github" gate. (a) a webhook-sourced item with kind/repo in
// its payload is offered and can run a matching action. (b) a repo_template
// action is absent from ActionViews when the payload lacks repo, and
// InvokeAction rejects it with the render reason in the error. (c) an
// interactive launch-session action (no repo_template) is offered for a
// webhook item regardless of payload shape. (d) an unknown itemID errors
// (no panic) from both ActionViews and InvokeAction.
func TestPipelineService_ActionViewsAndInvokeAreCapabilityGatedNotSourceGated(t *testing.T) {
	actionStore := configuredActionStore(t)
	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	executor := &recordingActionExecutor{}
	worker := newTestWorker(db, actionStore, dispatch.NewDispatcher(map[string]dispatch.Executor{
		"launch-session": executor,
	}), 0, zerolog.Nop())
	service := newTestInboxService(db, actionStore, worker)

	t.Run("webhook item with a matching repo_template action gets and runs it", func(t *testing.T) {
		itemID := insertActionItemSource(t, db, "webhook", "hook-1", "deploy", "Deploy prod", map[string]any{"repo": "acme/site"})

		views, err := service.ActionViews(t.Context(), itemID)
		require.NoError(t, err)
		ids := make([]string, len(views))
		for i, v := range views {
			ids[i] = v.ID
		}
		assert.Contains(t, ids, "deploy-repo")

		_, err = service.InvokeAction(t.Context(), InvokeActionRequest{ActionID: "deploy-repo", ItemID: itemID, Input: dispatch.ActionInvocationInput{}})
		require.NoError(t, err)
		waitForTestWorker(t, worker)
		assert.Equal(t, "hook-1", executor.data.Key)
	})

	t.Run("repo_template action is absent and rejected with the render reason when the payload lacks repo", func(t *testing.T) {
		itemID := insertActionItemSource(t, db, "webhook", "hook-2", "deploy", "No repo", nil)

		views, err := service.ActionViews(t.Context(), itemID)
		require.NoError(t, err)
		for _, v := range views {
			assert.NotEqual(t, "deploy-repo", v.ID, "repo_template action must be absent when the payload lacks repo")
		}

		_, err = service.InvokeAction(t.Context(), InvokeActionRequest{ActionID: "deploy-repo", ItemID: itemID, Input: dispatch.ActionInvocationInput{}})
		require.Error(t, err)
		assert.ErrorContains(t, err, "repo_template")
	})

	t.Run("interactive launch-session action is offered for a webhook item", func(t *testing.T) {
		itemID := insertActionItemSource(t, db, "webhook", "hook-3", "deploy", "Investigate", nil)

		views, err := service.ActionViews(t.Context(), itemID)
		require.NoError(t, err)
		var found *actions.View
		for i := range views {
			if views[i].ID == "launch-interactive" {
				found = &views[i]
			}
		}
		require.NotNil(t, found, "interactive launch-session actions demand nothing from the payload")
		assert.True(t, found.RequiresSessionInput)
	})

	t.Run("unknown itemID errors from both ActionViews and InvokeAction", func(t *testing.T) {
		_, err := service.ActionViews(t.Context(), 999999)
		require.Error(t, err)
		assert.Equal(t, KindNotFound, KindOf(err))
		_, err = service.InvokeAction(t.Context(), InvokeActionRequest{ActionID: "review-pr", ItemID: 999999, Input: dispatch.ActionInvocationInput{}})
		require.Error(t, err)
		assert.Equal(t, KindNotFound, KindOf(err))
	})
}

func TestPipelineService_AttemptedFailureReturnsPersistedActionRun(t *testing.T) {
	actionStore := configuredActionStore(t)
	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	// Use a dispatcher executor that records a dispatched side effect failure.
	failed := &attemptedFailureExecutor{}
	worker := newTestWorker(db, actionStore, dispatch.NewDispatcher(map[string]dispatch.Executor{"launch-session": failed}), 0, zerolog.Nop())
	service := newTestInboxService(db, actionStore, worker)
	prID := insertActionItem(t, db, "pr-1", "PR", "Fix it")

	view, err := service.InvokeAction(t.Context(), InvokeActionRequest{ActionID: "review-pr", ItemID: prID, Input: dispatch.ActionInvocationInput{}})
	require.NoError(t, err)
	assert.Equal(t, "running", view.Status)
	waitForTestWorker(t, worker)
	view, err = service.ActionRun(t.Context(), view.CommandID)
	require.NoError(t, err)
	assert.Equal(t, "failed", view.Status)
	assert.Equal(t, "side effect failed", view.Error)
	assert.Equal(t, "partial stdout", view.Stdout)
	assert.Equal(t, "partial stderr", view.Stderr)

	afterNavigation, err := service.ActionRun(t.Context(), view.CommandID)
	require.NoError(t, err)
	assert.Equal(t, view, afterNavigation)
}

type attemptedFailureExecutor struct{}

func (attemptedFailureExecutor) Execute(context.Context, actions.Action, dispatch.OutputData, dispatch.ActionInvocationInput) (dispatch.ExecutionResult, error) {
	return dispatch.ExecutionResult{Attempted: true, Log: dispatch.ExecutionLog{Stdout: "partial stdout", Stderr: "partial stderr"}}, fmt.Errorf("side effect failed")
}

func TestPipelineService_ActionRunSurvivesDatabaseReopen(t *testing.T) {
	actionStore := configuredActionStore(t)
	dir := t.TempDir()
	db, err := queries.Open(t.Context(), dir, queries.DefaultOpenOptions())
	require.NoError(t, err)
	failed := &attemptedFailureExecutor{}
	worker := newTestWorker(db, actionStore, dispatch.NewDispatcher(map[string]dispatch.Executor{"launch-session": failed}), 0, zerolog.Nop())
	prID := insertActionItem(t, db, "pr-1", "PR", "Fix it")
	view, err := newTestInboxService(db, actionStore, worker).InvokeAction(t.Context(), InvokeActionRequest{ActionID: "review-pr", ItemID: prID})
	require.NoError(t, err)
	waitForTestWorker(t, worker)
	view, err = newTestInboxService(db, actionStore, worker).ActionRun(t.Context(), view.CommandID)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	reopened, err := queries.Open(t.Context(), dir, queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })
	afterRestart, err := newTestInboxService(reopened, actionStore, nil).ActionRun(t.Context(), view.CommandID)
	require.NoError(t, err)
	assert.Equal(t, view, afterRestart)
}

func TestPipelineService_ActionRunLocationResolvesTheOriginatingItem(t *testing.T) {
	actionStore := configuredActionStore(t)
	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	worker := newTestWorker(db, actionStore, dispatch.NewDispatcher(map[string]dispatch.Executor{
		"shell": &recordingActionExecutor{},
	}), 0, zerolog.Nop())
	service := newTestInboxService(db, actionStore, worker)
	itemID := insertActionItem(t, db, "item-key-is-not-a-row-id", "Item", "Try it")
	st := stores.New(db, stores.Options{})
	require.NoError(t, st.FeedClaims.Upsert(t.Context(), models.FeedClaim{
		ProfileID: "p", FeedID: "p/demo", ItemID: itemID, SourceID: "source:p/demo",
	}))

	view, err := service.InvokeAction(t.Context(), InvokeActionRequest{ActionID: "triage-any", ItemID: itemID})
	require.NoError(t, err)
	location, err := service.ActionRunLocation(t.Context(), view.CommandID)
	require.NoError(t, err)
	assert.Equal(t, ActionRunLocation{ItemID: itemID, ProfileID: "p", FeedID: "p/demo"}, location)
	waitForTestWorker(t, worker)
}

func TestPipelineService_ConfirmedLaunchSessionExecutesRealActionPath(t *testing.T) {
	actionStore := configuredActionStore(t)
	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	launcher := &recordingSessionLauncher{}
	worker := newTestWorker(db, actionStore, dispatch.NewDispatcher(map[string]dispatch.Executor{
		"launch-session": dispatch.NewLaunchSessionExecutor(zerolog.Nop(), launcher, nil, stubExecEnv{}),
	}), 0, zerolog.Nop())
	service := newTestInboxService(db, actionStore, worker)
	prID := insertActionItem(t, db, "pr-1", "PR", "Fix it")

	_, err = service.InvokeAction(t.Context(), InvokeActionRequest{ActionID: "review-pr", ItemID: prID, Input: dispatch.ActionInvocationInput{}})
	require.NoError(t, err)
	waitForTestWorker(t, worker)
	require.Equal(t, []dispatch.LaunchSessionRequest{{
		Name: "review-pr-pr-1", Prompt: "Review Fix it", Repo: "git@example/repo.git", CollisionSuffix: "1",
		Origins: []models.ItemRef{{ProfileID: "p", SourceKind: "github", ExternalID: "pr-1"}},
	}}, launcher.calls)

	var status string
	require.NoError(t, db.Conn().QueryRowContext(t.Context(),
		`SELECT status FROM output_command WHERE action_id = ? AND key = ?`, "review-pr", "pr-1").Scan(&status))
	assert.Equal(t, "done", status)
}

func TestInboxService_NewSessionDraft(t *testing.T) {
	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	service := newTestInboxService(db, configuredActionStore(t), nil)

	itemID := insertActionItemSource(t, db, "github", "pr-1", "PR", "Fix the crash",
		map[string]any{"repo": "acme/site", "body": "Steps to repro", "url": "https://github.com/acme/site/issues/1"})

	draft, err := service.NewSessionDraft(t.Context(), []int64{itemID})
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/acme/site.git", draft.Repository, "owner/name must become a cloneable remote, not a bare path")
	assert.Equal(t, "fix-the-crash", draft.Name)
	assert.Contains(t, draft.Prompt, "Fix the crash")
	assert.Contains(t, draft.Prompt, "Steps to repro")
	assert.Contains(t, draft.Prompt, "https://github.com/acme/site/issues/1")

	secondID := insertActionItemSource(t, db, "github", "pr-2", "PR", "Fix the timeout",
		map[string]any{"repo": "acme/site", "body": "Timeout details", "url": "https://github.com/acme/site/issues/2"})
	combined, err := service.NewSessionDraft(t.Context(), []int64{secondID, itemID, secondID})
	require.NoError(t, err)
	assert.Equal(t, []int64{secondID, itemID}, combined.ItemIDs)
	assert.Equal(t, "https://github.com/acme/site.git", combined.Repository)
	assert.Regexp(t, `^inbox-selection-[0-9a-f]{8}$`, combined.Name)
	assert.Less(t, strings.Index(combined.Prompt, "Timeout details"), strings.Index(combined.Prompt, "Steps to repro"))

	otherRepoID := insertActionItemSource(t, db, "github", "pr-3", "PR", "Other repository",
		map[string]any{"repo": "acme/other", "url": "https://github.com/acme/other/issues/3"})
	mixed, err := service.NewSessionDraft(t.Context(), []int64{itemID, otherRepoID})
	require.NoError(t, err)
	assert.Empty(t, mixed.Repository)

	_, err = service.NewSessionDraft(t.Context(), nil)
	assert.Equal(t, KindInvalid, KindOf(err))
	_, err = service.NewSessionDraft(t.Context(), []int64{0})
	assert.Equal(t, KindInvalid, KindOf(err))
	_, err = service.NewSessionDraft(t.Context(), []int64{9999})
	assert.Equal(t, KindNotFound, KindOf(err))
}

// An action's declared inputs are part of the safe view (the detail pane needs
// them to build the form) and are validated against the catalog on the way in,
// so an incomplete or stale form is refused before it becomes a durable
// command.
func TestInboxService_DeclaredInputsAreViewedAndPreflighted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "actions.yml")
	require.NoError(t, os.WriteFile(path, []byte(`version: 1
actions:
  - id: ignore-alert
    label: Ignore alert
    type: shell
    show_in_detail: true
    inputs:
      - name: reason
        label: Reason
        type: multiline
        required: true
      - name: window
        type: select
        default: 1h
        options: [1h, 24h]
    command_template: "true"
`), 0o644))
	actionStore := actions.NewActionStore(path)
	require.NoError(t, actionStore.Reload())

	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	executor := &recordingActionExecutor{}
	worker := newTestWorker(db, actionStore, dispatch.NewDispatcher(map[string]dispatch.Executor{"shell": executor}), 0, zerolog.Nop())
	service := newTestInboxService(db, actionStore, worker)
	itemID := insertActionItem(t, db, "pr-1", "PR", "Fix it")

	views, err := service.ActionViews(t.Context(), itemID)
	require.NoError(t, err)
	require.Len(t, views, 1)
	require.Len(t, views[0].Inputs, 2)
	assert.Equal(t, "reason", views[0].Inputs[0].Name)
	assert.Equal(t, actions.InputTypeMultiline, views[0].Inputs[0].Type)
	assert.Equal(t, []string{"1h", "24h"}, views[0].Inputs[1].Options)

	_, err = service.InvokeAction(t.Context(), InvokeActionRequest{ActionID: "ignore-alert", ItemID: itemID})
	require.ErrorContains(t, err, "is required")
	assert.Equal(t, KindInvalid, KindOf(err))
	assert.Equal(t, 0, executor.calls, "a refused form never reaches the worker")

	_, err = service.InvokeAction(t.Context(), InvokeActionRequest{
		ActionID: "ignore-alert",
		ItemID:   itemID,
		Input:    dispatch.ActionInvocationInput{Inputs: map[string]string{"reason": "flapping"}},
	})
	require.NoError(t, err)
	waitForTestWorker(t, worker)
	require.Equal(t, 1, executor.calls)
	assert.Equal(t, map[string]string{"reason": "flapping", "window": "1h"}, executor.data.Inputs)
}

// A stale revision must map to KindConflict, not KindNotFound. The generic
// not-found transform would still match sql.ErrNoRows and misreport deletion.
func TestInboxService_ToggleArchivedStaleRevisionIsConflict(t *testing.T) {
	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	service := newTestInboxService(db, configuredActionStore(t), nil)

	itemID := insertActionItem(t, db, "pr-1", "PR", "Fix it")

	_, err = service.ToggleArchived(t.Context(), itemID, 1)
	require.NoError(t, err)

	_, err = service.ToggleArchived(t.Context(), itemID, 1)
	require.Error(t, err)
	assert.Equal(t, KindConflict, KindOf(err))
	assert.False(t, stores.IsNotFound(err))
}

func TestInboxService_MissingRowsMapOntoKinds(t *testing.T) {
	actionStore := configuredActionStore(t)
	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	worker := newTestWorker(db, actionStore, dispatch.NewDispatcher(map[string]dispatch.Executor{
		"launch-session": &recordingActionExecutor{},
	}), 0, zerolog.Nop())
	service := newTestInboxService(db, actionStore, worker)
	prID := insertActionItem(t, db, "pr-1", "PR", "Fix it")

	_, err = service.Events(t.Context(), 999999, 10)
	assert.Equal(t, KindNotFound, KindOf(err))
	_, err = service.ActionRun(t.Context(), 999999)
	assert.Equal(t, KindNotFound, KindOf(err))
	_, err = service.InvokeAction(t.Context(), InvokeActionRequest{ActionID: "review-pr", ItemID: prID, Input: dispatch.ActionInvocationInput{Rerun: true}})
	assert.Equal(t, KindInvalid, KindOf(err), "a rerun needs a completed run to repeat")
}
