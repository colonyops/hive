package dispatch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/activity"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/internal/core/git"
	"github.com/colonyops/hive/internal/core/messaging"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/colonyops/hive/internal/core/terminal/assess"
	"github.com/colonyops/hive/internal/hive"
)

// AgentActivityStatus is this app's own vocabulary for a captured tmux pane's
// detected state, projected from Hive's shared assessment engine at this seam
// so no app signature carries a shared hive type (Bounded Context,
// architecture.md).
type AgentActivityStatus string

const (
	// AgentActivityReady reports an input prompt: the agent is idle.
	AgentActivityReady AgentActivityStatus = AgentActivityStatus(terminal.StatusReady)
	// AgentActivityActive reports a busy indicator (spinner, "esc to interrupt").
	AgentActivityActive AgentActivityStatus = AgentActivityStatus(terminal.StatusActive)
	// AgentActivityApproval reports an approval or question blocking on the user.
	AgentActivityApproval AgentActivityStatus = AgentActivityStatus(terminal.StatusApproval)
)

// ClassifyAgentScreen classifies a captured tmux pane with Hive's stateless
// assessment engine. Questions project onto approval because the desktop's
// activity vocabulary has one user-blocked state.
func ClassifyAgentScreen(agent, screen string) AgentActivityStatus {
	assessment := assess.NewEngine().Assess(assess.Snapshot{Content: screen, Tool: strings.ToLower(agent)})
	switch assessment.State {
	case assess.StateWorking:
		return AgentActivityActive
	case assess.StateApproval, assess.StateQuestion:
		return AgentActivityApproval
	default:
		return AgentActivityReady
	}
}

// ErrDuplicateSessionName is the seam-local translation of Hive's
// session.ErrDuplicateName, so a core service can classify a name collision
// without importing the shared session package.
var ErrDuplicateSessionName = errors.New("session name already exists")

type SessionCreator interface {
	CreateSession(context.Context, hive.CreateOptions) (*session.Session, error)
}

type sessionLaunchOptionsSource interface {
	SessionLaunchOptions(context.Context) (hive.SessionLaunchOptions, error)
	ResolveSessionLaunchRepository(context.Context, string) (hive.SessionLaunchRepository, error)
}

// SessionManagement is the shared session surface the desktop manages
// sessions through. Every method matches hive's SessionService structurally, so
// a signature change in the shared package breaks this file rather than the core.
type SessionManagement interface {
	ListSessions(context.Context) ([]session.Session, error)
	GetSession(context.Context, string) (session.Session, error)
	RenameSession(ctx context.Context, id, newName string) error
	DeleteSession(ctx context.Context, id string) error
	RecycleSession(ctx context.Context, id string, w io.Writer) error
	Prune(ctx context.Context, all bool) (int, error)
	CheckSessionRisk(ctx context.Context, id string) (hive.SessionRisk, error)
	OpenTmuxSession(ctx context.Context, name, path, remote, targetWindow string, background bool) error
}

// SessionStateActive is the one state with a live checkout behind it, and so the
// only one whose terminal can be started or attached to.
const SessionStateActive = string(session.StateActive)

type sessionStatusSource interface {
	Available() bool
	FetchBatch(context.Context, []*session.Session, []hive.RootRepoTarget) map[string]hive.TerminalStatus
}

// SessionWindowRef joins Hive's index-based activity result to the stable tmux
// window id every desktop operation uses.
type SessionWindowRef struct {
	ID    string
	Index string
	Name  string
}

type sessionWindowSource interface {
	ListSessionWindows(context.Context, []string) (map[string][]SessionWindowRef, error)
}

// SessionSummary is one session as the desktop's session list sees it. The
// persisted tmux target stays internal; the rest is the frontend projection.
type SessionSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Repo        string `json:"repo"`
	State       string `json:"state"`
	TmuxSession string `json:"-"`
}

