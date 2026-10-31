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
func (s *SessionService) ListSessions(ctx context.Context) ([]SessionSummary, error) {
	sessions, err := s.sessions.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	return sessionSummariesOf(s.sessions.CanonicalSpace, sessions), nil
}

// ListServerSessions returns the sessions of each server under servers:, in
// config order. A server that did not answer has a state, an error, and no
// sessions. It is a call of its own so the local list never waits on ssh.
func (s *SessionService) ListServerSessions(ctx context.Context) []ServerSessions {
	return serverSessionsOf(s.sessions.ListServerSessions(ctx))
}

func (s *SessionService) Servers(ctx context.Context) []app.ServerStatus {
	return s.sessions.Servers(ctx)
}

func (s *SessionService) ServerLaunchOptions(ctx context.Context, server string) (dispatch.SessionLaunchOptions, error) {
	return s.sessions.ServerLaunchOptions(ctx, server)
}

// SessionStatuses returns the current terminal-detected agent state for each
// active session.
func (s *SessionService) SessionStatuses(ctx context.Context) (app.SessionStatusSnapshot, error) {
	return s.sessions.SessionStatuses(ctx)
}

// ServerSessionStatuses returns the same state for the sessions on servers. It
// is a call of its own so local status never waits on ssh.
func (s *SessionService) ServerSessionStatuses(ctx context.Context) []app.SessionStatus {
	return s.sessions.ServerSessionStatuses(ctx)
}

// SessionDetail reads one session in full, for the detail view.
func (s *SessionService) SessionDetail(ctx context.Context, id string) (SessionDetail, error) {
	detail, err := s.sessions.SessionDetail(ctx, id)
	if err != nil {
		return SessionDetail{}, err
	}
	return sessionDetailOf(id, detail), nil
}

// SessionGitStatus reads the session's checkout for the session status bar.
// Cheap and local — four git subprocesses — so the bar polls it, unlike
// SessionPullRequest.
func (s *SessionService) SessionGitStatus(ctx context.Context, id string) (SessionGitStatus, error) {
	status, err := s.sessions.SessionGitStatus(ctx, id)
	if err != nil {
		return SessionGitStatus{}, err
	}
	return sessionGitStatusOf(status), nil
}

// SessionChanges lists what differs in the session's checkout for a target. The
// review view polls it while open.
func (s *SessionService) SessionChanges(ctx context.Context, id string, target DiffTarget) (SessionChanges, error) {
	changes, err := s.sessions.SessionChanges(ctx, id, target.gitstatus())
	if err != nil {
		return SessionChanges{}, err
	}
	return sessionChangesOf(changes), nil
}

// SessionFileDiffs reads the diffs of paths for a target. Only a path in the
// current change list is read.
func (s *SessionService) SessionFileDiffs(ctx context.Context, id string, target DiffTarget, paths []string) ([]SessionFileDiff, error) {
	diffs, err := s.sessions.SessionFileDiffs(ctx, id, target.gitstatus(), paths)
	if err != nil {
		return nil, err
	}
	return sessionFileDiffsOf(diffs), nil
}

// SessionBranches lists the full refs the session's checkout can compare against.
func (s *SessionService) SessionBranches(ctx context.Context, id string) ([]string, error) {
	return s.sessions.SessionBranches(ctx, id)
}

// SessionPullRequest resolves the pull request for a branch SessionGitStatus
// reported, answering from a short-lived cache unless refresh is set. Its
// Status field says why there is nothing to show, so a caller never has to
// read an empty result as "none".
func (s *SessionService) SessionPullRequest(ctx context.Context, key app.SessionPullRequestKey, refresh bool) (app.SessionPullRequest, error) {
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
func (s *SessionService) ItemSessions(ctx context.Context, itemID int64) ([]app.ItemSessionView, error) {
	return s.sessions.ItemSessions(ctx, itemID)
}

// ItemChats returns the agent workspace chats an inbox item opened, newest
// first.
func (s *SessionService) ItemChats(ctx context.Context, itemID int64) ([]app.ItemChatView, error) {
	return s.sessions.ItemChats(ctx, itemID)
}

// SessionRisk reports the uncommitted or unpushed work a delete or recycle
// would discard, for the confirmation that precedes one.
func (s *SessionService) SessionRisk(ctx context.Context, id string) (app.SessionRisk, error) {
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

// EditSession renames a session and replaces its tags, and returns its new
// summary. The slug in it is the new tmux target: renaming re-slugs, so an
// attached caller has to re-attach under the name that comes back.
func (s *SessionService) EditSession(ctx context.Context, id, name string, tags []string) (SessionSummary, error) {
	edited, err := s.sessions.EditSession(ctx, id, name, tags)
	if err != nil {
		return SessionSummary{}, err
	}
	if server := serverOf(id); server != "" {
		return serverSummaryOf(server, edited), nil
	}
	return sessionSummaryOf(s.sessions.CanonicalSpace, edited), nil
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
