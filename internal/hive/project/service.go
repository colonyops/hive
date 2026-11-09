// Package project creates and tears down project sessions: one tmux session
// at a generated root that links to one ordinary hive session per repository.
package project

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/project"
	"github.com/colonyops/hive/internal/domain/session"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/platform/git"
	"github.com/colonyops/hive/pkg/logutils"
	"github.com/colonyops/hive/pkg/randid"
)

// SessionEnv names the variable that carries the project session id into
// the root's tmux session.
const SessionEnv = "HIVE_PROJECT_SESSION"

type Service struct {
	log      zerolog.Logger
	store    project.Store
	sessions *sessionsvc.Service
	cfg      *config.Config
}

func NewService(log zerolog.Logger, store project.Store, sessions *sessionsvc.Service, cfg *config.Config) *Service {
	return &Service{log: logutils.Component(log, "project"), store: store, sessions: sessions, cfg: cfg}
}

// RepoSpec names one repository of a new project session. Remote may also be
// a path to a local checkout, whose origin is cloned and whose files the
// copy rules read.
type RepoSpec struct {
	Remote        string
	Dir           string
	CloneStrategy string
}

type CreateOptions struct {
	Name string
	// Project names a definition in config.Projects. Its repos come before
	// Repos.
	Project    string
	Repos      []RepoSpec
	Prompt     string
	Agent      string
	Background bool
	SkipSpawn  bool
	Progress   io.Writer
	// OnStep reports each repository as it moves through the create, for a
	// caller that shows progress per repository.
	OnStep func(Step)
}

// StepState is where one repository of a create stands.
type StepState string

const (
	StepPending StepState = "pending"
	StepRunning StepState = "running"
	StepDone    StepState = "done"
	StepFailed  StepState = "failed"
	// StepRemoved is a repository the create checked out, then removed
	// because another one failed.
	StepRemoved StepState = "removed"
)

// Step is the state of one repository of a create. Dir is empty for the
// root: the links, AGENTS.md, and the agent.
type Step struct {
	Dir    string    `json:"dir"`
	Remote string    `json:"remote"`
	State  StepState `json:"state"`
	Detail string    `json:"detail"`
}

// Child is a project session's repository with its hive session.
type Child struct {
	Dir     string          `json:"dir"`
	Session session.Session `json:"session"`
	Branch  string          `json:"branch,omitempty"`
}

type resolvedRepo struct {
	dir, remote, source, strategy string
}