// ItemSessionView is one hive session an inbox item spawned. Only CreatedAt
// comes from the link — everything else is read live from hive, so a session
// renamed or recycled outside this app reports what it actually is.
type ItemSessionView struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Repo      string    `json:"repo"`
	State     string    `json:"state"`
	Running   bool      `json:"running"`
	CreatedAt time.Time `json:"createdAt"`
}

// ItemChatView is an agent workspace chat an inbox item opened.
type ItemChatView struct {
	ID        int64     `json:"id"`
	Workspace string    `json:"workspace"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

// SessionWindowStatus is one tmux window's detected agent activity.
type SessionWindowStatus struct {
	WindowID string `json:"windowId"`
	Status   string `json:"status"`
	Tool     string `json:"tool"`
}

// SessionStatus separates tmux liveness from the activity detected in each
// agent window.
type SessionStatus struct {
	SessionID string                `json:"sessionId"`
	Running   bool                  `json:"running"`
	Windows   []SessionWindowStatus `json:"windows"`
}

// SessionStatusSnapshot carries one poll result and the Hive-configured delay
// the caller should use before requesting the next one.
type SessionStatusSnapshot struct {
	Items        []SessionStatus
	PollInterval time.Duration
}

// SessionDetail is one session read in full, for a detail view.
type SessionDetail struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Slug           string    `json:"slug"`
	Repo           string    `json:"repo"`
	State          string    `json:"state"`
	Path           string    `json:"path"`
	CloneStrategy  string    `json:"cloneStrategy"`
	WorktreeBranch string    `json:"worktreeBranch"`
	TmuxSession    string    `json:"-"`
	Tags           []string  `json:"tags"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// SessionRisk is the pre-flight a destructive operation confirms against: what
// unsaved work the session holds, and whether recycling it is really a delete.
type SessionRisk struct {
	UncommittedChanges bool `json:"uncommittedChanges"`
	UnpushedCommits    bool `json:"unpushedCommits"`
	// RecycleDeletes reports that recycling this session destroys it: hive
	// routes a worktree session's recycle straight to DeleteSession, because a
	// worktree has no clone of its own to reset.
	RecycleDeletes bool `json:"recycleDeletes"`
}

// SessionGitStatus is one session's checkout as the session status bar reads
// it. Resolved separates "git answered" from the zero value; Error carries why
// a read failed and is never a substitute for it.
type SessionGitStatus struct {
	Path     string `json:"path"`
	Branch   string `json:"branch"`
	Dirty    bool   `json:"dirty"`
	Unpushed bool   `json:"unpushed"`
	// Additions and Deletions are lines against the default branch, not HEAD.
	Additions int `json:"additions"`
	Deletions int `json:"deletions"`
	// Host, Owner and Repo are the remote's coordinates, and are empty for a
	// remote that names no host — a local path or a bare clone URL.
	Host     string `json:"host"`
	Owner    string `json:"owner"`
	Repo     string `json:"repo"`
	Resolved bool   `json:"resolved"`
	Error    string `json:"error"`
}

// SessionPullRequestKey addresses the pull request a session's branch has.
// Host is what decides which forge is asked, so a lookup carries it rather
// than inferring one from owner and repo, which every forge spells the same.
type SessionPullRequestKey struct {
	Host   string `json:"host"`
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Branch string `json:"branch"`
}

// PullRequestStatus is why a session has no pull request to show, or that it
// does. The four are kept apart deliberately: rendering "no pull request" for
// a failed lookup or a disconnected account states a different, wrong fact.
type PullRequestStatus string

const (
	PullRequestStatusNone         PullRequestStatus = "none"
	PullRequestStatusFound        PullRequestStatus = "found"
	PullRequestStatusDisconnected PullRequestStatus = "disconnected"
	PullRequestStatusUnsupported  PullRequestStatus = "unsupported"
)

// SessionPullRequest is the branch's pull request as the session status bar
// shows it. Everything below Status is meaningful only for
// PullRequestStatusFound.
type SessionPullRequest struct {
	Status  PullRequestStatus `json:"status"`
	Number  int               `json:"number"`
	Title   string            `json:"title"`
	State   string            `json:"state"`
	IsDraft bool              `json:"isDraft"`
	URL     string            `json:"url"`
	// ReviewDecision is GitHub's own vocabulary — APPROVED,
	// CHANGES_REQUESTED, REVIEW_REQUIRED — or empty when review is not
	// required.
	ReviewDecision string `json:"reviewDecision"`
	// Checks is passing, pending, failing, or empty for a head commit with no
	// checks configured.
	Checks string `json:"checks"`
	// The pull request's own line counts, deliberately not SessionGitStatus's:
	// those measure the working tree and drift as the branch moves on.
	Additions int `json:"additions"`
	Deletions int `json:"deletions"`
	// Cached distinguishes "this just arrived" from "this was already known".
	// The bar animates only the former.
	Cached bool `json:"cached"`
}

// ItemSessionLinker persists the association between an inbox item and a
// session created for it, so the item can find the session again after a
// restart. Consumer-defined: the launcher needs one write, not a store.
type ItemSessionLinker interface {
	Link(ctx context.Context, sessionID string, ref models.ItemRef) error
}

// HiveSessionLauncher adapts Hive's session service to SessionLauncher.
type HiveSessionLauncher struct {
	// Config-derived state is atomic because Rebind can replace it at runtime.
	// Recorder and links outlive config edits. Constructors must call Rebind
	// before returning so hive() cannot load nil.
	current  atomic.Pointer[launcherHive]
	recorder activity.Recorder
	links    ItemSessionLinker
	logger   zerolog.Logger
}

type launcherHive struct{ sessions SessionCreator }

func NewHiveSessionLauncher(sessions SessionCreator) *HiveSessionLauncher {
	l := &HiveSessionLauncher{}
	l.Rebind(sessions)
	return l
}

// Rebind points the launcher at a Hive session service built from a freshly
// loaded config. In-flight calls finish against the service they started
// with; the next call takes the new one.
func (l *HiveSessionLauncher) Rebind(sessions SessionCreator) {
	l.current.Store(&launcherHive{sessions: sessions})
}

func (l *HiveSessionLauncher) hive() SessionCreator { return l.current.Load().sessions }

// SetRecorder attaches an activity recorder so created sessions surface in the
// Activity view. Optional: nil (the default) records nothing.
func (l *HiveSessionLauncher) SetRecorder(r activity.Recorder) { l.recorder = r }

// SetItemSessionLinker attaches the store that remembers which item a session
// came from. Optional: nil (the default) links nothing, and the session is
// still created.
func (l *HiveSessionLauncher) SetItemSessionLinker(links ItemSessionLinker, logger zerolog.Logger) {
	l.links, l.logger = links, logger
}

func (l *HiveSessionLauncher) LaunchSession(ctx context.Context, req LaunchSessionRequest) (SessionExecutionOutcome, error) {
	sessions := l.hive()
	if sessions == nil {
		return SessionExecutionOutcome{}, fmt.Errorf("launch session: hive session service is unavailable")
	}
	remote, source := req.Repo, ""
	if known, ok := sessions.(sessionLaunchOptionsSource); ok {
		repo, err := known.ResolveSessionLaunchRepository(ctx, req.Repo)
		if err != nil {
			return SessionExecutionOutcome{}, fmt.Errorf("resolve launch repository: %w", err)
		}
		remote, source = repo.Remote, repo.Source
	}
	// Tags are presentational, for a reader inside hive, and are never read
	// back. ItemSessionLinker writes the associations this app queries.
	origins := uniqueKnownOrigins(req.Origins)
	var tags []string
	for _, origin := range origins {
		tags = append(tags, origin.ExternalID)
	}
	// One progress writer per attempt: hive's error names the operation that
	// failed but not the step, so without this a clone failure arrives as
	// "clone repository: git clone: exec git: exit status 1" and nothing else.
	progress := &sessionProgress{}
	s, err := sessions.CreateSession(ctx, hive.CreateOptions{Name: req.Name, Prompt: req.Prompt, Remote: remote, Source: source, AgentKey: req.Agent, Background: true, UseBatchSpawn: false, Tags: tags, Progress: progress})
	if err != nil {
		if errors.Is(err, session.ErrDuplicateName) {
			return SessionExecutionOutcome{}, fmt.Errorf("%w: %w", ErrDuplicateSessionName, err)
		}
		failure := &SessionCreateError{
			Name:   req.Name,
			Remote: remote,
			Step:   progress.LastLine(),
			Output: progress.Tail(),
			Err:    err,
		}
		// hive names the operation, the checkout and the strategy on its own
		// error for every step that runs after it resolved a destination. Its
		// Operation beats the derived progress line: it is the authority on
		// which step failed, not a guess at the last thing that printed.
		if created, ok := errors.AsType[*hive.CreateSessionError](err); ok {
			failure.Destination, failure.CloneStrategy = created.Destination, created.CloneStrategy
			failure.LeftoverCheckout = leftoverCheckout(created.Destination)
			if created.Operation != "" {
				failure.Step = created.Operation
			}
		}
		return SessionExecutionOutcome{}, failure
	}
	// The session exists either way, so a failed link is logged rather than
	// returned: reporting the launch as failed would be a lie, and would
	// invite a retry that creates a second session.
	if l.links != nil {
		for _, origin := range origins {
			if linkErr := l.links.Link(ctx, s.ID, origin); linkErr != nil {
				l.logger.Warn().Ctx(ctx).Err(linkErr).Str("session_id", s.ID).Str("external_id", origin.ExternalID).Msg("linking session to an inbox item")
			}
		}
	}
	if l.recorder != nil {
		name := req.Name
		if s != nil && s.Name != "" {
			name = s.Name
		}
		l.recorder.Record(ctx, activity.SessionCreated(name, req.Agent, req.Repo))
	}
	return SessionExecutionOutcome{ID: s.ID, Name: s.Name, Slug: s.Slug, Path: s.Path}, nil
}

func uniqueKnownOrigins(origins []models.ItemRef) []models.ItemRef {
	seen := make(map[models.ItemRef]struct{}, len(origins))
	var unique []models.ItemRef
	for _, origin := range origins {
		if !origin.Known() {
			continue
		}
		if _, exists := seen[origin]; exists {
			continue
		}
		seen[origin] = struct{}{}
		unique = append(unique, origin)
	}
	return unique
}

// SessionLaunchOptions exposes only labels, remotes, and configured agent keys
// to the desktop; local source paths remain in the Hive service.
func (l *HiveSessionLauncher) SessionLaunchOptions(ctx context.Context) (SessionLaunchOptions, error) {
	sessions := l.hive()
	known, ok := sessions.(sessionLaunchOptionsSource)
	if !ok {
		return SessionLaunchOptions{}, fmt.Errorf("session launch options are unavailable")
	}
	options, err := known.SessionLaunchOptions(ctx)
	if err != nil {
		return SessionLaunchOptions{}, err
	}
	view := SessionLaunchOptions{
		DefaultRepository: options.DefaultRepository,
		Agents:            options.Agents,
		DefaultAgent:      options.DefaultAgent,
	}
	for _, repo := range options.Repositories {
		view.Repositories = append(view.Repositories, SessionLaunchRepository{Name: repo.Name, Repository: repo.Remote})
	}
	return view, nil
}

// HiveSessionManager adapts Hive's session service to the read and lifecycle
// operations the desktop's session list drives. It is separate from
// HiveSessionLauncher because launching is a dispatch action and managing is
// not: the launcher is what an output command reaches for, and widening it
// would hand every action executor a delete.
type HiveSessionManager struct {
	// current holds everything derived from the Hive config, swapped whole on
	// Rebind. windows is outside it: the tmux control-client pool is this
	// app's own and a config reload must not disturb a live attach.
	current atomic.Pointer[managerHive]
	windows sessionWindowSource
}

type managerHive struct {
	sessions           SessionManagement
	statuses           sessionStatusSource
	git                sessionGit
	statusPollInterval time.Duration
}

// sessionGit is the read-only slice of the shared git executor a session's
// status needs. Narrowed rather than taking git.Git whole so the seam cannot
// grow a Checkout or a ResetHard: reporting status must not move a worktree.
type sessionGit interface {
	Branch(ctx context.Context, dir string) (string, error)
	IsClean(ctx context.Context, dir string) (bool, error)
	HasUnpushedCommits(ctx context.Context, dir string) (bool, error)
	DiffStats(ctx context.Context, dir string) (additions, deletions int, err error)
}

var _ sessionGit = git.Git(nil)

func NewHiveSessionManager(sessions SessionManagement, statuses sessionStatusSource, windows sessionWindowSource, gitExec sessionGit, statusPollInterval time.Duration) *HiveSessionManager {
	m := &HiveSessionManager{windows: windows}
	m.Rebind(sessions, statuses, gitExec, statusPollInterval)
	return m
}

// Rebind points the manager at services built from a freshly loaded Hive
// config. Every method takes one snapshot and works from it, so a reload
// mid-call cannot pair a session service with the git executor of another
// config.
func (m *HiveSessionManager) Rebind(sessions SessionManagement, statuses sessionStatusSource, gitExec sessionGit, statusPollInterval time.Duration) {
	m.current.Store(&managerHive{sessions: sessions, statuses: statuses, git: gitExec, statusPollInterval: statusPollInterval})
}

func (m *HiveSessionManager) hive() *managerHive { return m.current.Load() }

// ListSessions returns every session, recycled and corrupted included: an
// unattachable session still has to be manageable, which is the whole point of
// listing it.
func (m *HiveSessionManager) ListSessions(ctx context.Context) ([]SessionSummary, error) {
	h := m.hive()
	sessions, err := h.sessions.ListSessions(ctx)
	if err != nil {
		return nil, fmt.Errorf("list hive sessions: %w", err)
	}
	out := make([]SessionSummary, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, sessionSummaryOf(s))
	}
	return out, nil
}

