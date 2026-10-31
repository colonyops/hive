package wailsui

import (
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
		Server: server, ID: session.Ref{Server: server, ID: s.ID}.String(), Name: s.Name, Slug: session.Ref{Server: server, ID: s.Slug}.String(),
		Repo: s.Remote, RepoIdentity: git.RemoteIdentity(s.Remote), State: string(s.State),
		Tags: tagsOf(s), CreatedAt: s.ActivatedAt(),
	}
}

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
	ref := refOf(id)
	return SessionDetail{
		Server:         serverOf(id),
		ID:             session.Ref{Server: ref.Server, ID: s.ID}.String(),
		Name:           s.Name,
		Slug:           session.Ref{Server: ref.Server, ID: s.Slug}.String(),
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

// refOf reads an id the service has already accepted, so a malformed one is
// only ever a bare local id.
func refOf(id string) session.Ref {
	ref, err := session.ParseRef(id, session.LocalServer)
	if err != nil {
		return session.Ref{Server: session.LocalServer, ID: id}
	}
	return ref
}

// serverOf is the server an id from the Code view names, "" for this machine.
func serverOf(id string) string {
	if ref := refOf(id); ref.Server != session.LocalServer {
		return ref.Server
	}
	return ""
}

// SessionGitStatus is one session's checkout as the session status bar reads
// it. Resolved separates "git answered" from the zero value; Error carries why
// a read failed and is never a substitute for it.
type SessionGitStatus struct {
	Path     string `json:"path"`
	Branch   string `json:"branch"`
	Dirty    bool   `json:"dirty"`
	Unpushed bool   `json:"unpushed"`
	// Additions and Deletions are the working tree against the merge base with
	// the default branch.
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

// ChangedFile is one file in the review view. Stamp changes when the file on
// disk does, so a held diff can be read again.
type ChangedFile struct {
	Path      string `json:"path"`
	OldPath   string `json:"oldPath"`
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Binary    bool   `json:"binary"`
	Stamp     string `json:"stamp"`
}

// SessionChanges is what the review view lists. Base is the base the read used,
// which is "uncommitted" whenever BranchAvailable is false.
// DiffTarget says what a read compares. See gitstatus.DiffTarget.
type DiffTarget struct {
	Base    string `json:"base"`
	Against string `json:"against"`
	Commit  string `json:"commit"`
}

func (t DiffTarget) gitstatus() gitstatus.DiffTarget {
	return gitstatus.DiffTarget{Base: gitstatus.DiffBase(t.Base), Against: t.Against, Commit: t.Commit}
}

type SessionCommit struct {
	ID      string `json:"id"`
	Subject string `json:"subject"`
	Author  string `json:"author"`
	Unix    int64  `json:"unix"`
}

type SessionChanges struct {
	Resolved        bool            `json:"resolved"`
	Branch          string          `json:"branch"`
	DefaultBranch   string          `json:"defaultBranch"`
	Base            string          `json:"base"`
	BranchAvailable bool            `json:"branchAvailable"`
	Against         string          `json:"against"`
	Commit          string          `json:"commit"`
	Commits         []SessionCommit `json:"commits"`
	Files           []ChangedFile   `json:"files"`
	Truncated       bool            `json:"truncated"`
}

func sessionChangesOf(c gitstatus.Changes) SessionChanges {
	files := make([]ChangedFile, 0, len(c.Files))
	for _, f := range c.Files {
		files = append(files, ChangedFile{
			Path:      f.Path,
			OldPath:   f.OldPath,
			Status:    f.Status.String(),
			Additions: f.Additions,
			Deletions: f.Deletions,
			Binary:    f.Binary,
			Stamp:     f.Stamp,
		})
	}
	commits := make([]SessionCommit, 0, len(c.Commits))
	for _, commit := range c.Commits {
		commits = append(commits, SessionCommit{ID: commit.ID, Subject: commit.Subject, Author: commit.Author, Unix: commit.Unix})
	}
	return SessionChanges{
		Resolved:        c.Resolved,
		Branch:          c.Branch,
		DefaultBranch:   c.DefaultBranch,
		Base:            c.Base.String(),
		BranchAvailable: c.BranchAvailable,
		Against:         c.Against,
		Commit:          c.Commit,
		Commits:         commits,
		Files:           files,
		Truncated:       c.Truncated,
	}
}

// SessionFileDiff is one file's diff. At most one state field is set, and with
// none set Text is the unified diff.
type SessionFileDiff struct {
	Path        string `json:"path"`
	Text        string `json:"text"`
	Binary      bool   `json:"binary"`
	Truncated   bool   `json:"truncated"`
	Unavailable bool   `json:"unavailable"`
	Gone        bool   `json:"gone"`
	Error       string `json:"error"`
}

func sessionFileDiffsOf(diffs []gitstatus.FileDiff) []SessionFileDiff {
	out := make([]SessionFileDiff, 0, len(diffs))
	for _, d := range diffs {
		out = append(out, SessionFileDiff(d))
	}
	return out
}
