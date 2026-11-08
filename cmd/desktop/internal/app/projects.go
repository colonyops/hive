package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/colonyops/hive/internal/domain/project"
	"github.com/colonyops/hive/internal/domain/session"
	projectsvc "github.com/colonyops/hive/internal/hive/project"
	"github.com/colonyops/hive/internal/platform/git"
)

const (
	newProjectJobActionID     = "new-project"
	deleteProjectJobActionID  = "delete-project"
	recycleProjectJobActionID = "recycle-project"
)

// ProjectRepoView is one repository of a project session: the child hive
// session behind it, or Missing when that session is gone.
type ProjectRepoView struct {
	Dir           string `json:"dir"`
	SessionID     string `json:"sessionId"`
	Remote        string `json:"remote"`
	RepoIdentity  string `json:"repoIdentity"`
	Path          string `json:"path"`
	Branch        string `json:"branch"`
	CloneStrategy string `json:"cloneStrategy"`
	Missing       bool   `json:"missing"`
}

// ProjectView is a project session as the Code tree lists it. Slug is the
// tmux session the agent runs in, at Root.
type ProjectView struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Slug      string            `json:"slug"`
	Project   string            `json:"project"`
	Root      string            `json:"root"`
	Repos     []ProjectRepoView `json:"repos"`
	CreatedAt time.Time         `json:"createdAt"`
}

// ProjectDefinitionView is a project from the projects: section of config.
type ProjectDefinitionView struct {
	Name  string               `json:"name"`
	Agent string               `json:"agent"`
	Repos []ProjectRepoRequest `json:"repos"`
}

// ProjectRepoRequest is one repository of a New project form. An empty Dir
// takes the repository name; an empty CloneStrategy takes the rules'.
type ProjectRepoRequest struct {
	Remote        string `json:"remote"`
	Dir           string `json:"dir"`
	CloneStrategy string `json:"cloneStrategy"`
}

type CreateProjectRequest struct {
	Name    string               `json:"name"`
	Project string               `json:"project"`
	Repos   []ProjectRepoRequest `json:"repos"`
	Prompt  string               `json:"prompt"`
	Agent   string               `json:"agent"`
}

// ProjectCreateProgress is the state of the last project create. The form
// reads it on each projects:progress wake-up.
type ProjectCreateProgress struct {
	Name  string            `json:"name"`
	Slug  string            `json:"slug"`
	Steps []projectsvc.Step `json:"steps"`
	Done  bool              `json:"done"`
	Error string            `json:"error"`
}

// ProjectRepoRisk is what recycling or deleting one repository of a project
// would discard.
type ProjectRepoRisk struct {
	Dir           string `json:"dir"`
	CloneStrategy string `json:"cloneStrategy"`
	SessionRisk
}

// ListProjects lists the project sessions on this machine with their
// repositories.
func (s *SessionsService) ListProjects(ctx context.Context) ([]ProjectView, error) {
	projects := s.hive.Projects()
	all, err := projects.List(ctx)
	if err != nil {
		return nil, Wrap(err, KindInternal, "listing project sessions")
	}
	out := make([]ProjectView, 0, len(all))
	for _, ps := range all {
		children, err := projects.Children(ctx, ps)
		if err != nil {
			return nil, Wrap(err, KindInternal, "reading project session %q", ps.Name)
		}
		out = append(out, projectViewOf(ps, children))
	}
	return out, nil
}

func projectViewOf(ps project.Session, children []projectsvc.Child) ProjectView {
	repos := make([]ProjectRepoView, 0, len(children))
	for i, c := range children {
		v := ProjectRepoView{Dir: c.Dir, SessionID: ps.Repos[i].SessionID, Branch: c.Branch}
		if c.Session.ID == "" {
			v.Missing = true
		} else {
			v.Remote = c.Session.Remote
			v.RepoIdentity = git.RemoteIdentity(c.Session.Remote)
			v.Path = c.Session.Path
			v.CloneStrategy = c.Session.CloneStrategy
		}
		repos = append(repos, v)
	}
	return ProjectView{
		ID: ps.ID, Name: ps.Name, Slug: ps.Slug, Project: ps.Project, Root: ps.Root,
		Repos: repos, CreatedAt: ps.CreatedAt,
	}
}