// SessionStatuses projects Hive's terminal detection without exposing pane
// content or shared status types beyond this anti-corruption layer.
func (m *HiveSessionManager) SessionStatuses(ctx context.Context) (SessionStatusSnapshot, error) {
	h := m.hive()
	snapshot := SessionStatusSnapshot{
		Items:        []SessionStatus{},
		PollInterval: h.statusPollInterval,
	}
	if h.statuses == nil || !h.statuses.Available() {
		return snapshot, nil
	}

	sessions, err := h.sessions.ListSessions(ctx)
	if err != nil {
		return SessionStatusSnapshot{}, fmt.Errorf("list hive sessions for status: %w", err)
	}
	active := make([]*session.Session, 0, len(sessions))
	for i := range sessions {
		if sessions[i].State == session.StateActive {
			active = append(active, &sessions[i])
		}
	}
	statuses := h.statuses.FetchBatch(ctx, active, nil)
	windowSets, err := m.sessionWindows(ctx, active)
	if err != nil {
		return SessionStatusSnapshot{}, fmt.Errorf("list tmux windows for status: %w", err)
	}
	for _, s := range active {
		status, ok := statuses[s.ID]
		if !ok {
			continue
		}
		refs := windowSets[hive.SessionTarget(*s).Session]
		item := SessionStatus{SessionID: s.ID, Running: len(refs) > 0, Windows: []SessionWindowStatus{}}
		if m.windows == nil {
			item.Running = status.Status != terminal.StatusMissing
		}
		if len(status.Windows) == 0 {
			if windowID := stableWindowID("", status.WindowName, refs); windowID != "" {
				item.Windows = append(item.Windows, SessionWindowStatus{
					WindowID: windowID,
					Status:   desktopAgentStatus(status.Status),
					Tool:     status.Tool,
				})
			}
		}
		for _, window := range status.Windows {
			windowID := stableWindowID(window.WindowIndex, window.WindowName, refs)
			if windowID == "" {
				continue
			}
			item.Windows = append(item.Windows, SessionWindowStatus{
				WindowID: windowID,
				Status:   desktopAgentStatus(window.Status),
				Tool:     window.Tool,
			})
		}
		snapshot.Items = append(snapshot.Items, item)
	}
	return snapshot, nil
}

