package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/colonyops/hive/internal/domain/project"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive/daemonclient"
	projectsvc "github.com/colonyops/hive/internal/hive/project"
	"github.com/colonyops/hive/internal/hive/projectapi"
	"github.com/colonyops/hive/internal/platform/git"
)

const (
	newProjectJobActionID     = "new-project"
	deleteProjectJobActionID  = "delete-project"
	recycleProjectJobActionID = "recycle-project"
)

// ProjectRepoView is one repository of a project session: the child hive
// session behind it, or Missing when that session is gone. SessionID is in
// the "<server>:<id>" form for a project on a server.
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
// tmux session the agent runs in, at Root. ID and Slug carry the
// "<server>:" prefix for a project on a server.
type ProjectView struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Slug      string            `json:"slug"`
	Server    string            `json:"server"`
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

// CreateProjectRequest is a New project form. Server is "" for this machine.
type CreateProjectRequest struct {
	Name    string               `json:"name"`
	Server  string               `json:"server"`
	Project string               `json:"project"`
	Repos   []ProjectRepoRequest `json:"repos"`
	Prompt  string               `json:"prompt"`
	Agent   string               `json:"agent"`
}

// ProjectCreateProgress is the state of the last project create. The form
// reads it on each projects:progress wake-up.
type ProjectCreateProgress struct {
	Name   string            `json:"name"`
	Server string            `json:"server"`
	Slug   string            `json:"slug"`
	Steps  []projectsvc.Step `json:"steps"`
	Done   bool              `json:"done"`
	Error  string            `json:"error"`
}

// ProjectRepoRisk is what recycling or deleting one repository of a project
// would discard.
type ProjectRepoRisk struct {
	Dir           string `json:"dir"`
	CloneStrategy string `json:"cloneStrategy"`
	SessionRisk
}

// projectTarget is where a project id points: the project API of its
// server, and its id there.
type projectTarget struct {
	api    projectapi.API
	server string
	ref    string
}

func (t projectTarget) local() bool { return t.server == session.LocalServer }

// qualify puts the "<server>:" prefix on an id or slug of a server's
// project, the form the Code view keys server rows by.
func (t projectTarget) qualify(id string) string {
	return session.Ref{Server: t.server, ID: id}.String()
}

func (s *SessionsService) localProjects() projectapi.API {
	return projectapi.NewLocal(s.hive)
}

func (s *SessionsService) projectsOn(server string) (projectTarget, error) {
	server = strings.TrimSpace(server)
	if server == "" || server == session.LocalServer {
		return projectTarget{api: s.localProjects(), server: session.LocalServer}, nil
	}
	fleet := s.fleet()
	if fleet == nil {
		return projectTarget{}, Errorf(KindUnavailable, "the server fleet is not loaded")
	}
	srv, ok := fleet.Server(server)
	if !ok {
		return projectTarget{}, Errorf(KindNotFound, "server %q is not under servers: in config.yaml", server)
	}
	return projectTarget{api: srv.Client(), server: server}, nil
}

func (s *SessionsService) projectTargetOf(id string) (projectTarget, error) {
	if strings.TrimSpace(id) == "" {
		return projectTarget{}, Errorf(KindInvalid, "project session id is required")
	}
	ref, err := session.ParseRef(id, session.LocalServer)
	if err != nil {
		return projectTarget{}, Wrap(err, KindInvalid, "project session id")
	}
	target, err := s.projectsOn(ref.Server)
	if err != nil {
		return projectTarget{}, err
	}
	target.ref = ref.ID
	return target, nil
}

func (t projectTarget) wrap(err error, kind Kind, format string, args ...any) error {
	if errors.Is(err, project.ErrNotFound) || errors.Is(err, session.ErrNotFound) {
		return Errorf(KindNotFound, "no project session %q", t.qualify(t.ref))
	}
	if t.local() {
		return Wrap(err, kind, format, args...)
	}
	return serverError(err, KindUnavailable, format+" on server %q", append(args, t.server)...)
}

// ListProjects lists the project sessions on this machine.
func (s *SessionsService) ListProjects(ctx context.Context) ([]ProjectView, error) {
	local, err := s.localProjects().ListProjects(ctx)
	if err != nil {
		return nil, Wrap(err, KindInternal, "listing project sessions")
	}
	out := make([]ProjectView, 0, len(local))
	here := projectTarget{server: session.LocalServer}
	for _, p := range local {
		out = append(out, here.view(p))
	}
	return out, nil
}

