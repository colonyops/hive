package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/colonyops/hive/internal/domain/session"

	"github.com/colonyops/hive/pkg/osopen"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/cmd/desktop/internal/app/activity"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
	"github.com/colonyops/hive/cmd/desktop/internal/app/execenv"
)

// Job action ids label session jobs in the jobs UI.
const (
	newSessionJobActionID        = "new-session"
	deleteSessionJobActionID     = "delete-session"
	recycleSessionJobActionID    = "recycle-session"
	pruneSessionsJobActionID     = "prune-sessions"
	sessionRenameRollbackTimeout = 5 * time.Second
)

type sessionLauncher interface {
	LaunchSession(context.Context, dispatch.LaunchSessionRequest) (dispatch.SessionExecutionOutcome, error)
	SessionLaunchOptions(context.Context) (dispatch.SessionLaunchOptions, error)
}

type workspaceSessionLauncher interface {
	dispatch.WorkspaceSessionLauncher
	SessionLaunchWorkspaces(context.Context) []dispatch.SessionLaunchWorkspace
}

// sessionManager is the read and lifecycle half of the session surface, kept
// apart from sessionLauncher because launching is a dispatch action: an output
// command holds a launcher, and it has no business holding a delete.
type sessionManager interface {
	ListSessions(context.Context) ([]dispatch.SessionSummary, error)
	SessionDetail(ctx context.Context, id string) (dispatch.SessionDetail, error)
	SessionRisk(ctx context.Context, id string) (dispatch.SessionRisk, error)
	RenameSession(ctx context.Context, id, name string) error
	DeleteSession(ctx context.Context, id string) error
	RecycleSession(ctx context.Context, id string) error
	PruneSessions(ctx context.Context) (int, error)
	SpawnTmuxSession(ctx context.Context, name, path, repo string) error
}

type sessionStatusSource interface {
	SessionStatuses(context.Context) (dispatch.SessionStatusSnapshot, error)
	RunningSessions(ctx context.Context, ids []string) (map[string]bool, error)
}

// sessionGitSource reads a session's checkout. Its own interface rather than a
// tenth method on sessionManager: reading git is not part of managing a
// session's lifecycle.
type sessionGitSource interface {
	SessionGitStatus(ctx context.Context, id string) (dispatch.SessionGitStatus, error)
}

// sessionTmux preflights a live tmux rename before Hive updates the record.
// The shared Hive service repeats the lifecycle operation to persist its actual
// multiplexer target; Desktop's manager treats that second, absent source as a
// successful no-op.
type sessionTmux interface {
	RenameSessionIfPresent(ctx context.Context, from, to string) (bool, error)
}

// sessionJobRunner runs slow session work as a tracked background job so a
// clone, a worktree removal or a prune does not block the caller.
type sessionJobRunner interface {
	Track(ctx context.Context, label, actionID, target string, fn func(context.Context) error) int64
}

type inboxItemRefReader interface {
	RefByID(ctx context.Context, itemID int64) (models.ItemRef, error)
}

// itemSessionStore owns durable item-session associations and removes links
// to sessions Hive no longer has.
type itemSessionStore interface {
	List(ctx context.Context, ref models.ItemRef) ([]stores.ItemSession, error)
	ListChats(ctx context.Context, ref models.ItemRef) ([]stores.ItemChat, error)
	Unlink(ctx context.Context, sessionIDs []string) error
}

// SessionsService is the desktop's session surface: the New Session form's
// launch path, read plus lifecycle management of the sessions that exist, and
// the configured actions a terminal session or window offers.
type SessionsService struct {
	launcher          sessionLauncher
	workspaceLauncher workspaceSessionLauncher
	manager           sessionManager
	statuses          sessionStatusSource
	git               sessionGitSource
	tmux              sessionTmux
	agentWindows      sessionAgentWindows
	agentCommands     func() map[string]string
	jobs              sessionJobRunner
	items             inboxItemRefReader
	links             itemSessionStore
	catalog           *actions.ActionStore
	dispatcher        *dispatch.Dispatcher
	recorder          activity.Recorder
	// pullRequests is nil in a build with no GitHub client, which reads as
	// disconnected.
	pullRequests  *sessionPullRequests
	execEnv       *execenv.Resolver
	editorCommand EditorCommandReader
	// defaultAgentEnv is never nil: newSessionsService substitutes
	// NopDefaultAgentReader, so withEnvironmentDefaultAgent never guards it.
	defaultAgentEnv DefaultAgentReader
	events          *events.Bus
	logger          zerolog.Logger

	// The last failed New Session form, so reopening restores what was typed.
	// One slot, because the form has one instance; not persisted, because the
	// activity row recordFailedCreate writes is the durable half
	// (ADR a-failed-session-creation-is-a-retryable-draft).
	failedCreateMu sync.Mutex
	failedCreate   *dispatch.SessionDraft
}