func (m *HiveSessionManager) sessionWindows(ctx context.Context, sessions []*session.Session) (map[string][]SessionWindowRef, error) {
	if m.windows == nil {
		return nil, nil
	}
	targets := make([]string, 0, len(sessions))
	for _, s := range sessions {
		targets = append(targets, hive.SessionTarget(*s).Session)
	}
	return m.windows.ListSessionWindows(ctx, targets)
}

func stableWindowID(index, name string, refs []SessionWindowRef) string {
	for _, ref := range refs {
		if index != "" && ref.Index == index && (name == "" || ref.Name == name) {
			return ref.ID
		}
	}
	if name != "" {
		match := ""
		for _, ref := range refs {
			if ref.Name != name {
				continue
			}
			if match != "" {
				return ""
			}
			match = ref.ID
		}
		if match != "" {
			return match
		}
	}
	if index == "" && len(refs) == 1 {
		return refs[0].ID
	}
	return ""
}

func desktopAgentStatus(status terminal.Status) string {
	if status == terminal.StatusQuestion {
		return string(terminal.StatusApproval)
	}
	return string(status)
}

// RunningSessions reports which of ids currently have a live tmux session. It
// is the narrow counterpart to SessionStatuses: an inbox item asks about the
// one or two sessions it spawned, and a full sweep would pay a tmux round trip
// for every active session in the install to answer that.
//
// An unavailable status source is data, not a failure — nothing is reported
// running, which is what "we cannot see tmux from here" honestly looks like.
func (m *HiveSessionManager) RunningSessions(ctx context.Context, ids []string) (map[string]bool, error) {
	h := m.hive()
	running := map[string]bool{}
	if len(ids) == 0 || h.statuses == nil || !h.statuses.Available() {
		return running, nil
	}
	wanted := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		wanted[id] = struct{}{}
	}
	sessions, err := h.sessions.ListSessions(ctx)
	if err != nil {
		return nil, fmt.Errorf("list hive sessions for status: %w", err)
	}
	subset := make([]*session.Session, 0, len(ids))
	for i := range sessions {
		if _, ok := wanted[sessions[i].ID]; !ok || sessions[i].State != session.StateActive {
			continue
		}
		subset = append(subset, &sessions[i])
	}
	windowSets, err := m.sessionWindows(ctx, subset)
	if err != nil {
		return nil, fmt.Errorf("list tmux windows for liveness: %w", err)
	}
	if m.windows != nil {
		for _, s := range subset {
			if len(windowSets[hive.SessionTarget(*s).Session]) > 0 {
				running[s.ID] = true
			}
		}
		return running, nil
	}
	for id, status := range h.statuses.FetchBatch(ctx, subset, nil) {
		running[id] = status.Status != terminal.StatusMissing
	}
	return running, nil
}