// ListServerProjects lists the project sessions on every server that
// answers. It is separate from ListProjects, so local projects never wait on
// a server. A server that does not answer, or whose hive is too old for
// projects, lists none: its session list already shows it offline.
func (s *SessionsService) ListServerProjects(ctx context.Context) []ProjectView {
	fleet := s.fleet()
	if fleet == nil {
		return []ProjectView{}
	}
	all := fleet.Servers()
	found := make([][]ProjectView, len(all))
	var wg sync.WaitGroup
	for i, srv := range all {
		wg.Go(func() {
			ctx, cancel := context.WithTimeout(ctx, serverReadTimeout)
			defer cancel()
			name := srv.Config().Name
			projects, err := srv.Client().ListProjects(ctx)
			if err != nil {
				if errors.Is(err, daemonclient.ErrProjectsUnsupported) {
					s.logger.Debug().Err(err).Str("server", name).Msg("list project sessions on server")
				} else {
					s.logger.Info().Err(err).Str("server", name).Msg("list project sessions on server")
				}
				return
			}
			there := projectTarget{server: name}
			for _, p := range projects {
				found[i] = append(found[i], there.view(p))
			}
		})
	}
	wg.Wait()
	out := []ProjectView{}
	for _, views := range found {
		out = append(out, views...)
	}
	return out
}

func (t projectTarget) view(p projectapi.Project) ProjectView {
	repos := make([]ProjectRepoView, 0, len(p.Repos))
	for _, r := range p.Repos {
		repos = append(repos, ProjectRepoView{
			Dir: r.Dir, SessionID: t.qualify(r.SessionID), Remote: r.Remote, RepoIdentity: git.RemoteIdentity(r.Remote),
			Path: r.Path, Branch: r.Branch, CloneStrategy: r.CloneStrategy, Missing: r.Missing,
		})
	}
	server := ""
	if !t.local() {
		server = t.server
	}
	return ProjectView{
		ID: t.qualify(p.ID), Name: p.Name, Slug: t.qualify(p.Slug), Server: server, Project: p.Definition,
		Root: p.Root, Repos: repos, CreatedAt: p.CreatedAt,
	}
}

// ProjectDefinitions lists the projects: section of the config on server,
// "" for this machine, with each repository's clone strategy as the rules
// resolve it.
func (s *SessionsService) ProjectDefinitions(ctx context.Context, server string) ([]ProjectDefinitionView, error) {
	target, err := s.projectsOn(server)
	if err != nil {
		return nil, err
	}
	defs, err := target.api.ProjectDefinitions(ctx)
	if err != nil {
		return nil, target.wrap(err, KindInternal, "reading the projects in config")
	}
	out := make([]ProjectDefinitionView, 0, len(defs))
	for _, d := range defs {
		view := ProjectDefinitionView{Name: d.Name, Agent: d.Agent, Repos: []ProjectRepoRequest{}}
		for _, r := range d.Repos {
			view.Repos = append(view.Repos, ProjectRepoRequest{Remote: r.Remote, Dir: r.Dir, CloneStrategy: r.CloneStrategy})
		}
		out = append(out, view)
	}
	return out, nil
}

// DefaultCloneStrategy is the clone strategy the rules on server give
// remote, for the New project form's per-repository default.
func (s *SessionsService) DefaultCloneStrategy(ctx context.Context, server, remote string) (string, error) {
	target, err := s.projectsOn(server)
	if err != nil {
		return "", err
	}
	strategy, err := target.api.DefaultCloneStrategy(ctx, remote)
	if err != nil {
		return "", target.wrap(err, KindInternal, "reading the clone strategy of %s", remote)
	}
	return strategy, nil
}

