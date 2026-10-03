package wailsui

import (
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive/gitstatus"
)

// SessionSummary is one session as the session list sees it.
type SessionSummary struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Slug  string `json:"slug"`
	Repo  string `json:"repo"`
	State string `json:"state"`
}

func sessionSummaryOf(s session.Session) SessionSummary {
	return SessionSummary{ID: s.ID, Name: s.Name, Slug: s.Slug, Repo: s.Remote, State: string(s.State)}
}

func sessionSummariesOf(sessions []session.Session) []SessionSummary {
	out := make([]SessionSummary, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, sessionSummaryOf(s))
	}
	return out
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
	Tags           []string  `json:"tags"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

func sessionDetailOf(s session.Session) SessionDetail {
	return SessionDetail{
		ID:             s.ID,
		Name:           s.Name,
		Slug:           s.Slug,
		Repo:           s.Remote,
		State:          string(s.State),
		Path:           s.Path,
		CloneStrategy:  s.CloneStrategy,
		WorktreeBranch: s.GetMeta(session.MetaWorktreeBranch),
		Tags:           s.Tags,
		CreatedAt:      s.CreatedAt,
		UpdatedAt:      s.UpdatedAt,
	}
}

// SessionRisk is the pre-flight a destructive operation confirms against: what
// unsaved work the session holds, and whether recycling it is really a delete.
type SessionRisk struct {
	UncommittedChanges bool `json:"uncommittedChanges"`
	UnpushedCommits    bool `json:"unpushedCommits"`
	// RecycleDeletes reports that recycling this session destroys it: hive
	// routes a worktree session's recycle straight to a delete, because a
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
	// remote that names no host, such as a local path.
	Host     string `json:"host"`
	Owner    string `json:"owner"`
	Repo     string `json:"repo"`
	Resolved bool   `json:"resolved"`
	Error    string `json:"error"`
}

func sessionGitStatusOf(s gitstatus.Status) SessionGitStatus {
	status := SessionGitStatus{
		Path:      s.Path,
		Branch:    s.Branch,
		Dirty:     s.Dirty,
		Unpushed:  s.Unpushed,
		Additions: s.Additions,
		Deletions: s.Deletions,
		Host:      s.Host,
		Owner:     s.Owner,
		Repo:      s.Repo,
		Resolved:  s.Resolved,
	}
	if err := s.Error(); err != nil {
		status.Error = err.Error()
	}
	return status
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

// SessionStatusSnapshot is one poll result in the units the browser timer uses.
type SessionStatusSnapshot struct {
	Items          []SessionStatus `json:"items"`
	PollIntervalMS int64           `json:"pollIntervalMs"`
}

func sessionStatusSnapshotOf(snapshot app.SessionStatusSnapshot) SessionStatusSnapshot {
	items := make([]SessionStatus, 0, len(snapshot.Items))
	for _, item := range snapshot.Items {
		windows := make([]SessionWindowStatus, 0, len(item.Windows))
		for _, w := range item.Windows {
			windows = append(windows, SessionWindowStatus{WindowID: w.WindowID, Status: string(w.Status), Tool: w.Tool})
		}
		items = append(items, SessionStatus{SessionID: item.SessionID, Running: item.Running, Windows: windows})
	}
	return SessionStatusSnapshot{Items: items, PollIntervalMS: snapshot.PollInterval.Milliseconds()}
}

// ItemSessionView is one hive session an inbox item spawned. Only CreatedAt
// comes from the link. Everything else is read live from hive, so a session
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

func itemSessionViewsOf(views []app.ItemSessionView) []ItemSessionView {
	out := make([]ItemSessionView, 0, len(views))
	for _, v := range views {
		out = append(out, ItemSessionView{
			ID: v.ID, Name: v.Name, Slug: v.Slug, Repo: v.Repo,
			State: string(v.State), Running: v.Running, CreatedAt: v.CreatedAt,
		})
	}
	return out
}

// ItemChatView is an agent workspace chat an inbox item opened.
type ItemChatView struct {
	ID        int64     `json:"id"`
	Workspace string    `json:"workspace"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

func itemChatViewsOf(views []app.ItemChatView) []ItemChatView {
	out := make([]ItemChatView, 0, len(views))
	for _, v := range views {
		out = append(out, ItemChatView{ID: v.ID, Workspace: v.Workspace, Name: v.Name, CreatedAt: v.CreatedAt})
	}
	return out
}

// SessionPullRequestKey addresses the pull request a session's branch has.
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
	PullRequestStatusNone         PullRequestStatus = PullRequestStatus(app.PullRequestStatusNone)
	PullRequestStatusFound        PullRequestStatus = PullRequestStatus(app.PullRequestStatusFound)
	PullRequestStatusDisconnected PullRequestStatus = PullRequestStatus(app.PullRequestStatusDisconnected)
	PullRequestStatusUnsupported  PullRequestStatus = PullRequestStatus(app.PullRequestStatusUnsupported)
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
	// ReviewDecision is GitHub's own vocabulary (APPROVED, CHANGES_REQUESTED,
	// REVIEW_REQUIRED), or empty when review is not required.
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

func sessionPullRequestOf(pr app.SessionPullRequest) SessionPullRequest {
	return SessionPullRequest{
		Status:         PullRequestStatus(pr.Status),
		Number:         pr.Number,
		Title:          pr.Title,
		State:          pr.State,
		IsDraft:        pr.IsDraft,
		URL:            pr.URL,
		ReviewDecision: pr.ReviewDecision,
		Checks:         pr.Checks,
		Additions:      pr.Additions,
		Deletions:      pr.Deletions,
		Cached:         pr.Cached,
	}
}