func (m *HiveSessionManager) SessionDetail(ctx context.Context, id string) (SessionDetail, error) {
	h := m.hive()
	s, err := h.sessions.GetSession(ctx, id)
	if err != nil {
		return SessionDetail{}, fmt.Errorf("get hive session: %w", err)
	}
	return SessionDetail{
		ID:             s.ID,
		Name:           s.Name,
		Slug:           s.Slug,
		Repo:           s.Remote,
		State:          string(s.State),
		Path:           s.Path,
		CloneStrategy:  s.CloneStrategy,
		WorktreeBranch: s.GetMeta(session.MetaWorktreeBranch),
		TmuxSession:    hive.SessionTarget(s).Session,
		Tags:           s.Tags,
		CreatedAt:      s.CreatedAt,
		UpdatedAt:      s.UpdatedAt,
	}, nil
}

// SessionRisk reports what a delete or recycle of id would cost. Hive reports
// no risk for a non-active session, which is correct — there is no live clone
// left to hold unsaved work.
func (m *HiveSessionManager) SessionRisk(ctx context.Context, id string) (SessionRisk, error) {
	h := m.hive()
	s, err := h.sessions.GetSession(ctx, id)
	if err != nil {
		return SessionRisk{}, fmt.Errorf("get hive session: %w", err)
	}
	risk, err := h.sessions.CheckSessionRisk(ctx, id)
	if err != nil {
		return SessionRisk{}, fmt.Errorf("check hive session risk: %w", err)
	}
	return SessionRisk{
		UncommittedChanges: risk.UncommittedChanges,
		UnpushedCommits:    risk.UnpushedCommits,
		RecycleDeletes:     s.CloneStrategy == session.CloneStrategyWorktree,
	}, nil
}