// DefaultAgentReader reads HIVE_DEFAULT_AGENT from the user's resolved
// terminal environment.
type DefaultAgentReader interface {
	DefaultAgent(ctx context.Context) string
}

// NopDefaultAgentReader leaves Hive's configured agent unchanged.
type NopDefaultAgentReader struct{}

func (NopDefaultAgentReader) DefaultAgent(context.Context) string { return "" }

type NopEditorCommandReader struct{}

func (NopEditorCommandReader) Editor(context.Context) (string, error) { return "", nil }

type SessionsDeps struct {
	Launcher          sessionLauncher
	WorkspaceLauncher workspaceSessionLauncher
	Manager           sessionManager
	Statuses          sessionStatusSource
	Git               sessionGitSource
	Tmux              sessionTmux
	AgentWindows      sessionAgentWindows
	AgentCommands     func() map[string]string
	Jobs              sessionJobRunner
	Items             inboxItemRefReader
	Links             itemSessionStore
	Catalog           *actions.ActionStore
	Dispatcher        *dispatch.Dispatcher
	Recorder          activity.Recorder
	PullRequests      *sessionPullRequests
	ExecEnv           *execenv.Resolver
	// EditorCommand reads the configured editor from settings on every call,
	// so a settings change applies without restarting. nil means
	// NopEditorCommandReader.
	EditorCommand EditorCommandReader
	// DefaultAgentEnv reads HIVE_DEFAULT_AGENT the way the user's terminal
	// would. nil means NopDefaultAgentReader.
	DefaultAgentEnv DefaultAgentReader
	Events          *events.Bus
	Logger          zerolog.Logger
}

func newSessionsService(d SessionsDeps) *SessionsService {
	if d.EditorCommand == nil {
		d.EditorCommand = NopEditorCommandReader{}
	}
	if d.DefaultAgentEnv == nil {
		d.DefaultAgentEnv = NopDefaultAgentReader{}
	}
	return &SessionsService{
		launcher:          d.Launcher,
		workspaceLauncher: d.WorkspaceLauncher,
		manager:           d.Manager,
		statuses:          d.Statuses,
		git:               d.Git,
		tmux:              d.Tmux,
		agentWindows:      d.AgentWindows,
		agentCommands:     d.AgentCommands,
		jobs:              d.Jobs,
		items:             d.Items,
		links:             d.Links,
		catalog:           d.Catalog,
		dispatcher:        d.Dispatcher,
		recorder:          d.Recorder,
		pullRequests:      d.PullRequests,
		execEnv:           d.ExecEnv,
		editorCommand:     d.EditorCommand,
		defaultAgentEnv:   d.DefaultAgentEnv,
		events:            d.Events,
		logger:            d.Logger,
	}
}

// SessionLaunchOptions supplies the repository, workspace, and agent choices
// the New Session form presents.
func (s *SessionsService) SessionLaunchOptions(ctx context.Context) (dispatch.SessionLaunchOptions, error) {
	opts, err := s.launcher.SessionLaunchOptions(ctx)
	if err != nil {
		return dispatch.SessionLaunchOptions{}, Wrap(err, KindInternal, "resolving session launch options")
	}
	opts.Workspaces = s.SessionLaunchWorkspaces(ctx)
	return s.withEnvironmentDefaultAgent(ctx, opts), nil
}

// SessionLaunchWorkspaces supplies workspace choices without resolving
// repository options.
func (s *SessionsService) SessionLaunchWorkspaces(ctx context.Context) []dispatch.SessionLaunchWorkspace {
	if s.workspaceLauncher == nil {
		return nil
	}
	return s.workspaceLauncher.SessionLaunchWorkspaces(ctx)
}