// ProjectDefinitions lists the projects: section of config, with each
// repository's clone strategy as the rules resolve it.
func (s *SessionsService) ProjectDefinitions(context.Context) []ProjectDefinitionView {
	cfg := s.hive.Config()
	out := make([]ProjectDefinitionView, 0, len(cfg.Projects))
	for _, p := range cfg.Projects {
		def := ProjectDefinitionView{Name: p.Name, Agent: p.Agent}
		for _, r := range p.Repos {
			dir := r.Dir
			if dir == "" {
				dir = git.ExtractRepoName(r.Remote)
			}
			def.Repos = append(def.Repos, ProjectRepoRequest{Remote: r.Remote, Dir: dir, CloneStrategy: cfg.GetCloneStrategy(r.Remote)})
		}
		out = append(out, def)
	}
	return out
}

// DefaultCloneStrategy is the clone strategy the rules give remote, for the
// New project form's per-repository default.
func (s *SessionsService) DefaultCloneStrategy(_ context.Context, remote string) string {
	return s.hive.Config().GetCloneStrategy(remote)
}

// CreateProject validates a New project form and creates the project session
// as a background job. Progress per repository is read with
// ProjectCreateProgress.
func (s *SessionsService) CreateProject(ctx context.Context, req CreateProjectRequest) (int64, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return 0, Errorf(KindInvalid, "session name is required")
	}
	if err := session.ValidateName(name); err != nil {
		return 0, Wrap(err, KindInvalid, "session name")
	}
	definition := strings.TrimSpace(req.Project)
	if definition == "" && len(req.Repos) == 0 {
		return 0, Errorf(KindInvalid, "choose a project or at least one repository")
	}
	opts := projectsvc.CreateOptions{
		Name:       name,
		Prompt:     strings.TrimSpace(req.Prompt),
		Agent:      s.resolveLaunchAgent(ctx, req.Agent),
		Background: true,
	}
	// A definition picked in the form arrives with its repos already listed,
	// so the form's list is the one that counts: the user may have changed a
	// folder or a clone strategy.
	opts.Project = definition
	if len(req.Repos) > 0 {
		opts.Project = ""
		for _, r := range req.Repos {
			remote := strings.TrimSpace(r.Remote)
			if remote == "" {
				return 0, Errorf(KindInvalid, "every repository needs a remote")
			}
			dir := strings.TrimSpace(r.Dir)
			if dir != "" {
				if err := project.ValidateDir(dir); err != nil {
					return 0, Wrap(err, KindInvalid, "folder for %s", remote)
				}
			}
			opts.Repos = append(opts.Repos, projectsvc.RepoSpec{Remote: remote, Dir: dir, CloneStrategy: strings.TrimSpace(r.CloneStrategy)})
		}
	}

	progress := &ProjectCreateProgress{Name: name, Slug: session.Slugify(name)}
	s.setProjectProgress(ctx, progress)
	opts.OnStep = func(step projectsvc.Step) {
		s.projectProgressMu.Lock()
		i := slices.IndexFunc(progress.Steps, func(have projectsvc.Step) bool { return have.Dir == step.Dir })
		if i < 0 {
			progress.Steps = append(progress.Steps, step)
		} else {
			if step.Remote == "" {
				step.Remote = progress.Steps[i].Remote
			}
			progress.Steps[i] = step
		}
		s.projectProgressMu.Unlock()
		s.events.Publish(ctx, events.ProjectProgress{Name: name})
	}

	s.logger.Info().Str("session_name", name).Str("project", definition).Int("repos", len(opts.Repos)).Msg("creating project session")
	return s.jobs.Track(ctx, "Create project session", newProjectJobActionID, name, func(bg context.Context) error {
		_, err := s.hive.Projects().Create(bg, opts)
		s.projectProgressMu.Lock()
		progress.Done = true
		if err != nil {
			progress.Error = err.Error()
		}
		s.projectProgressMu.Unlock()
		s.events.Publish(bg, events.ProjectProgress{Name: name})
		s.events.Publish(bg, events.SessionsUpdated{})
		if errors.Is(err, session.ErrDuplicateName) || errors.Is(err, project.ErrDuplicateName) {
			return fmt.Errorf("a session named %q already exists", name)
		}
		return err
	}), nil
}

func (s *SessionsService) setProjectProgress(ctx context.Context, p *ProjectCreateProgress) {
	s.projectProgressMu.Lock()
	s.projectProgress = p
	s.projectProgressMu.Unlock()
	s.events.Publish(ctx, events.ProjectProgress{Name: p.Name})
}

// ProjectCreateProgress returns the state of the last project create, or an
// empty value when there was none in this run.
func (s *SessionsService) ProjectCreateProgress(context.Context) ProjectCreateProgress {
	s.projectProgressMu.Lock()
	defer s.projectProgressMu.Unlock()
	if s.projectProgress == nil {
		return ProjectCreateProgress{}
	}
	out := *s.projectProgress
	out.Steps = slices.Clone(out.Steps)
	return out
}