// SessionLaunchRepository is the safe presentation of a repository a session
// can start in. Local checkout paths stay backend-only.
type SessionLaunchRepository struct {
	Name       string `json:"name"`
	Repository string `json:"repository"`
}

// SessionLaunchWorkspace identifies a configured workspace by its stable
// directory name.
type SessionLaunchWorkspace struct {
	Dir            string `json:"dir"`
	Name           string `json:"name"`
	SupportsPrompt bool   `json:"supportsPrompt"`
}

func sessionLaunchWorkspacesOf(workspaces []dispatch.SessionLaunchWorkspace) []SessionLaunchWorkspace {
	if workspaces == nil {
		return nil
	}
	out := make([]SessionLaunchWorkspace, 0, len(workspaces))
	for _, w := range workspaces {
		out = append(out, SessionLaunchWorkspace{Dir: w.Dir, Name: w.Name, SupportsPrompt: w.SupportsPrompt})
	}
	return out
}

// SessionLaunchOptions is what the New Session form offers.
type SessionLaunchOptions struct {
	Repositories      []SessionLaunchRepository `json:"repositories"`
	DefaultRepository string                    `json:"defaultRepository"`
	Workspaces        []SessionLaunchWorkspace  `json:"workspaces"`
	Agents            []string                  `json:"agents"`
	DefaultAgent      string                    `json:"defaultAgent"`
}

func sessionLaunchOptionsOf(opts dispatch.SessionLaunchOptions) SessionLaunchOptions {
	view := SessionLaunchOptions{
		DefaultRepository: opts.DefaultRepository,
		Workspaces:        sessionLaunchWorkspacesOf(opts.Workspaces),
		Agents:            opts.Agents,
		DefaultAgent:      opts.DefaultAgent,
	}
	for _, repo := range opts.Repositories {
		view.Repositories = append(view.Repositories, SessionLaunchRepository{Name: repo.Name, Repository: repo.Repository})
	}
	return view
}

// CreateSessionRequest is a user-submitted New Session form. ItemIDs are the
// inbox items the form was drafted from; the core resolves their identities
// and never takes item refs from a client.
type CreateSessionRequest struct {
	Repository string  `json:"repository,omitempty"`
	Workspace  string  `json:"workspace,omitempty"`
	Name       string  `json:"name"`
	Prompt     string  `json:"prompt"`
	Agent      string  `json:"agent,omitempty"`
	ItemIDs    []int64 `json:"itemIds,omitempty"`
}

func (r CreateSessionRequest) core() dispatch.CreateSessionRequest {
	return dispatch.CreateSessionRequest{
		Repository: r.Repository,
		Workspace:  r.Workspace,
		Name:       r.Name,
		Prompt:     r.Prompt,
		Agent:      r.Agent,
		ItemIDs:    r.ItemIDs,
	}
}

// SessionDraft is a New Session form the app prefills: from inbox items, or
// from a creation attempt that failed and is being handed back.
type SessionDraft struct {
	Repository string  `json:"repository,omitempty"`
	Workspace  string  `json:"workspace,omitempty"`
	Name       string  `json:"name"`
	Prompt     string  `json:"prompt"`
	Agent      string  `json:"agent,omitempty"`
	ItemIDs    []int64 `json:"itemIds,omitempty"`
	// Failure is null on a draft that is not a retry, which is also what "no
	// attempt is waiting" looks like.
	Failure *SessionCreateFailure `json:"failure,omitempty"`
}

// SessionCreateFailure is why one session creation attempt failed.
type SessionCreateFailure struct {
	// Reason is the wrapped error, a chain like
	// "clone repository: git clone: exec git: exit status 1".
	Reason string `json:"reason"`
	// Step is the last thing creation reported: "Cloning repository...",
	// "Executing rules...".
	Step string `json:"step"`
	// Output is the tail of the attempt's progress output, hook output included.
	Output string `json:"output"`
	// CloneStrategy is "full" or "worktree".
	CloneStrategy string `json:"cloneStrategy"`
	// Destination is the checkout hive resolved for the attempt.
	Destination string `json:"destination"`
	// LeftoverCheckout reports that Destination survived the failure, which is
	// what makes it a directory to go and delete.
	LeftoverCheckout bool      `json:"leftoverCheckout"`
	At               time.Time `json:"at"`
}

func sessionDraftOf(d dispatch.SessionDraft) SessionDraft {
	draft := SessionDraft{
		Repository: d.Repository,
		Workspace:  d.Workspace,
		Name:       d.Name,
		Prompt:     d.Prompt,
		Agent:      d.Agent,
		ItemIDs:    d.ItemIDs,
	}
	if f := d.Failure; f != nil {
		draft.Failure = &SessionCreateFailure{
			Reason:           f.Reason,
			Step:             f.Step,
			Output:           f.Output,
			CloneStrategy:    f.CloneStrategy,
			Destination:      f.Destination,
			LeftoverCheckout: f.LeftoverCheckout,
			At:               f.At,
		}
	}
	return draft
}

// TerminalTarget identifies the terminal session, and optionally the window
// inside it, an action was invoked from. It carries identity only: every value
// a template can read is resolved from the session record at invocation time,
// so a client cannot hand an executor a checkout path of its choosing.
type TerminalTarget struct {
	Slug string `json:"slug"`
	// WindowID is set when the invocation came from a window row. It is the
	// tmux window id, so it addresses the window across renames.
	WindowID string `json:"windowId,omitempty"`
}

func (t TerminalTarget) core() dispatch.TerminalTarget {
	return dispatch.TerminalTarget{Slug: t.Slug, WindowID: t.WindowID}
}

func sessionRiskOf(r app.SessionRisk) SessionRisk {
	return SessionRisk{UncommittedChanges: r.UncommittedChanges, UnpushedCommits: r.UnpushedCommits, RecycleDeletes: r.RecycleDeletes}
}