// withEnvironmentDefaultAgent preselects the agent hive itself would run:
// HIVE_DEFAULT_AGENT when it names a configured profile, otherwise the
// agents.default hive's config already resolved (colonyops/hive#365). The
// variable comes from the user's terminal rather than this process, because a
// launched .app has neither it nor anything else a startup file exports
// (ADR hive-env-overrides-resolve-through-the-login-shell).
//
// An unknown profile is ignored rather than refused: preselecting is all this
// does, and hive validates the agent it is handed.
func (s *SessionsService) withEnvironmentDefaultAgent(ctx context.Context, opts dispatch.SessionLaunchOptions) dispatch.SessionLaunchOptions {
	if preferred := s.preferredEnvAgent(ctx, opts.Agents); preferred != "" {
		opts.DefaultAgent = preferred
	}
	return opts
}

// preferredEnvAgent answers HIVE_DEFAULT_AGENT when it names one of the given
// configured profiles, otherwise "". Both the form (withEnvironmentDefaultAgent)
// and the launch path (resolveLaunchAgent) preselect the same env override
// against the same profile list, so the check lives here once.
//
// An unknown profile answers "" rather than the raw env value: hive refuses a
// LaunchSessionRequest.Agent it does not recognize with `unknown agent %q`,
// which would turn a harmless preselection into a failed launch.
func (s *SessionsService) preferredEnvAgent(ctx context.Context, agents []string) string {
	return environmentAgentOverride(s.defaultAgentEnv.DefaultAgent(ctx), agents)
}

// resolveLaunchAgent answers the agent a launch should run when the request
// names none: HIVE_DEFAULT_AGENT when it names a configured profile,
// otherwise "" so hive resolves its own agents.default. A launch-options read
// failure also falls through to "": a create must not start failing because a
// preselection could not be read.
//
// This runs synchronously on the click that starts a session
// (CreateSession builds the request before handing off to the job), so the
// options read -- a git subprocess per configured workspace
// (cmd/desktop/frontend/src/composables/useNewSession.ts) -- is gated on there
// being an override to validate. The env read is a cached map lookup, so
// checking it first is free for the common case of no override at all.
func (s *SessionsService) resolveLaunchAgent(ctx context.Context, requested string) string {
	if trimmed := strings.TrimSpace(requested); trimmed != "" {
		return trimmed
	}
	if strings.TrimSpace(s.defaultAgentEnv.DefaultAgent(ctx)) == "" {
		return ""
	}
	opts, err := s.launcher.SessionLaunchOptions(ctx)
	if err != nil {
		return ""
	}
	return s.preferredEnvAgent(ctx, opts.Agents)
}

// ListSessions returns every session, whatever its state. Only an active one
// can be attached to — Slug is its tmux session name — but a recycled or
// corrupted session still has to be readable and deletable.
func (s *SessionsService) ListSessions(ctx context.Context) ([]dispatch.SessionSummary, error) {
	sessions, err := s.manager.ListSessions(ctx)
	if err != nil {
		return nil, Wrap(err, KindInternal, "listing sessions")
	}
	return sessions, nil
}

// SessionStatuses detects the live agent state for active sessions. Missing or
// unavailable terminals are data, so only a failure to read the session set
// fails the request.
func (s *SessionsService) SessionStatuses(ctx context.Context) (dispatch.SessionStatusSnapshot, error) {
	statuses, err := s.statuses.SessionStatuses(ctx)
	if err != nil {
		return dispatch.SessionStatusSnapshot{}, Wrap(err, KindInternal, "reading session status")
	}
	return statuses, nil
}

// ItemChats returns the agent workspace chats an inbox item opened, newest
// first.
func (s *SessionsService) ItemChats(ctx context.Context, itemID int64) ([]dispatch.ItemChatView, error) {
	ref, err := s.items.RefByID(ctx, itemID)
	if err != nil {
		if stores.IsNotFound(err) {
			return nil, Wrap(err, KindNotFound, "inbox item %d not found", itemID)
		}
		return nil, Wrap(err, KindInternal, "reading inbox item %d", itemID)
	}
	chats, err := s.links.ListChats(ctx, ref)
	if err != nil {
		return nil, Wrap(err, KindInternal, "listing chats for item %d", itemID)
	}
	views := make([]dispatch.ItemChatView, 0, len(chats))
	for _, chat := range chats {
		views = append(views, dispatch.ItemChatView{
			ID: chat.ChatID, Workspace: chat.Workspace, Name: chat.Name, CreatedAt: time.UnixMilli(chat.CreatedAt),
		})
	}
	return views, nil
}