// ProjectRisk reports, per repository, the work a recycle or delete of the
// project session would discard.
func (s *SessionsService) ProjectRisk(ctx context.Context, id string) ([]ProjectRepoRisk, error) {
	ps, err := s.resolveProject(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]ProjectRepoRisk, 0, len(ps.Repos))
	for _, r := range ps.Repos {
		risk := ProjectRepoRisk{Dir: r.Dir}
		sess, err := s.hive.Sessions().GetSession(ctx, r.SessionID)
		if err != nil {
			out = append(out, risk)
			continue
		}
		risk.CloneStrategy = sess.CloneStrategy
		check, err := s.hive.Sessions().CheckSessionRisk(ctx, r.SessionID)
		if err != nil {
			return nil, Wrap(err, KindInternal, "checking %s of project session %q", r.Dir, ps.Name)
		}
		risk.SessionRisk = SessionRisk{
			UncommittedChanges: check.UncommittedChanges,
			UnpushedCommits:    check.UnpushedCommits,
			RecycleDeletes:     sess.CloneStrategy == session.CloneStrategyWorktree,
		}
		out = append(out, risk)
	}
	return out, nil
}

// RecycleProject recycles every repository of the project session, then
// removes its root and its terminal, as a background job.
func (s *SessionsService) RecycleProject(ctx context.Context, id string) (int64, error) {
	return s.projectJob(ctx, id, "Recycle project session", recycleProjectJobActionID, func(bg context.Context, ref string) error {
		return s.hive.Projects().Recycle(bg, ref, io.Discard)
	})
}

// DeleteProject deletes every repository of the project session, its root,
// and its terminal, as a background job.
func (s *SessionsService) DeleteProject(ctx context.Context, id string) (int64, error) {
	return s.projectJob(ctx, id, "Delete project session", deleteProjectJobActionID, func(bg context.Context, ref string) error {
		return s.hive.Projects().Delete(bg, ref)
	})
}

func (s *SessionsService) projectJob(ctx context.Context, id, label, actionID string, op func(context.Context, string) error) (int64, error) {
	ps, err := s.resolveProject(ctx, id)
	if err != nil {
		return 0, err
	}
	return s.jobs.Track(ctx, label, actionID, ps.Name, func(bg context.Context) error {
		err := op(bg, ps.ID)
		s.events.Publish(bg, events.SessionsUpdated{})
		return err
	}), nil
}

func (s *SessionsService) resolveProject(ctx context.Context, id string) (project.Session, error) {
	if strings.TrimSpace(id) == "" {
		return project.Session{}, Errorf(KindInvalid, "project session id is required")
	}
	ps, err := s.hive.Projects().Resolve(ctx, id)
	if errors.Is(err, project.ErrNotFound) {
		return project.Session{}, Errorf(KindNotFound, "no project session %q", id)
	}
	if err != nil {
		return project.Session{}, Wrap(err, KindInternal, "reading project session %q", id)
	}
	return ps, nil
}

// startProjectTmux starts the terminal of the project session whose slug is
// slug, in its root. found is false when no project session has that slug.
func (s *SessionsService) startProjectTmux(ctx context.Context, slug string) (result multiplexer.LaunchResult, found bool, err error) {
	all, err := s.hive.Projects().List(ctx)
	if err != nil {
		return result, false, Wrap(err, KindInternal, "listing project sessions")
	}
	i := slices.IndexFunc(all, func(ps project.Session) bool { return ps.Slug == slug })
	if i < 0 {
		return result, false, nil
	}
	// Detached for the same reason StartTmuxSession is.
	if err := s.hive.Projects().Open(ctx, all[i].ID, true); err != nil {
		return result, true, sessionStartError(all[i].Name, err)
	}
	return multiplexer.LaunchResult{Created: true}, true, nil
}

// projectRoot is the root directory of the project session whose slug is
// slug, for a terminal window opened in it.
func (s *SessionsService) projectRoot(ctx context.Context, slug string) (string, bool) {
	all, err := s.hive.Projects().List(ctx)
	if err != nil {
		s.logger.Debug().Err(err).Msg("list project sessions for a terminal directory")
		return "", false
	}
	i := slices.IndexFunc(all, func(ps project.Session) bool { return ps.Slug == slug })
	if i < 0 {
		return "", false
	}
	return all[i].Root, true
}
