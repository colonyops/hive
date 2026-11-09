package projectapi

import (
	"context"
	"io"

	"github.com/colonyops/hive/internal/domain/project"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive"
	projectsvc "github.com/colonyops/hive/internal/hive/project"
	"github.com/colonyops/hive/internal/platform/git"
)

// Local reads the engine's services on every call, so it follows a Reload.
type Local struct {
	engine *hive.Engine
}

var _ API = (*Local)(nil)

func NewLocal(engine *hive.Engine) *Local {
	return &Local{engine: engine}
}

func (l *Local) ListProjects(ctx context.Context) ([]Project, error) {
	projects := l.engine.Projects()
	all, err := projects.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Project, 0, len(all))
	for _, ps := range all {
		children, err := projects.Children(ctx, ps)
		if err != nil {
			return nil, err
		}
		out = append(out, projectOf(ps, children))
	}
	return out, nil
}

func projectOf(ps project.Session, children []projectsvc.Child) Project {
	repos := make([]Repo, 0, len(children))
	for i, c := range children {
		r := Repo{Dir: c.Dir, SessionID: ps.Repos[i].SessionID, Branch: c.Branch}
		if c.Session.ID == "" {
			r.Missing = true
		} else {
			r.Remote = c.Session.Remote
			r.Path = c.Session.Path
			r.CloneStrategy = c.Session.CloneStrategy
		}
		repos = append(repos, r)
	}
	return Project{
		ID: ps.ID, Name: ps.Name, Slug: ps.Slug, Definition: ps.Project, Root: ps.Root,
		Repos: repos, CreatedAt: ps.CreatedAt,
	}
}

func (l *Local) ProjectDefinitions(context.Context) ([]Definition, error) {
	cfg := l.engine.Config()
	out := make([]Definition, 0, len(cfg.Projects))
	for _, p := range cfg.Projects {
		def := Definition{Name: p.Name, Agent: p.Agent}
		for _, r := range p.Repos {
			dir := r.Dir
			if dir == "" {
				dir = git.ExtractRepoName(r.Remote)
			}
			def.Repos = append(def.Repos, projectsvc.RepoSpec{Remote: r.Remote, Dir: dir, CloneStrategy: cfg.GetCloneStrategy(r.Remote)})
		}
		out = append(out, def)
	}
	return out, nil
}

func (l *Local) DefaultCloneStrategy(_ context.Context, remote string) (string, error) {
	return l.engine.Config().GetCloneStrategy(remote), nil
}

func (l *Local) CreateProject(ctx context.Context, opts projectsvc.CreateOptions) (Project, error) {
	opts.Background = true
	projects := l.engine.Projects()
	ps, err := projects.Create(ctx, opts)
	if err != nil {
		return Project{}, err
	}
	children, err := projects.Children(ctx, ps)
	if err != nil {
		return Project{}, err
	}
	return projectOf(ps, children), nil
}

func (l *Local) ProjectRisk(ctx context.Context, ref string) ([]RepoRisk, error) {
	ps, err := l.engine.Projects().Resolve(ctx, ref)
	if err != nil {
		return nil, err
	}
	sessions := l.engine.Sessions()
	out := make([]RepoRisk, 0, len(ps.Repos))
	for _, r := range ps.Repos {
		risk := RepoRisk{Dir: r.Dir}
		sess, err := sessions.GetSession(ctx, r.SessionID)
		if err != nil {
			risk.Missing = true
			out = append(out, risk)
			continue
		}
		risk.CloneStrategy = sess.CloneStrategy
		check, err := sessions.CheckSessionRisk(ctx, r.SessionID)
		if err != nil {
			return nil, err
		}
		risk.UncommittedChanges = check.UncommittedChanges
		risk.UnpushedCommits = check.UnpushedCommits
		out = append(out, risk)
	}
	return out, nil
}

func (l *Local) RecycleProject(ctx context.Context, ref string) error {
	return l.engine.Projects().Recycle(ctx, ref, io.Discard)
}

func (l *Local) DeleteProject(ctx context.Context, ref string) error {
	return l.engine.Projects().Delete(ctx, ref)
}

func (l *Local) OpenProject(ctx context.Context, ref string) error {
	return l.engine.Projects().Open(ctx, ref, true)
}

func (l *Local) ProjectOf(ctx context.Context, sessionID string) (string, bool, error) {
	ps, ok, err := l.engine.Projects().ProjectOf(ctx, sessionID)
	if err != nil || !ok {
		return "", ok, err
	}
	return ps.Slug, true, nil
}

// IsWorktree reports whether a recycle of the repository deletes it, as a
// worktree's does.
func (r RepoRisk) IsWorktree() bool {
	return r.CloneStrategy == session.CloneStrategyWorktree
}