// ItemSessions returns the hive sessions an inbox item spawned, newest first,
// joined to the state hive reports for them now. The read is also what
// reconciles (ADR macos-dmg-installer).
func (s *SessionsService) ItemSessions(ctx context.Context, itemID int64) ([]dispatch.ItemSessionView, error) {
	ref, err := s.items.RefByID(ctx, itemID)
	if err != nil {
		if stores.IsNotFound(err) {
			return nil, Wrap(err, KindNotFound, "inbox item %d not found", itemID)
		}
		return nil, Wrap(err, KindInternal, "reading inbox item %d", itemID)
	}
	links, err := s.links.List(ctx, ref)
	if err != nil {
		return nil, Wrap(err, KindInternal, "listing sessions for item %d", itemID)
	}
	if len(links) == 0 {
		return []dispatch.ItemSessionView{}, nil
	}

	sessions, err := s.manager.ListSessions(ctx)
	if err != nil {
		return nil, Wrap(err, KindInternal, "listing sessions")
	}
	known := make(map[string]dispatch.SessionSummary, len(sessions))
	for _, summary := range sessions {
		known[summary.ID] = summary
	}

	views := make([]dispatch.ItemSessionView, 0, len(links))
	ids := make([]string, 0, len(links))
	var gone []string
	for _, link := range links {
		summary, ok := known[link.SessionID]
		if !ok {
			gone = append(gone, link.SessionID)
			continue
		}
		ids = append(ids, summary.ID)
		views = append(views, dispatch.ItemSessionView{
			ID:        summary.ID,
			Name:      summary.Name,
			Slug:      summary.Slug,
			Repo:      summary.Repo,
			State:     summary.State,
			CreatedAt: time.UnixMilli(link.CreatedAt),
		})
	}
	if err := s.links.Unlink(ctx, gone); err != nil {
		// The view above is already correct without the prune; failing the read
		// over a cleanup would hide the sessions that do still exist.
		s.logger.Warn().Err(err).Int64("item_id", itemID).Msg("dropping links to deleted sessions")
	}

	running, err := s.statuses.RunningSessions(ctx, ids)
	if err != nil {
		// Same reason as the prune above: the views are already correct
		// without liveness, and failing the read blanks a pane that had an
		// answer.
		s.logger.Warn().Err(err).Int64("item_id", itemID).Msg("reading session status")
		return views, nil
	}
	for i := range views {
		views[i].Running = running[views[i].ID]
	}
	return views, nil
}

// SessionDetail reads one session in full.
func (s *SessionsService) SessionDetail(ctx context.Context, id string) (dispatch.SessionDetail, error) {
	if strings.TrimSpace(id) == "" {
		return dispatch.SessionDetail{}, Errorf(KindInvalid, "session id is required")
	}
	detail, err := s.manager.SessionDetail(ctx, id)
	if err != nil {
		return dispatch.SessionDetail{}, Wrap(err, KindNotFound, "reading session %q", id)
	}
	return detail, nil
}

// SessionGitStatus reads the session's checkout for the status bar: branch,
// dirty, unpushed, and the line delta against the default branch. A session
// with no live checkout answers an unresolved status rather than an error —
// there is nothing wrong, there is just nothing to read.
func (s *SessionsService) SessionGitStatus(ctx context.Context, id string) (dispatch.SessionGitStatus, error) {
	if strings.TrimSpace(id) == "" {
		return dispatch.SessionGitStatus{}, Errorf(KindInvalid, "session id is required")
	}
	status, err := s.git.SessionGitStatus(ctx, id)
	if err != nil {
		return dispatch.SessionGitStatus{}, Wrap(err, KindNotFound, "reading git status for session %q", id)
	}
	return status, nil
}

// SessionPullRequest resolves the pull request for a branch SessionGitStatus
// already reported. The branch is passed rather than read again: resolving it
// costs a git subprocess the caller has just paid for, and the two halves
// refresh on different cadences.
func (s *SessionsService) SessionPullRequest(ctx context.Context, key dispatch.SessionPullRequestKey, refresh bool) (dispatch.SessionPullRequest, error) {
	if s.pullRequests == nil {
		return dispatch.SessionPullRequest{Status: dispatch.PullRequestStatusDisconnected}, nil
	}
	return s.pullRequests.Lookup(ctx, key, refresh)
}