// SessionGitStatus reads one session's checkout: its branch, whether it is
// dirty, whether it has commits the remote does not, and the line delta
// against the default branch.
//
// Unlike SessionRisk it reports failures instead of assuming the risky answer.
// Hive treats an IsClean error as dirty because a delete confirmation must
// over-warn; a badge claiming "dirty" on a git that never ran is a lie the
// user acts on.
func (m *HiveSessionManager) SessionGitStatus(ctx context.Context, id string) (SessionGitStatus, error) {
	h := m.hive()
	s, err := h.sessions.GetSession(ctx, id)
	if err != nil {
		return SessionGitStatus{}, fmt.Errorf("get hive session: %w", err)
	}
	if s.State != session.StateActive || s.Path == "" {
		return SessionGitStatus{}, nil
	}
	if h.git == nil {
		return SessionGitStatus{Error: "git is unavailable"}, nil
	}

	host, owner, repo := remoteCoordinates(s.Remote)
	status := SessionGitStatus{Path: s.Path, Host: host, Owner: owner, Repo: repo}

	branch, err := h.git.Branch(ctx, s.Path)
	if err != nil {
		// Every other read needs the working checkout Branch proves, so its
		// failure stands for the whole status.
		return SessionGitStatus{Path: s.Path, Host: host, Owner: owner, Repo: repo, Error: err.Error()}, nil
	}
	status.Branch = branch
	status.Resolved = true

	if clean, err := h.git.IsClean(ctx, s.Path); err == nil {
		status.Dirty = !clean
	} else {
		status.Error = err.Error()
	}
	if unpushed, err := h.git.HasUnpushedCommits(ctx, s.Path); err == nil {
		status.Unpushed = unpushed
	} else if status.Error == "" {
		// A worktree with no upstream reaches here on every read, so it must
		// not displace a real error above.
		status.Error = err.Error()
	}
	if additions, deletions, err := h.git.DiffStats(ctx, s.Path); err == nil {
		status.Additions, status.Deletions = additions, deletions
	} else if status.Error == "" {
		status.Error = err.Error()
	}
	return status, nil
}

