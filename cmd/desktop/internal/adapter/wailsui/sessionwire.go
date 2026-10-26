package wailsui

import (
	"time"

	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/domain/session"
	drivesvc "github.com/colonyops/hive/internal/hive/drive"
	"github.com/colonyops/hive/internal/hive/gitstatus"
	"github.com/colonyops/hive/internal/platform/git"
)

// SessionSummary is the session row shown in Code. CreatedAt starts with the
// current logical session even when Hive reuses an older clone record.
type SessionSummary struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Slug         string    `json:"slug"`
	Repo         string    `json:"repo"`
	RepoIdentity string    `json:"repoIdentity"`
	State        string    `json:"state"`
	CanvasOwner  string    `json:"canvasOwner"`
	CreatedAt    time.Time `json:"createdAt"`
}

func sessionSummaryOf(canonical func(drive.Space) drive.Space, s session.Session) SessionSummary {
	space, _ := drivesvc.SpaceForRemote(s.Remote)
	if space.Key != "" {
		space = canonical(space)
	}
	return SessionSummary{
		ID: s.ID, Name: s.Name, Slug: s.Slug, Repo: s.Remote, RepoIdentity: git.RemoteIdentity(s.Remote), State: string(s.State),
		CanvasOwner: space.Key, CreatedAt: s.ActivatedAt(),
	}
}

func sessionSummariesOf(canonical func(drive.Space) drive.Space, sessions []session.Session) []SessionSummary {
	out := make([]SessionSummary, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, sessionSummaryOf(canonical, s))
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
	Labels         []string  `json:"labels"`
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
		Labels:         s.Labels,
		CreatedAt:      s.CreatedAt,
		UpdatedAt:      s.UpdatedAt,
	}
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