// OpenSessionInEditor launches the configured editor on the session's
// checkout, detached. It takes a session id, never a path: launching a
// configured program on a caller-supplied directory is not this API's to offer.
func (s *SessionsService) OpenSessionInEditor(ctx context.Context, id string) error {
	dir, err := s.sessionCheckout(ctx, id)
	if err != nil {
		return err
	}
	command := ""
	if configured, err := s.editorCommand.Editor(ctx); err == nil {
		command = configured
	}
	return launchEditor(ctx, s.execEnv, command, dir)
}

// RevealSession opens the session's checkout in the OS file manager.
func (s *SessionsService) RevealSession(ctx context.Context, id string) error {
	dir, err := s.sessionCheckout(ctx, id)
	if err != nil {
		return err
	}
	return Wrap(osopen.Open(dir), KindInternal, "opening %s", dir)
}

// sessionCheckout resolves the directory a session's own record names, which
// is what keeps open and reveal off arbitrary paths.
func (s *SessionsService) sessionCheckout(ctx context.Context, id string) (string, error) {
	detail, err := s.SessionDetail(ctx, id)
	if err != nil {
		return "", err
	}
	if detail.Path == "" {
		return "", Errorf(KindConflict, "session %q has no checkout", detail.Name)
	}
	return detail.Path, nil
}

// SessionRisk reports the work a delete or recycle of id would discard, so the
// caller can name it in its confirmation instead of guessing.
func (s *SessionsService) SessionRisk(ctx context.Context, id string) (dispatch.SessionRisk, error) {
	if strings.TrimSpace(id) == "" {
		return dispatch.SessionRisk{}, Errorf(KindInvalid, "session id is required")
	}
	risk, err := s.manager.SessionRisk(ctx, id)
	if err != nil {
		return dispatch.SessionRisk{}, Wrap(err, KindNotFound, "checking session %q", id)
	}
	return risk, nil
}

// CreateSession validates a New Session form, then launches a repository
// session or workspace chat as a background job and returns its id. Creation
// runs asynchronously and its outcome surfaces in the jobs UI, so only
// validation errors are returned here.
func (s *SessionsService) CreateSession(ctx context.Context, req dispatch.CreateSessionRequest) (int64, error) {
	repo := strings.TrimSpace(req.Repository)
	workspace := strings.TrimSpace(req.Workspace)
	if (repo == "") == (workspace == "") {
		return 0, Errorf(KindInvalid, "exactly one of repository or workspace is required")
	}
	if workspace != "" && s.workspaceLauncher == nil {
		return 0, Errorf(KindUnavailable, "agent workspace sessions are unavailable")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return 0, Errorf(KindInvalid, "session name is required")
	}
	if err := session.ValidateName(name); err != nil {
		return 0, Wrap(err, KindInvalid, "session name")
	}

	// Items pruned between opening and submitting the form are omitted rather
	// than blocking the one session the user asked for.
	var origins []models.ItemRef
	seenIDs := make(map[int64]struct{}, len(req.ItemIDs))
	seenOrigins := make(map[models.ItemRef]struct{}, len(req.ItemIDs))
	for _, itemID := range req.ItemIDs {
		if itemID <= 0 {
			continue
		}
		if _, exists := seenIDs[itemID]; exists {
			continue
		}
		seenIDs[itemID] = struct{}{}
		resolved, err := s.items.RefByID(ctx, itemID)
		if err != nil {
			if stores.IsNotFound(err) {
				continue
			}
			return 0, Wrap(err, KindInternal, "reading inbox item %d", itemID)
		}
		if _, exists := seenOrigins[resolved]; exists {
			continue
		}
		seenOrigins[resolved] = struct{}{}
		origins = append(origins, resolved)
	}

	prompt := strings.TrimSpace(req.Prompt)
	agent := ""
	if repo != "" {
		agent = s.resolveLaunchAgent(ctx, req.Agent)
	}
	launch := dispatch.LaunchSessionRequest{
		Name: name, Prompt: prompt, Agent: agent, Repo: repo, Origins: origins,
	}
	// The anchor for the failure. hive logs "cloning repository" with dest= and
	// strategy= but names no session, so without a line either side of it a log
	// holding several creates cannot say whose path that was.
	s.logger.Info().
		Str("session_name", name).
		Str("repository", repo).
		Str("workspace", workspace).
		Str("agent", launch.Agent).
		Ints64("item_ids", req.ItemIDs).
		Msg("creating session")

	// Submitting retires the previous failure: leaving it pending would hand the
	// old attempt back over the session just asked for.
	s.setFailedCreate(nil)
	jobID := s.jobs.Track(ctx, "Create session", newSessionJobActionID, name, func(bg context.Context) error {
		var err error
		if workspace != "" {
			_, err = s.workspaceLauncher.LaunchWorkspaceSession(bg, dispatch.LaunchWorkspaceSessionRequest{
				Workspace: workspace, Name: name, Prompt: prompt, Origins: launch.Origins,
			})
		} else {
			_, err = s.launcher.LaunchSession(bg, launch)
		}
		if err == nil {
			return nil
		}
		if errors.Is(err, session.ErrDuplicateName) {
			return fmt.Errorf("a session named %q already exists", name)
		}
		return s.recordFailedCreate(bg, req, err)
	})
	return jobID, nil
}