// Create checks out every repository as its own hive session, writes the
// root, and starts one tmux session there. A failure removes the children
// this call created.
func (s *Service) Create(ctx context.Context, opts CreateOptions) (project.Session, error) {
	if err := session.ValidateName(opts.Name); err != nil {
		return project.Session{}, err
	}
	slug := session.Slugify(opts.Name)
	if err := s.claimSlug(ctx, slug); err != nil {
		return project.Session{}, err
	}

	specs, instructions, agent, err := s.specs(opts)
	if err != nil {
		return project.Session{}, err
	}
	repos, err := s.resolveRepos(ctx, specs)
	if err != nil {
		return project.Session{}, err
	}

	id := randid.Generate(6)
	ps := project.Session{
		ID:      id,
		Name:    opts.Name,
		Slug:    slug,
		Project: opts.Project,
		Root:    filepath.Join(s.cfg.ProjectsDir(), fmt.Sprintf("%s-%s", slug, id)),
	}

	// The checkouts run at once, so steps and progress lines arrive from
	// several goroutines; callers get them one at a time.
	var reportMu sync.Mutex
	step := func(dir, remote string, state StepState, detail string) {
		if opts.OnStep == nil {
			return
		}
		reportMu.Lock()
		defer reportMu.Unlock()
		opts.OnStep(Step{Dir: dir, Remote: remote, State: state, Detail: detail})
	}
	progress := opts.Progress
	if progress != nil {
		progress = &lockedWriter{w: progress, mu: &reportMu}
	}
	for _, r := range repos {
		step(r.dir, r.remote, StepPending, r.strategy)
	}
	step("", "", StepPending, "")

	results := make([]*session.Session, len(repos))
	undo := func() {
		for i, sess := range results {
			if sess == nil {
				continue
			}
			if err := s.sessions.DeleteSession(context.WithoutCancel(ctx), sess.ID); err != nil {
				s.log.Warn().Err(err).Str("session_id", sess.ID).Msg("remove child of failed project session")
				continue
			}
			step(repos[i].dir, repos[i].remote, StepRemoved, "")
		}
		if err := os.RemoveAll(ps.Root); err != nil {
			s.log.Warn().Err(err).Str("root", ps.Root).Msg("remove root of failed project session")
		}
	}

	// Every repository checks out at once; resolveRepos let no repository in
	// twice. A failure stops the checkouts not yet begun, but lets the
	// running ones finish: a clone stopped halfway would leave a checkout
	// that the undo does not know about.
	var group errgroup.Group
	var failed atomic.Bool
	for i, r := range repos {
		group.Go(func() error {
			if failed.Load() || ctx.Err() != nil {
				return nil
			}
			progressf(progress, "Checking out %s (%s)...", r.dir, r.remote)
			step(r.dir, r.remote, StepRunning, r.strategy)
			sess, err := s.sessions.CreateSession(ctx, sessionsvc.CreateOptions{
				Name:          fmt.Sprintf("%s-%s", slug, r.dir),
				Remote:        r.remote,
				Source:        r.source,
				CloneStrategy: r.strategy,
				SkipSpawn:     true,
				Progress:      progress,
			})
			if err != nil {
				failed.Store(true)
				step(r.dir, r.remote, StepFailed, err.Error())
				return fmt.Errorf("check out %s: %w", r.dir, err)
			}
			results[i] = sess
			step(r.dir, r.remote, StepDone, sess.Path)
			return nil
		})
	}
	err = group.Wait()
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		undo()
		return project.Session{}, err
	}

	children := make([]Child, 0, len(repos))
	for i, r := range repos {
		children = append(children, Child{Dir: r.dir, Session: *results[i]})
		ps.Repos = append(ps.Repos, project.Repo{Dir: r.dir, SessionID: results[i].ID})
	}

	progressf(opts.Progress, "Writing project root %s...", ps.Root)
	step("", "", StepRunning, ps.Root)
	s.fillBranches(ctx, children)
	if err := writeRoot(ps, children, instructions); err != nil {
		step("", "", StepFailed, err.Error())
		undo()
		return project.Session{}, fmt.Errorf("write project root: %w", err)
	}

	now := time.Now()
	ps.CreatedAt, ps.UpdatedAt = now, now
	if err := s.store.Save(ctx, ps); err != nil {
		step("", "", StepFailed, err.Error())
		undo()
		return project.Session{}, err
	}

	if !opts.SkipSpawn {
		progressf(opts.Progress, "Spawning terminal...")
		if err := s.sessions.SpawnRoot(ctx, s.rootSpawn(ps, children, opts.Prompt, firstNonEmpty(opts.Agent, agent), opts.Background), false); err != nil {
			step("", "", StepFailed, err.Error())
			return ps, err
		}
	}
	step("", "", StepDone, ps.Root)
	s.log.Info().Str("project_session_id", ps.ID).Str("root", ps.Root).Int("repos", len(ps.Repos)).Msg("project session created")
	return ps, nil
}

func (s *Service) rootSpawn(ps project.Session, children []Child, prompt, agent string, background bool) sessionsvc.RootSpawn {
	// Claude Code and codex resolve the root's symlinks and refuse to read
	// or write the real checkouts unless they are granted explicitly.
	addDirs := make([]string, 0, len(children))
	for _, c := range children {
		if c.Session.Path != "" {
			addDirs = append(addDirs, c.Session.Path)
		}
	}
	return sessionsvc.RootSpawn{
		Path:       ps.Root,
		Name:       ps.Name,
		Slug:       ps.Slug,
		Prompt:     prompt,
		Agent:      agent,
		Background: background,
		AddDirs:    addDirs,
		// An empty HIVE_AGENT_SESSION hides one inherited from the tmux
		// server, so hive commands resolve the caller from the working
		// directory: the child repository the agent stands in.
		Env: []string{sessionsvc.AgentSessionEnv + "=", SessionEnv + "=" + ps.ID},
	}
}