// remoteCoordinates reads the host, owner and repo off a remote. Which forge
// serves that host is not decided here: the app layer asks the connected
// accounts, and a host none of them serves is what makes a pull-request lookup
// unsupported.
//
// A remote naming no host — a local path — carries no coordinates either, since
// git.ExtractOwnerRepo is host-agnostic and would read the last two path
// segments of anything.
func remoteCoordinates(remote string) (host, owner, repo string) {
	// Lowered here so the forge match and the pull-request cache key are both
	// canonical: a remote may be written with any casing in its host.
	host = strings.ToLower(git.ExtractHost(remote))
	if host == "" {
		return "", "", ""
	}
	owner, repo = git.ExtractOwnerRepo(remote)
	return host, owner, repo
}

// SpawnTmuxSession creates the tmux session for a session hive already holds,
// from hive's own spawn configuration for the remote — the windows, working
// directory and commands hive would have used itself. A session tmux already
// has is left alone, so this is idempotent.
//
// It spawns detached: the desktop attaches over control mode, and an attaching
// spawn would hand the session to whatever terminal launched the app — or fail
// for a launcher that has none.
func (m *HiveSessionManager) SpawnTmuxSession(ctx context.Context, name, path, repo string) error {
	h := m.hive()
	if err := h.sessions.OpenTmuxSession(ctx, name, path, repo, "", true); err != nil {
		return fmt.Errorf("open hive tmux session: %w", err)
	}
	return nil
}