// recordFailedCreate returns the error the job records.
//
// A name collision never reaches here: CreateSession answers that itself,
// because a duplicate name is a form error fixed in the field, not a failure
// to report against the whole attempt.
func (s *SessionsService) recordFailedCreate(ctx context.Context, req dispatch.CreateSessionRequest, err error) error {
	failure := dispatch.SessionCreateFailure{Reason: err.Error(), At: time.Now()}
	var detail *dispatch.SessionCreateError
	if errors.As(err, &detail) {
		failure = dispatch.SessionCreateFailure{
			Reason:           detail.Err.Error(),
			Step:             detail.Step,
			Output:           detail.Output,
			CloneStrategy:    detail.CloneStrategy,
			Destination:      detail.Destination,
			LeftoverCheckout: detail.LeftoverCheckout,
			At:               time.Now(),
		}
	}

	// One flat chain, every field present even when empty: a line whose shape
	// depends on what was known is one a log query cannot rely on.
	var remote string
	if detail != nil {
		remote = detail.Remote
	}
	s.logger.Error().
		Str("session_name", strings.TrimSpace(req.Name)).
		Str("repository", strings.TrimSpace(req.Repository)).
		Str("workspace", strings.TrimSpace(req.Workspace)).
		Str("remote", remote).
		Str("clone_strategy", failure.CloneStrategy).
		Str("destination", failure.Destination).
		Bool("leftover_checkout", failure.LeftoverCheckout).
		Str("step", failure.Step).
		Str("progress", failure.Output).
		Err(err).
		Msg("creating session failed")

	draft := dispatch.SessionDraft{
		Repository: strings.TrimSpace(req.Repository),
		Workspace:  strings.TrimSpace(req.Workspace),
		Name:       strings.TrimSpace(req.Name),
		Prompt:     strings.TrimSpace(req.Prompt),
		Agent:      req.Agent,
		ItemIDs:    append([]int64(nil), req.ItemIDs...),
		Failure:    &failure,
	}
	s.setFailedCreate(&draft)
	// The durable half: the toast and the slot die with the process, the row
	// does not, and it carries the form to come back to.
	if s.recorder != nil {
		target := draft.Repository
		if target == "" {
			target = draft.Workspace
		}
		s.recorder.Record(ctx, activity.SessionCreateFailed(
			draft.Name, target, failure.Step, failure.Reason,
			dispatch.SessionDraftMetadata(draft),
		))
	}
	if s.events != nil {
		s.events.Publish(ctx, events.SessionCreateFailed{Name: draft.Name})
	}
	return err
}

// FailedSessionDraft returns the last failed New Session form. A nil Failure
// is what says there is no attempt waiting.
func (s *SessionsService) FailedSessionDraft(context.Context) (dispatch.SessionDraft, error) {
	s.failedCreateMu.Lock()
	defer s.failedCreateMu.Unlock()
	if s.failedCreate == nil {
		return dispatch.SessionDraft{}, nil
	}
	return *s.failedCreate, nil
}

// SessionDraftFromActivity decodes the form an activity row carries. It takes
// the row's metadata rather than its id because the caller already holds the
// row it rendered, and forwarding an opaque bag keeps the encoding in Go.
//
// A bag carrying no retry is KindInvalid rather than an empty draft: a form
// silently prefilled with nothing is worse than a refused one.
func (s *SessionsService) SessionDraftFromActivity(_ context.Context, metadata map[string]string) (dispatch.SessionDraft, error) {
	draft, ok := dispatch.SessionDraftFromMetadata(metadata)
	if !ok {
		return dispatch.SessionDraft{}, Errorf(KindInvalid, "this activity event carries no session to retry")
	}
	return draft, nil
}