// CreateProject validates a New project form and creates the project session
// as a background job. Progress per repository is read with
// ProjectCreateProgress.
func (s *SessionsService) CreateProject(ctx context.Context, req CreateProjectRequest) (int64, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return 0, Errorf(KindInvalid, "project name is required")
	}
	if err := session.ValidateName(name); err != nil {
		return 0, Wrap(err, KindInvalid, "project name")
	}
	definition := strings.TrimSpace(req.Project)
	if definition == "" && len(req.Repos) == 0 {
		return 0, Errorf(KindInvalid, "choose a project or at least one repository")
	}
	target, err := s.projectsOn(req.Server)
	if err != nil {
		return 0, err
	}
	agent := strings.TrimSpace(req.Agent)
	if target.local() {
		agent = s.resolveLaunchAgent(ctx, agent)
	}
	opts := projectsvc.CreateOptions{
		Name:       name,
		Prompt:     strings.TrimSpace(req.Prompt),
		Agent:      agent,
		Background: true,
	}
	// A definition picked in the form arrives with its repos already listed,
	// so the form's list is the one that counts: the user may have added or
	// removed a repository.
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

	server := ""
	if !target.local() {
		server = target.server
	}
	progress := &ProjectCreateProgress{Name: name, Server: server, Slug: target.qualify(session.Slugify(name))}
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

	s.logger.Info().Str("session_name", name).Str("server", target.server).Str("project", definition).Int("repos", len(opts.Repos)).Msg("creating project session")
	return s.jobs.Track(ctx, "Create project session", newProjectJobActionID, name, func(bg context.Context) error {
		_, err := target.api.CreateProject(bg, opts)
		if errors.Is(err, session.ErrDuplicateName) || errors.Is(err, project.ErrDuplicateName) {
			err = fmt.Errorf("a session named %q already exists", name)
		}
		s.projectProgressMu.Lock()
		progress.Done = true
		if err != nil {
			progress.Error = err.Error()
		}
		s.projectProgressMu.Unlock()
		s.events.Publish(bg, events.ProjectProgress{Name: name})
		s.events.Publish(bg, events.SessionsUpdated{})
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
	target, err := s.projectTargetOf(id)
	if err != nil {
		return nil, err
	}
	risks, err := target.api.ProjectRisk(ctx, target.ref)
	if err != nil {
		return nil, target.wrap(err, KindInternal, "checking project session %q", id)
	}
	out := make([]ProjectRepoRisk, 0, len(risks))
	for _, r := range risks {
		out = append(out, ProjectRepoRisk{
			Dir:                r.Dir,
			CloneStrategy:      r.CloneStrategy,
			UncommittedChanges: r.UncommittedChanges,
			UnpushedCommits:    r.UnpushedCommits,
			RecycleDeletes:     r.IsWorktree(),
		})
	}
	return out, nil
}

// RecycleProject recycles every repository of the project session, then
// removes its root and its terminal, as a background job.
func (s *SessionsService) RecycleProject(ctx context.Context, id string) (int64, error) {
	return s.projectJob(ctx, id, "Recycle project session", recycleProjectJobActionID, projectapi.API.RecycleProject)
}

// DeleteProject deletes every repository of the project session, its root,
// and its terminal, as a background job.
func (s *SessionsService) DeleteProject(ctx context.Context, id string) (int64, error) {
	return s.projectJob(ctx, id, "Delete project session", deleteProjectJobActionID, projectapi.API.DeleteProject)
}

func (s *SessionsService) projectJob(ctx context.Context, id, label, actionID string, op func(projectapi.API, context.Context, string) error) (int64, error) {
	target, err := s.projectTargetOf(id)
	if err != nil {
		return 0, err
	}
	return s.jobs.Track(ctx, label, actionID, id, func(bg context.Context) error {
		err := op(target.api, bg, target.ref)
		s.events.Publish(bg, events.SessionsUpdated{})
		if err != nil {
			return target.wrap(err, KindInternal, "%s %q", strings.ToLower(label), id)
		}
		return nil
	}), nil
}

// startProjectTmux starts the terminal of the project session whose slug is
// slug on server, in its root. found is false when no project session there
// has that slug.
func (s *SessionsService) startProjectTmux(ctx context.Context, server, slug string) (result multiplexer.LaunchResult, found bool, err error) {
	target, err := s.projectsOn(server)
	if err != nil {
		return result, false, err
	}
	all, err := target.api.ListProjects(ctx)
	if err != nil {
		if errors.Is(err, daemonclient.ErrProjectsUnsupported) {
			return result, false, nil
		}
		return result, false, target.wrap(err, KindInternal, "listing project sessions")
	}
	i := slices.IndexFunc(all, func(p projectapi.Project) bool { return p.Slug == slug })
	if i < 0 {
		return result, false, nil
	}
	// Detached for the same reason StartTmuxSession is.
	if err := target.api.OpenProject(ctx, all[i].ID); err != nil {
		if target.local() {
			return result, true, sessionStartError(all[i].Name, err)
		}
		return result, true, serverError(err, KindUnavailable, "starting project %q on server %q", all[i].Name, server)
	}
	return multiplexer.LaunchResult{Created: true}, true, nil
}

// projectRoot is the root directory of the local project session whose slug
// is slug, for a terminal window opened in it.
func (s *SessionsService) projectRoot(ctx context.Context, slug string) (string, bool) {
	all, err := s.localProjects().ListProjects(ctx)
	if err != nil {
		s.logger.Debug().Err(err).Msg("list project sessions for a terminal directory")
		return "", false
	}
	i := slices.IndexFunc(all, func(p projectapi.Project) bool { return p.Slug == slug })
	if i < 0 {
		return "", false
	}
	return all[i].Root, true
}