// Open attaches to the project session's tmux session, and starts it in the
// root when it is gone.
func (s *Service) Open(ctx context.Context, ref string, background bool) error {
	ps, err := s.Resolve(ctx, ref)
	if err != nil {
		return err
	}
	agent := ""
	if def, ok := s.cfg.Project(ps.Project); ok {
		agent = def.Agent
	}
	children, err := s.Children(ctx, ps)
	if err != nil {
		return err
	}
	return s.sessions.SpawnRoot(ctx, s.rootSpawn(ps, children, "", agent, background), true)
}

func (s *Service) specs(opts CreateOptions) (specs []RepoSpec, instructions, agent string, err error) {
	if opts.Project != "" {
		def, ok := s.cfg.Project(opts.Project)
		if !ok {
			return nil, "", "", fmt.Errorf("project %q is not defined in config", opts.Project)
		}
		for _, r := range def.Repos {
			specs = append(specs, RepoSpec{Remote: r.Remote, Dir: r.Dir})
		}
		instructions, agent = def.Instructions, def.Agent
	}
	specs = append(specs, opts.Repos...)
	if len(specs) == 0 {
		return nil, "", "", errors.New("a project session needs at least one repo")
	}
	return specs, instructions, agent, nil
}

func (s *Service) resolveRepos(ctx context.Context, specs []RepoSpec) ([]resolvedRepo, error) {
	out := make([]resolvedRepo, 0, len(specs))
	dirs := make(map[string]bool, len(specs))
	remotes := make(map[string]string, len(specs))
	for _, spec := range specs {
		remote, source := strings.TrimSpace(spec.Remote), ""
		if info, err := os.Stat(remote); err == nil && info.IsDir() {
			abs, err := filepath.Abs(remote)
			if err != nil {
				return nil, err
			}
			detected, err := s.sessions.DetectRemote(ctx, abs)
			if err != nil {
				return nil, fmt.Errorf("repo %s: detect origin: %w", remote, err)
			}
			remote, source = detected, abs
		} else {
			repo, err := s.sessions.ResolveSessionLaunchRepository(ctx, remote)
			if err != nil {
				return nil, fmt.Errorf("repo %s: %w", remote, err)
			}
			remote, source = repo.Remote, repo.Source
		}

		// One checkout per repository: the agent would see two copies of the
		// same code, and a full clone may reuse a recycled checkout, which two
		// creates at once could both pick.
		identity := git.RemoteIdentity(remote)
		if first, ok := remotes[identity]; ok {
			return nil, fmt.Errorf("repo %s is in the project twice (also as %s)", spec.Remote, first)
		}
		remotes[identity] = spec.Remote

		dir := spec.Dir
		if dir == "" {
			dir = git.ExtractRepoName(remote)
		}
		if err := project.ValidateDir(dir); err != nil {
			return nil, err
		}
		if dirs[dir] {
			return nil, fmt.Errorf("two repos use dir %q; set dir on one of them", dir)
		}
		dirs[dir] = true
		out = append(out, resolvedRepo{dir: dir, remote: remote, source: source, strategy: spec.CloneStrategy})
	}
	return out, nil
}

// claimSlug keeps the project's tmux name clear of every active hive
// session and every other project session.
func (s *Service) claimSlug(ctx context.Context, slug string) error {
	existing, err := s.store.List(ctx)
	if err != nil {
		return err
	}
	for _, ps := range existing {
		if ps.Slug == slug {
			return fmt.Errorf("%w: %q", project.ErrDuplicateName, slug)
		}
	}
	sessions, err := s.sessions.ListSessions(ctx)
	if err != nil {
		return err
	}
	for _, sess := range sessions {
		if sess.State == session.StateActive && (sess.Slug == slug || sessionsvc.Target(sess).Session == slug) {
			return fmt.Errorf("%w: hive session %q uses %q", project.ErrDuplicateName, sess.Name, slug)
		}
	}
	return nil
}