// DismissFailedSession drops the pending failed attempt.
func (s *SessionsService) DismissFailedSession(context.Context) error {
	s.setFailedCreate(nil)
	return nil
}

func (s *SessionsService) setFailedCreate(draft *dispatch.SessionDraft) {
	s.failedCreateMu.Lock()
	defer s.failedCreateMu.Unlock()
	s.failedCreate = draft
}

// RenameSession renames a session and returns its new summary.
//
// Desktop preflights the live tmux rename so a collision cannot move the record
// onto another session's target. The shared Hive service then updates the
// record and its persisted multiplexer target. If that update fails, Desktop
// puts the live tmux name back.
func (s *SessionsService) RenameSession(ctx context.Context, id, name string) (dispatch.SessionSummary, error) {
	if strings.TrimSpace(id) == "" {
		return dispatch.SessionSummary{}, Errorf(KindInvalid, "session id is required")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return dispatch.SessionSummary{}, Errorf(KindInvalid, "session name is required")
	}
	if err := session.ValidateName(name); err != nil {
		return dispatch.SessionSummary{}, Wrap(err, KindInvalid, "session name")
	}

	current, err := s.manager.SessionDetail(ctx, id)
	if err != nil {
		return dispatch.SessionSummary{}, Wrap(err, KindNotFound, "reading session %q", id)
	}
	slug := session.Slugify(name)
	if err := s.assertSlugFree(ctx, id, slug); err != nil {
		return dispatch.SessionSummary{}, err
	}

	tmuxSession := current.TmuxSession
	if tmuxSession == "" {
		tmuxSession = current.Slug
	}
	tmuxRenamed, err := s.tmux.RenameSessionIfPresent(ctx, tmuxSession, slug)
	if err != nil {
		return dispatch.SessionSummary{}, Wrap(err, KindConflict, "renaming the terminal session for %q", current.Name)
	}
	if err := s.manager.RenameSession(ctx, id, name); err != nil {
		if tmuxRenamed {
			rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sessionRenameRollbackTimeout)
			defer cancel()
			if _, rollbackErr := s.tmux.RenameSessionIfPresent(rollbackCtx, slug, tmuxSession); rollbackErr != nil {
				return dispatch.SessionSummary{}, Wrap(err, KindInternal, "renaming session %q (its terminal session is now named %q)", current.Name, slug)
			}
		}
		return dispatch.SessionSummary{}, Wrap(err, KindInternal, "renaming session %q", current.Name)
	}

	renamed, err := s.manager.SessionDetail(ctx, id)
	if err != nil {
		return dispatch.SessionSummary{}, Wrap(err, KindInternal, "re-reading renamed session %q", id)
	}
	return dispatch.SessionSummary{
		ID:    renamed.ID,
		Name:  renamed.Name,
		Slug:  renamed.Slug,
		Repo:  renamed.Repo,
		State: renamed.State,
	}, nil
}

// StartTmuxSession spawns the tmux session a slug names. A session hive knows
// about does not have to have one — its tmux server was restarted, the machine
// rebooted, or it was created by something that never spawned one — and this is
// what lets the terminal attach to it anyway.
//
// The windows are hive's: what a session's terminal holds is its spawn
// configuration for the remote, and a second definition of that here would
// drift from the one `hive` itself uses. Spawning a session tmux already has is
// a no-op, so this is safe to call before every cold attach.
func (s *SessionsService) StartTmuxSession(ctx context.Context, slug string) error {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return Errorf(KindInvalid, "session slug is required")
	}

	detail, err := s.detailBySlug(ctx, slug)
	if err != nil {
		return err
	}
	if detail.State != string(session.StateActive) {
		return Errorf(KindConflict, "session %q is %s, so there is no checkout left to open a terminal in", detail.Name, detail.State)
	}
	// Hive spawns under the slug it derives from the name, so a record whose two
	// disagree would create a tmux session nothing is attaching to (ADR session-rename-keeps-slug-and-tmux-in-step).
	if spawned := session.Slugify(detail.Name); spawned != slug {
		return Errorf(KindConflict, "session %q would start as %q, not %q", detail.Name, spawned, slug)
	}
	if err := s.manager.SpawnTmuxSession(ctx, detail.Name, detail.Path, detail.Repo); err != nil {
		return Wrap(err, KindInternal, "starting the terminal session for %q", detail.Name)
	}
	return nil
}

