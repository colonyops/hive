// Package projectapi is the project session view Hive Desktop shares between
// this machine and a server. Local runs it on an in-process engine; the
// daemon client runs it over RPC.
package projectapi

import (
	"context"
	"time"

	projectsvc "github.com/colonyops/hive/internal/hive/project"
)

// Repo is one repository of a project session. Missing is a repository
// whose hive session is gone.
type Repo struct {
	Dir           string
	SessionID     string
	Remote        string
	Path          string
	Branch        string
	CloneStrategy string
	Missing       bool
}

type Project struct {
	ID   string
	Name string
	// Slug is the tmux session the agent runs in, at Root.
	Slug string
	// Definition names the projects: entry the project came from, if any.
	Definition string
	Root       string
	Repos      []Repo
	CreatedAt  time.Time
}

// Definition is a project from the projects: section of config, with each
// repository's folder and clone strategy resolved.
type Definition struct {
	Name  string
	Agent string
	Repos []projectsvc.RepoSpec
}

// RepoRisk is what a recycle or delete of one repository would discard.
type RepoRisk struct {
	Dir                string
	CloneStrategy      string
	Missing            bool
	UncommittedChanges bool
	UnpushedCommits    bool
}

// API creates every project in the background: the implementation may run
// in a daemon with no terminal.
type API interface {
	ListProjects(ctx context.Context) ([]Project, error)
	ProjectDefinitions(ctx context.Context) ([]Definition, error)
	DefaultCloneStrategy(ctx context.Context, remote string) (string, error)
	// CreateProject reports each step to opts.OnStep when set.
	CreateProject(ctx context.Context, opts projectsvc.CreateOptions) (Project, error)
	ProjectRisk(ctx context.Context, ref string) ([]RepoRisk, error)
	RecycleProject(ctx context.Context, ref string) error
	DeleteProject(ctx context.Context, ref string) error
	// OpenProject starts the project's tmux session in the background, and
	// its agent when no tmux session runs.
	OpenProject(ctx context.Context, ref string) error
	// ProjectOf returns the slug of the project that holds the hive
	// session, and false when it is in none.
	ProjectOf(ctx context.Context, sessionID string) (string, bool, error)
}