func (s *Service) List(ctx context.Context) ([]project.Session, error) {
	return s.store.List(ctx)
}

// Resolve finds a project session by id, slug, or name.
func (s *Service) Resolve(ctx context.Context, ref string) (project.Session, error) {
	if ps, err := s.store.Get(ctx, ref); err == nil {
		return ps, nil
	} else if !errors.Is(err, project.ErrNotFound) {
		return project.Session{}, err
	}
	all, err := s.store.List(ctx)
	if err != nil {
		return project.Session{}, err
	}
	for _, ps := range all {
		if ps.Slug == ref || ps.Name == ref {
			return ps, nil
		}
	}
	return project.Session{}, fmt.Errorf("%w: %q", project.ErrNotFound, ref)
}

// Children returns the project session's repositories with their sessions.
// A child whose session is gone comes back with an empty Session so the
// caller can show it as missing.
func (s *Service) Children(ctx context.Context, ps project.Session) ([]Child, error) {
	out := make([]Child, 0, len(ps.Repos))
	for _, r := range ps.Repos {
		sess, err := s.sessions.GetSession(ctx, r.SessionID)
		if err != nil {
			s.log.Debug().Err(err).Str("session_id", r.SessionID).Msg("project child session missing")
			out = append(out, Child{Dir: r.Dir})
			continue
		}
		out = append(out, Child{Dir: r.Dir, Session: sess})
	}
	s.fillBranches(ctx, out)
	return out, nil
}

// ProjectOf returns the project session that holds the hive session.
func (s *Service) ProjectOf(ctx context.Context, sessionID string) (project.Session, bool, error) {
	return s.store.ProjectOf(ctx, sessionID)
}

// Delete deletes every child session, the root, and the record.
func (s *Service) Delete(ctx context.Context, ref string) error {
	return s.teardown(ctx, ref, func(id string) error { return s.sessions.DeleteSession(ctx, id) })
}

// Recycle recycles every child session, so full clones return to the reuse
// pool, then removes the root and the record.
func (s *Service) Recycle(ctx context.Context, ref string, w io.Writer) error {
	return s.teardown(ctx, ref, func(id string) error { return s.sessions.RecycleSession(ctx, id, w) })
}

func (s *Service) teardown(ctx context.Context, ref string, each func(id string) error) error {
	ps, err := s.Resolve(ctx, ref)
	if err != nil {
		return err
	}
	ctx = context.WithoutCancel(ctx)

	var errs []error
	for _, r := range ps.Repos {
		if err := each(r.SessionID); err != nil && !errors.Is(err, session.ErrNotFound) {
			errs = append(errs, fmt.Errorf("%s: %w", r.Dir, err))
		}
	}
	if err := s.sessions.KillTmuxSession(ctx, ps.Slug); err != nil {
		s.log.Debug().Err(err).Str("tmux", ps.Slug).Msg("no project tmux session to kill")
	}
	// RemoveAll removes the symlinks, not what they point at.
	if err := os.RemoveAll(ps.Root); err != nil {
		errs = append(errs, fmt.Errorf("remove root: %w", err))
	}
	if err := s.store.Delete(ctx, ps.ID); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (s *Service) fillBranches(ctx context.Context, children []Child) {
	for i := range children {
		c := &children[i]
		if c.Session.ID == "" {
			continue
		}
		if b := c.Session.GetMeta(session.MetaWorktreeBranch); b != "" {
			c.Branch = b
			continue
		}
		branch, err := s.sessions.Git().Branch(ctx, c.Session.Path)
		if err != nil {
			s.log.Debug().Err(err).Str("path", c.Session.Path).Msg("read child branch")
			continue
		}
		c.Branch = branch
	}
}

type lockedWriter struct {
	w  io.Writer
	mu *sync.Mutex
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

func progressf(w io.Writer, format string, args ...any) {
	if w != nil {
		_, _ = fmt.Fprintf(w, format+"\n", args...)
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