// SessionDirectory answers the checkout a slug's terminal should open in. A
// session with no checkout left is a conflict rather than an empty path, so a
// terminal is never opened somewhere the caller did not ask for.
func (s *SessionsService) SessionDirectory(ctx context.Context, slug string) (string, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return "", Errorf(KindInvalid, "session slug is required")
	}
	detail, err := s.detailBySlug(ctx, slug)
	if err != nil {
		return "", err
	}
	if detail.State != string(session.StateActive) {
		return "", Errorf(KindConflict, "session %q is %s, so there is no checkout left to open a terminal in", detail.Name, detail.State)
	}
	return detail.Path, nil
}

// detailBySlug reads the session a tmux session name belongs to. The listing is
// what maps a slug to an id; nothing queries hive by slug.
func (s *SessionsService) detailBySlug(ctx context.Context, slug string) (dispatch.SessionDetail, error) {
	sessions, err := s.manager.ListSessions(ctx)
	if err != nil {
		return dispatch.SessionDetail{}, Wrap(err, KindInternal, "listing sessions")
	}
	for _, summary := range sessions {
		if summary.Slug != slug {
			continue
		}
		detail, err := s.manager.SessionDetail(ctx, summary.ID)
		if err != nil {
			return dispatch.SessionDetail{}, Wrap(err, KindNotFound, "reading session %q", summary.Name)
		}
		return detail, nil
	}
	return dispatch.SessionDetail{}, Errorf(KindNotFound, "no session named %q", slug)
}

// DeleteSession removes a session, its worktree or clone, and its tmux session,
// as a background job: it runs git and filesystem work that a caller must not
// block on. The caller is expected to have confirmed against SessionRisk first.
func (s *SessionsService) DeleteSession(ctx context.Context, id string) (int64, error) {
	return s.destructiveJob(ctx, id, "Delete session", deleteSessionJobActionID, s.manager.DeleteSession)
}

// RecycleSession resets a session for reuse, as a background job for the same
// reason DeleteSession is one. For a worktree session hive recycles by
// deleting, which is what SessionRisk.RecycleDeletes warns about.
func (s *SessionsService) RecycleSession(ctx context.Context, id string) (int64, error) {
	return s.destructiveJob(ctx, id, "Recycle session", recycleSessionJobActionID, s.manager.RecycleSession)
}

// PruneSessions deletes every recycled and corrupted session as a background
// job, and reports the job id rather than the count: the work outlives the
// call, so the count is not known yet.
func (s *SessionsService) PruneSessions(ctx context.Context) (int64, error) {
	return s.jobs.Track(ctx, "Prune sessions", pruneSessionsJobActionID, "", func(bg context.Context) error {
		_, err := s.manager.PruneSessions(bg)
		return err
	}), nil
}

// destructiveJob resolves the session's name for the job label, then runs op in
// the background. The name is read up front because after the operation there
// may be no session left to read it from.
func (s *SessionsService) destructiveJob(ctx context.Context, id, label, actionID string, op func(context.Context, string) error) (int64, error) {
	if strings.TrimSpace(id) == "" {
		return 0, Errorf(KindInvalid, "session id is required")
	}
	detail, err := s.manager.SessionDetail(ctx, id)
	if err != nil {
		return 0, Wrap(err, KindNotFound, "reading session %q", id)
	}
	return s.jobs.Track(ctx, label, actionID, detail.Name, func(bg context.Context) error {
		return op(bg, id)
	}), nil
}

// assertSlugFree rejects a rename that would give two sessions the same slug
// or live tmux target. Hive validates the new name but does not check it for
// collisions, and the session table has no uniqueness constraint.
func (s *SessionsService) assertSlugFree(ctx context.Context, id, slug string) error {
	sessions, err := s.manager.ListSessions(ctx)
	if err != nil {
		return Wrap(err, KindInternal, "checking existing session names")
	}
	for _, other := range sessions {
		if other.ID != id && (other.Slug == slug || other.TmuxSession == slug) {
			return Errorf(KindInvalid, "a session named %q already exists", other.Name)
		}
	}
	return nil
}
