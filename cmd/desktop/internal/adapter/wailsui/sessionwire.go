package wailsui

import (
	"strings"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app"

	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/domain/session"
	drivesvc "github.com/colonyops/hive/internal/hive/drive"
	"github.com/colonyops/hive/internal/hive/gitstatus"
	"github.com/colonyops/hive/internal/platform/git"
)

// SessionSummary is the session row shown in Code. CreatedAt starts with the
// current logical session even when Hive reuses an older clone.
//
// A session on a server has Server set, and its ID and Slug carry the
// "<server>:" prefix, so every id-keyed call and every terminal call reaches
// that server. The local rows leave Server empty.
type SessionSummary struct {
	Server       string    `json:"server"`
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Slug         string    `json:"slug"`
	Repo         string    `json:"repo"`
	RepoIdentity string    `json:"repoIdentity"`
	State        string    `json:"state"`
	CanvasOwner  string    `json:"canvasOwner"`
	Tags         []string  `json:"tags"`
	CreatedAt    time.Time `json:"createdAt"`
}

func sessionSummaryOf(canonical func(drive.Space) drive.Space, s session.Session) SessionSummary {
	space, _ := drivesvc.SpaceForRemote(s.Remote)
	if space.Key != "" {
		space = canonical(space)
	}
	return SessionSummary{
		ID: s.ID, Name: s.Name, Slug: s.Slug, Repo: s.Remote, RepoIdentity: git.RemoteIdentity(s.Remote), State: string(s.State),
		CanvasOwner: space.Key, Tags: tagsOf(s), CreatedAt: s.ActivatedAt(),
	}
}

// serverSummaryOf is s as a row of server. Its repository owns no canvas
// here: the drive is on this machine.
func serverSummaryOf(server string, s session.Session) SessionSummary {
	return SessionSummary{
		Server: server, ID: server + ":" + s.ID, Name: s.Name, Slug: server + ":" + s.Slug,
		Repo: s.Remote, RepoIdentity: git.RemoteIdentity(s.Remote), State: string(s.State),
		Tags: tagsOf(s), CreatedAt: s.ActivatedAt(),
	}
}

// ServerSessions is one server's rows for the Code view.
type ServerSessions struct {
	Server   string           `json:"server"`
	SSH      string           `json:"ssh"`
	State    string           `json:"state"`
	Error    string           `json:"error"`
	Sessions []SessionSummary `json:"sessions"`
}

func serverSessionsOf(lists []app.ServerSessionList) []ServerSessions {
	out := make([]ServerSessions, 0, len(lists))
	for _, list := range lists {
		rows := make([]SessionSummary, 0, len(list.Sessions))
		for _, s := range list.Sessions {
			rows = append(rows, serverSummaryOf(list.Server, s))
		}
		out = append(out, ServerSessions{Server: list.Server, SSH: list.SSH, State: string(list.State), Error: list.Error, Sessions: rows})
	}
	return out
}

// tagsOf keeps the wire field an array, so the frontend never sees null.
func tagsOf(s session.Session) []string {
	if s.Tags == nil {
		return []string{}
	}
	return s.Tags
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
	Server         string    `json:"server"`
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

// sessionDetailOf is s as read through id, which keeps the "<server>:" prefix
// of a session on a server.
func sessionDetailOf(id string, s session.Session) SessionDetail {
	server := serverOf(id)
	return SessionDetail{
		Server:         server,
		ID:             qualified(server, s.ID),
		Name:           s.Name,
		Slug:           qualified(server, s.Slug),
		Repo:           s.Remote,
		State:          string(s.State),
		Path:           s.Path,
		CloneStrategy:  s.CloneStrategy,
		WorktreeBranch: s.GetMeta(session.MetaWorktreeBranch),
		Tags:           tagsOf(s),
		CreatedAt:      s.CreatedAt,
		UpdatedAt:      s.UpdatedAt,
	}
}

// serverOf is the server an id from the Code view names, "" for a local id.
func serverOf(id string) string {
	server, _, found := strings.Cut(id, ":")
	if !found {
		return ""
	}
	return server
}

func qualified(server, value string) string {
	if server == "" {
		return value
	}
	return server + ":" + value
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
