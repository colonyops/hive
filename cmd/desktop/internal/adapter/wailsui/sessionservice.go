package wailsui

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
)

// SessionService is the frontend API for the New Session form and for managing
// the sessions that exist.
type SessionService struct {
	sessions *app.SessionsService
}

func NewSessionService(sessions *app.SessionsService) *SessionService {
	return &SessionService{sessions: sessions}
}

func (s *SessionService) SessionLaunchOptions(ctx context.Context) (dispatch.SessionLaunchOptions, error) {
	return s.sessions.SessionLaunchOptions(ctx)
}

// SessionLaunchWorkspaces returns workspace choices without resolving
// repository options.
func (s *SessionService) SessionLaunchWorkspaces(ctx context.Context) []dispatch.SessionLaunchWorkspace {
	return s.sessions.SessionLaunchWorkspaces(ctx)
}

// ListSessions returns every session in every state; Slug is the tmux target an
// attach uses, and only an active session has one.
func (s *SessionService) ListSessions(ctx context.Context) ([]dispatch.SessionSummary, error) {
	return s.sessions.ListSessions(ctx)
}

// SessionStatusSnapshot is one poll result in the units the browser timer uses.
type SessionStatusSnapshot struct {
	Items          []dispatch.SessionStatus `json:"items"`
	PollIntervalMS int64                    `json:"pollIntervalMs"`
}

// SessionStatuses returns the current terminal-detected agent state for each
// active session.
func (s *SessionService) SessionStatuses(ctx context.Context) (SessionStatusSnapshot, error) {
	snapshot, err := s.sessions.SessionStatuses(ctx)
	if err != nil {
		return SessionStatusSnapshot{}, err
	}
	return sessionStatusSnapshotOf(snapshot), nil
}

func sessionStatusSnapshotOf(snapshot dispatch.SessionStatusSnapshot) SessionStatusSnapshot {
	return SessionStatusSnapshot{
		Items:          snapshot.Items,
		PollIntervalMS: snapshot.PollInterval.Milliseconds(),
	}
}

// SessionDetail reads one session in full, for the detail view.
func (s *SessionService) SessionDetail(ctx context.Context, id string) (dispatch.SessionDetail, error) {
	return s.sessions.SessionDetail(ctx, id)
}

// SessionGitStatus reads the session's checkout for the session status bar.
// Cheap and local — four git subprocesses — so the bar polls it, unlike
// SessionPullRequest.
func (s *SessionService) SessionGitStatus(ctx context.Context, id string) (dispatch.SessionGitStatus, error) {
	return s.sessions.SessionGitStatus(ctx, id)
}

// SessionPullRequest resolves the pull request for a branch SessionGitStatus
// reported, answering from a short-lived cache unless refresh is set. Its
// Status field says why there is nothing to show, so a caller never has to
// read an empty result as "none".
func (s *SessionService) SessionPullRequest(ctx context.Context, key dispatch.SessionPullRequestKey, refresh bool) (dispatch.SessionPullRequest, error) {
	return s.sessions.SessionPullRequest(ctx, key, refresh)
}

// OpenSessionInEditor launches the configured editor on the session's checkout.
func (s *SessionService) OpenSessionInEditor(ctx context.Context, id string) error {
	return s.sessions.OpenSessionInEditor(ctx, id)
}

// RevealSession opens the session's checkout in the OS file manager.
func (s *SessionService) RevealSession(ctx context.Context, id string) error {
	return s.sessions.RevealSession(ctx, id)
}

// ItemSessions returns the sessions an inbox item spawned, newest first, with
// the state hive reports for each now. Slug is the attach target.
func (s *SessionService) ItemSessions(ctx context.Context, itemID int64) ([]dispatch.ItemSessionView, error) {
	return s.sessions.ItemSessions(ctx, itemID)
}

// SessionRisk reports the uncommitted or unpushed work a delete or recycle
// would discard, for the confirmation that precedes one.
func (s *SessionService) SessionRisk(ctx context.Context, id string) (dispatch.SessionRisk, error) {
	return s.sessions.SessionRisk(ctx, id)
}

// CreateSession validates the form and starts its repository session or
// workspace chat as a background job. Its outcome surfaces in the jobs UI.
func (s *SessionService) CreateSession(ctx context.Context, req dispatch.CreateSessionRequest) (int64, error) {
	return s.sessions.CreateSession(ctx, req)
}

// FailedSessionDraft returns the last New Session form whose creation failed,
// with the failure on it. A draft whose Failure is null means none is waiting.
func (s *SessionService) FailedSessionDraft(ctx context.Context) (dispatch.SessionDraft, error) {
	return s.sessions.FailedSessionDraft(ctx)
}

// SessionDraftFromActivity decodes the New Session form a failed-create
// activity row carries. Pass the row's own metadata; the keys in it are the
// backend's.
func (s *SessionService) SessionDraftFromActivity(ctx context.Context, metadata map[string]string) (dispatch.SessionDraft, error) {
	return s.sessions.SessionDraftFromActivity(ctx, metadata)
}

// DismissFailedSession drops the pending failed attempt.
func (s *SessionService) DismissFailedSession(ctx context.Context) error {
	return s.sessions.DismissFailedSession(ctx)
}

// RenameSession renames a session and returns its new summary. The slug in it
// is the new tmux target: renaming re-slugs, so an attached caller has to
// re-attach under the name that comes back.
func (s *SessionService) RenameSession(ctx context.Context, id, name string) (dispatch.SessionSummary, error) {
	return s.sessions.RenameSession(ctx, id, name)
}

// DeleteSession starts the delete as a background job and returns the job id.
func (s *SessionService) DeleteSession(ctx context.Context, id string) (int64, error) {
	return s.sessions.DeleteSession(ctx, id)
}

// RecycleSession starts the recycle as a background job and returns the job id.
func (s *SessionService) RecycleSession(ctx context.Context, id string) (int64, error) {
	return s.sessions.RecycleSession(ctx, id)
}

// PruneSessions starts the prune of every recycled and corrupted session as a
// background job and returns the job id.
func (s *SessionService) PruneSessions(ctx context.Context) (int64, error) {
	return s.sessions.PruneSessions(ctx)
}

// TerminalActionViews returns the configured actions the terminal offers on
// one of its surfaces — "session" for a session row, "window" for a window
// row.
func (s *SessionService) TerminalActionViews(ctx context.Context, target string) ([]actions.View, error) {
	return s.sessions.TerminalActionViews(ctx, target)
}

// InvokeTerminalAction runs an action against a terminal target as a
// background job and returns the job id; its outcome surfaces in the jobs UI.
func (s *SessionService) InvokeTerminalAction(ctx context.Context, actionID string, target dispatch.TerminalTarget, inputs map[string]string) (int64, error) {
	return s.sessions.InvokeTerminalAction(ctx, actionID, target, inputs)
}

// RenderTerminalClipboardAction returns the text a clipboard action renders
// for a terminal target. The frontend writes it through the native Wails
// clipboard; the core produces the text and never touches the clipboard.
func (s *SessionService) RenderTerminalClipboardAction(ctx context.Context, actionID string, target dispatch.TerminalTarget, inputs map[string]string) (string, error) {
	return s.sessions.RenderTerminalClipboardAction(ctx, actionID, target, inputs)
}