func (m *HiveSessionManager) RenameSession(ctx context.Context, id, name string) error {
	h := m.hive()
	if err := h.sessions.RenameSession(ctx, id, name); err != nil {
		return fmt.Errorf("rename hive session: %w", err)
	}
	return nil
}

func (m *HiveSessionManager) DeleteSession(ctx context.Context, id string) error {
	h := m.hive()
	if err := h.sessions.DeleteSession(ctx, id); err != nil {
		return fmt.Errorf("delete hive session: %w", err)
	}
	return nil
}

// RecycleSession discards the recycle commands' output. The commands are the
// user's own (hive's recycle_commands), and the job records whether they
// succeeded; streaming their stdout would need a job log to stream into.
func (m *HiveSessionManager) RecycleSession(ctx context.Context, id string) error {
	h := m.hive()
	if err := h.sessions.RecycleSession(ctx, id, io.Discard); err != nil {
		return fmt.Errorf("recycle hive session: %w", err)
	}
	return nil
}

// PruneSessions deletes every recycled and corrupted session. Hive's Prune also
// has a mode that only trims each pool back to max_recycled; that is a config
// reconciliation, not something a menu entry can honestly name, so the desktop
// exposes the unambiguous one.
func (m *HiveSessionManager) PruneSessions(ctx context.Context) (int, error) {
	h := m.hive()
	count, err := h.sessions.Prune(ctx, true)
	if err != nil {
		return count, fmt.Errorf("prune hive sessions: %w", err)
	}
	return count, nil
}

func sessionSummaryOf(s session.Session) SessionSummary {
	return SessionSummary{
		ID:          s.ID,
		Name:        s.Name,
		Slug:        s.Slug,
		Repo:        s.Remote,
		State:       string(s.State),
		TmuxSession: hive.SessionTarget(s).Session,
	}
}

// SlugifySessionName converts a display name to the slug Hive uses for
// tmux session names and directory paths. It wraps the shared
// session.Slugify so that launch_session_executor.go — not itself an ACL
// seam — never imports internal/core/session directly; a rename there
// breaks this one file instead of spreading to a non-seam caller.
func SlugifySessionName(name string) string {
	return session.Slugify(name)
}

// ValidateSessionName validates name against Hive's session naming rules.
// See SlugifySessionName for why this wraps session.ValidateName
// instead of letting callers import internal/core/session directly.
func ValidateSessionName(name string) error {
	return session.ValidateName(name)
}

type DurableMessageService interface {
	Publish(context.Context, messaging.Message, []string) (messaging.PublishResult, error)
}
type HiveMessagePublisher struct{ current atomic.Pointer[publisherHive] }

// publisherHive carries the configured retention cap, so a reload is what
// makes a changed messaging.max_messages take effect.
type publisherHive struct{ messages DurableMessageService }

func NewHiveMessagePublisher(messages DurableMessageService) *HiveMessagePublisher {
	p := &HiveMessagePublisher{}
	p.Rebind(messages)
	return p
}

// Rebind points the publisher at a message service built from a freshly
// loaded Hive config.
func (p *HiveMessagePublisher) Rebind(messages DurableMessageService) {
	p.current.Store(&publisherHive{messages: messages})
}

func (p *HiveMessagePublisher) hive() DurableMessageService { return p.current.Load().messages }

func (p *HiveMessagePublisher) PublishMessage(ctx context.Context, payload, topic string) (string, error) {
	messages := p.hive()
	if messages == nil {
		return "", fmt.Errorf("publish message: hive message service is unavailable")
	}
	result, err := messages.Publish(ctx, messaging.Message{Payload: payload, Sender: "hive-desktop", SessionID: ""}, []string{topic})
	if err != nil {
		return "", fmt.Errorf("publish message: %w", err)
	}
	if len(result.Topics) != 1 || result.Topics[0] != topic {
		return "", fmt.Errorf("publish message: expected exact topic %q, got %v", topic, result.Topics)
	}
	return result.Topics[0], nil
}
