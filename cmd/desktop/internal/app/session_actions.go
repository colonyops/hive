package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/cmd/desktop/internal/app/activity"
	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
	"github.com/colonyops/hive/internal/domain/session"
)

// terminalFailureStderrBytes bounds the diagnostic tail a failed terminal
// action reports. A terminal action enqueues no output_command, so there is no
// durable row holding its streams — without this, "exit status 1" is the whole
// story.
const terminalFailureStderrBytes = 400

// TerminalActionViews returns the configured actions offered on one terminal
// surface: actions.TargetSession for a session row, actions.TargetWindow for a
// window row inside it.
//
// Unlike a feed item's, the set does not depend on which session: a terminal
// action declares the surface it belongs on and there is no item kind to
// narrow it further. Presentation order is the catalog's, same as everywhere
// else actions are offered.
func (s *SessionsService) TerminalActionViews(_ context.Context, target string) ([]actions.View, error) {
	if err := validateTerminalSurface(target); err != nil {
		return nil, err
	}
	views := make([]actions.View, 0)
	for _, action := range s.catalog.List() {
		if action.HasTarget(target) {
			views = append(views, action.View())
		}
	}
	return views, nil
}

// InvokeTerminalAction runs one configured action against a terminal target as
// a background job, and returns the job id — the run outlives the call the way
// every other session job does, and the jobs UI is where its outcome lands.
//
// It deliberately enqueues no output_command. A durable command's
// UNIQUE (action_id, key) is what stops an already-run action from re-firing,
// and that is exactly wrong here: "open the worktree in Finder" is a manual
// operation against live local state, meant to be repeatable without a rerun
// confirmation and meaningless to replay after a restart.
//
// The authorization chain: the action must exist, declare the surface it was
// invoked from, and satisfy its declared inputs; the session must exist and be
// active. Everything the templates read is resolved here from the session
// record rather than taken from the caller.
func (s *SessionsService) InvokeTerminalAction(ctx context.Context, actionID string, target dispatch.TerminalTarget, inputs map[string]string) (int64, error) {
	action, data, err := s.terminalActionContext(ctx, actionID, target, inputs)
	if err != nil {
		return 0, err
	}
	if _, isClipboard := action.Config.(*actions.ClipboardConfig); isClipboard {
		// Same split the detail pane makes: a clipboard action produces text,
		// not a side effect, so it is copied rather than run.
		return 0, Errorf(KindInvalid, "action %q is a clipboard action; copy it instead", actionID)
	}
	label := terminalActionLabel(action)
	jobID := s.jobs.Track(ctx, label, action.ID, data.Session.Name, func(bg context.Context) error {
		result, err := s.dispatcher.Execute(bg, action, data, dispatch.ActionInvocationInput{Inputs: inputs})
		if err != nil {
			reason := terminalActionFailure(err, result)
			s.record(bg, activity.ActionFailed(label, reason))
			return errors.New(reason)
		}
		s.record(bg, activity.ActionRun(label, data.Session.Name))
		return nil
	})
	return jobID, nil
}

// RenderTerminalClipboardAction resolves a clipboard action against a terminal
// target and returns the text for the desktop adapter to place on the
// clipboard. It is the render-only sibling of InvokeTerminalAction, mirroring
// the split the detail pane already makes: re-copying just renders again.
func (s *SessionsService) RenderTerminalClipboardAction(ctx context.Context, actionID string, target dispatch.TerminalTarget, inputs map[string]string) (string, error) {
	action, data, err := s.terminalActionContext(ctx, actionID, target, inputs)
	if err != nil {
		return "", err
	}
	if _, isClipboard := action.Config.(*actions.ClipboardConfig); !isClipboard {
		return "", Errorf(KindInvalid, "action %q is not a clipboard action", actionID)
	}
	result, err := s.dispatcher.Execute(ctx, action, data, dispatch.ActionInvocationInput{Inputs: inputs})
	if err != nil {
		return "", Wrap(err, KindInvalid, "rendering clipboard action %q", actionID)
	}
	if result.Outcome == nil || result.Outcome.Clipboard == nil {
		return "", Errorf(KindInternal, "action %q produced no clipboard text", actionID)
	}
	return result.Outcome.Clipboard.Text, nil
}

// terminalActionContext runs the whole authorization chain and assembles the
// template context both invocation paths render over, so what the clipboard
// copies and what a run executes can never see a different session.
func (s *SessionsService) terminalActionContext(
	ctx context.Context,
	actionID string,
	target dispatch.TerminalTarget,
	inputs map[string]string,
) (actions.Action, dispatch.OutputData, error) {
	slug := strings.TrimSpace(target.Slug)
	if slug == "" {
		return actions.Action{}, dispatch.OutputData{}, Errorf(KindInvalid, "session slug is required")
	}
	action, ok := s.catalog.Get(actionID)
	if !ok {
		return actions.Action{}, dispatch.OutputData{}, Errorf(KindNotFound, "unknown action %q", actionID)
	}
	// The window id is what makes an invocation a window invocation, so a
	// session-only action reached from a window row is refused rather than
	// silently run against the session.
	surface := actions.TargetSession
	windowID := strings.TrimSpace(target.WindowID)
	if windowID != "" {
		surface = actions.TargetWindow
	}
	if !action.HasTarget(surface) {
		return actions.Action{}, dispatch.OutputData{}, Errorf(KindInvalid, "action %q is not offered on a terminal %s", actionID, surface)
	}

	detail, err := s.detailBySlug(ctx, slug)
	if err != nil {
		return actions.Action{}, dispatch.OutputData{}, err
	}
	if detail.State != string(session.StateActive) {
		return actions.Action{}, dispatch.OutputData{}, Errorf(KindConflict, "session %q is %s, so there is no checkout left to run an action in", detail.Name, detail.State)
	}
	resolved, err := action.ResolveInputs(inputs)
	if err != nil {
		return actions.Action{}, dispatch.OutputData{}, Wrap(err, KindInvalid, "invoking action %q", actionID)
	}

	data := dispatch.OutputData{
		Key:       detail.Slug,
		Inputs:    resolved,
		CreatedAt: time.Now().UnixMilli(),
		Session: &dispatch.SessionTarget{
			ID:     detail.ID,
			Name:   detail.Name,
			Slug:   detail.Slug,
			Repo:   detail.Repo,
			Path:   detail.Path,
			Branch: detail.WorktreeBranch,
		},
	}
	if windowID != "" {
		data.Window = &dispatch.WindowTarget{ID: windowID}
	}
	return action, data, nil
}

func (s *SessionsService) record(ctx context.Context, e activity.Event) {
	if s.recorder != nil {
		s.recorder.Record(ctx, e)
	}
}

func validateTerminalSurface(target string) error {
	if target != actions.TargetSession && target != actions.TargetWindow {
		return Errorf(KindInvalid, "unknown terminal target %q (expected %s or %s)", target, actions.TargetSession, actions.TargetWindow)
	}
	return nil
}

func terminalActionLabel(action actions.Action) string {
	if action.Label != "" {
		return action.Label
	}
	return action.ID
}

// terminalActionFailure is the reason a failed terminal action reports to the
// jobs UI and the activity log. Without the stderr tail a failed shell action
// reports its exit status and nothing else, which is not enough to act on.
func terminalActionFailure(err error, result dispatch.ExecutionResult) string {
	stderr := strings.TrimSpace(result.Log.Stderr)
	if stderr == "" {
		return err.Error()
	}
	if len(stderr) > terminalFailureStderrBytes {
		stderr = "…" + stderr[len(stderr)-terminalFailureStderrBytes:]
	}
	return err.Error() + ": " + stderr
}
