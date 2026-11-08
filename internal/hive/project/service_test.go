package project_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/colonyops/hive/internal/domain/project"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/hive/events/testbus"
	"github.com/colonyops/hive/internal/hive/hivetest"
	projectsvc "github.com/colonyops/hive/internal/hive/project"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/pkg/executil"
)

type recordingMux struct {
	hivetest.Mux
	mu      sync.Mutex
	created []multiplexer.SessionSpec
	killed  []string
}

func (m *recordingMux) CreateSession(_ context.Context, spec multiplexer.SessionSpec) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.created = append(m.created, spec)
	return nil
}

func (m *recordingMux) KillSession(_ context.Context, t multiplexer.Target) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.killed = append(m.killed, t.Session)
	return nil
}

type fixture struct {
	engine   *hive.Engine
	mux      *recordingMux
	frontend string
	backend  string
	dataDir  string
}

func newFixture(t *testing.T, edits ...func(*config.Config)) fixture {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	// The developer's global git config (signing, hooks) must not reach the
	// fixture repos or the clones the service makes.
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")

	origins := t.TempDir()
	dataDir := t.TempDir()
	cfg := hivetest.LoadConfig(t, dataDir)
	for _, edit := range edits {
		edit(cfg)
	}

	database, err := hive.OpenDB(t.Context(), t.TempDir(), cfg.Database)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })

	mux := &recordingMux{}
	engine, err := hive.New(cfg, hive.Ports{
		DB:       database,
		Bus:      testbus.New(t).EventBus,
		Executor: &executil.RealExecutor{},
		Mux:      mux,
		DataDir:  t.TempDir(),
		Logger:   zerolog.Nop(),
	})
	require.NoError(t, err)

	return fixture{
		engine:   engine,
		mux:      mux,
		frontend: makeOrigin(t, origins, "frontend"),
		backend:  makeOrigin(t, origins, "backend"),
		dataDir:  dataDir,
	}
}

// makeOrigin returns a file:// URL to a bare repo with one commit on main.
func makeOrigin(t *testing.T, dir, name string) string {
	t.Helper()
	work := filepath.Join(dir, name+"-work")
	bare := filepath.Join(dir, name+".git")
	run(t, "", "git", "init", "-q", "-b", "main", work)
	require.NoError(t, os.WriteFile(filepath.Join(work, "README.md"), []byte("# "+name+"\n"), 0o644))
	run(t, work, "git", "add", ".")
	run(t, work, "git", "commit", "-q", "-m", "init")
	run(t, "", "git", "clone", "-q", "--bare", work, bare)
	return "file://" + bare
}

func run(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s %v: %s", name, args, out)
	return string(out)
}

func (f fixture) create(t *testing.T, name string) project.Session {
	t.Helper()
	ps, err := f.engine.Projects().Create(t.Context(), projectsvc.CreateOptions{
		Name: name,
		Repos: []projectsvc.RepoSpec{
			{Remote: f.frontend},
			{Remote: f.backend, CloneStrategy: "worktree"},
		},
		Background: true,
	})
	require.NoError(t, err)
	return ps
}

func TestCreateLinksEveryRepoUnderOneRoot(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	ps := f.create(t, "auth fix")

	require.Len(t, ps.Repos, 2)
	assert.Equal(t, "frontend", ps.Repos[0].Dir)
	assert.Equal(t, "backend", ps.Repos[1].Dir)
	assert.True(t, strings.HasPrefix(ps.Root, filepath.Join(f.dataDir, "projects")))

	children, err := f.engine.Projects().Children(ctx, ps)
	require.NoError(t, err)
	require.Len(t, children, 2)
	assert.Equal(t, session.CloneStrategyFull, children[0].Session.CloneStrategy)
	assert.Equal(t, session.CloneStrategyWorktree, children[1].Session.CloneStrategy)
	assert.Equal(t, "main", children[0].Branch)
	assert.NotEmpty(t, children[1].Branch)

	for _, c := range children {
		link := filepath.Join(ps.Root, c.Dir)
		target, err := os.Readlink(link)
		require.NoError(t, err)
		assert.Equal(t, c.Session.Path, target)
		_, err = os.Stat(filepath.Join(link, "README.md"))
		require.NoError(t, err, "README.md reachable through the %s link", c.Dir)

		owner, ok, err := f.engine.Projects().ProjectOf(ctx, c.Session.ID)
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, ps.ID, owner.ID)
	}

	agents, err := os.ReadFile(filepath.Join(ps.Root, "AGENTS.md"))
	require.NoError(t, err)
	assert.Contains(t, string(agents), "`frontend/`")
	assert.Contains(t, string(agents), "`backend/`")
	assert.Contains(t, string(agents), children[1].Branch)
	claude, err := os.ReadFile(filepath.Join(ps.Root, "CLAUDE.md"))
	require.NoError(t, err)
	assert.Equal(t, "@AGENTS.md\n", string(claude))

	require.Len(t, f.mux.created, 1, "children skip spawn; only the root gets a tmux session")
	spec := f.mux.created[0]
	assert.Equal(t, ps.Root, spec.WorkingDirectory)
	assert.Equal(t, ps.Slug, spec.Target.Session)
	assert.Contains(t, spec.Env, sessionsvc.AgentSessionEnv+"=")
	assert.Contains(t, spec.Env, projectsvc.SessionEnv+"="+ps.ID)

	require.NotEmpty(t, spec.Windows)
	agentCmd := spec.Windows[0].Command
	assert.True(t, strings.HasPrefix(agentCmd, "claude "), "default agent is claude: %q", agentCmd)
	for _, c := range children {
		assert.Contains(t, agentCmd, "--add-dir '"+c.Session.Path+"'")
	}
}

func TestRootGrantsNoDirsToAnAgentWithoutTheFlag(t *testing.T) {
	f := newFixture(t, func(cfg *config.Config) { cfg.Agents.Profiles["pi"] = config.AgentProfile{} })
	_, err := f.engine.Projects().Create(t.Context(), projectsvc.CreateOptions{
		Name: "pi", Agent: "pi", Repos: []projectsvc.RepoSpec{{Remote: f.frontend}}, Background: true,
	})
	require.NoError(t, err)
	require.Len(t, f.mux.created, 1)
	cmd := f.mux.created[0].Windows[0].Command
	assert.True(t, strings.HasPrefix(cmd, "pi"), cmd)
	assert.NotContains(t, cmd, "--add-dir")
}

func TestCallerThroughTheRootResolvesToTheChild(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	ps := f.create(t, "caller")
	children, err := f.engine.Projects().Children(ctx, ps)
	require.NoError(t, err)

	sub := filepath.Join(ps.Root, "backend", "pkg")
	require.NoError(t, os.MkdirAll(sub, 0o755))

	// The root's tmux session sets HIVE_AGENT_SESSION empty, so the working
	// directory decides.
	caller, err := f.engine.Sessions().ResolveCaller(ctx, "", "", sub)
	require.NoError(t, err)
	assert.Equal(t, children[1].Session.ID, caller.Session.ID)

	caller, err = f.engine.Sessions().ResolveCaller(ctx, "", "", filepath.Join(ps.Root, "frontend"))
	require.NoError(t, err)
	assert.Equal(t, children[0].Session.ID, caller.Session.ID)

	_, err = f.engine.Sessions().ResolveCaller(ctx, "", "", ps.Root)
	require.Error(t, err, "the root itself is no session's checkout")
}

func TestRecycleReturnsFullClonesToThePool(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	first := f.create(t, "first")
	before, err := f.engine.Projects().Children(ctx, first)
	require.NoError(t, err)

	require.NoError(t, f.engine.Projects().Recycle(ctx, first.ID, nil))

	_, err = os.Lstat(first.Root)
	assert.True(t, os.IsNotExist(err), "root removed")
	_, err = f.engine.Projects().Resolve(ctx, first.ID)
	require.ErrorIs(t, err, project.ErrNotFound)
	assert.Contains(t, f.mux.killed, first.Slug)

	frontend, err := f.engine.Sessions().GetSession(ctx, before[0].Session.ID)
	require.NoError(t, err)
	assert.Equal(t, session.StateRecycled, frontend.State)
	_, err = f.engine.Sessions().GetSession(ctx, before[1].Session.ID)
	require.Error(t, err, "a worktree child is deleted, not kept recycled")
	_, err = os.Stat(before[1].Session.Path)
	assert.True(t, os.IsNotExist(err))

	second := f.create(t, "second")
	after, err := f.engine.Projects().Children(ctx, second)
	require.NoError(t, err)
	assert.Equal(t, before[0].Session.Path, after[0].Session.Path, "the full clone is reused")
	assert.NotEqual(t, before[0].Session.ID, after[0].Session.ID)
	target, err := os.Readlink(filepath.Join(second.Root, "frontend"))
	require.NoError(t, err)
	assert.Equal(t, before[0].Session.Path, target)
}

func TestDeleteRemovesChildrenWorktreesAndRoot(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	ps := f.create(t, "gone")
	children, err := f.engine.Projects().Children(ctx, ps)
	require.NoError(t, err)

	require.NoError(t, f.engine.Projects().Delete(ctx, ps.Slug))

	for _, c := range children {
		_, err := os.Stat(c.Session.Path)
		assert.True(t, os.IsNotExist(err), "%s checkout removed", c.Dir)
		_, err = f.engine.Sessions().GetSession(ctx, c.Session.ID)
		require.Error(t, err)
	}
	_, err = os.Lstat(ps.Root)
	assert.True(t, os.IsNotExist(err))

	var bares []string
	err = filepath.WalkDir(filepath.Join(f.dataDir, "repos", ".bare"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if _, err := os.Stat(filepath.Join(path, "HEAD")); d.IsDir() && err == nil {
			bares = append(bares, path)
			return filepath.SkipDir
		}
		return nil
	})
	require.NoError(t, err)
	require.Len(t, bares, 1, "the worktree child's bare clone")
	for _, bare := range bares {
		out := run(t, bare, "git", "worktree", "list", "--porcelain")
		assert.Equal(t, 1, strings.Count(out, "worktree "), "only the bare repo is left: %s", out)
	}
}

func TestFailedCreateRemovesTheChildrenItMade(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()

	_, err := f.engine.Projects().Create(ctx, projectsvc.CreateOptions{
		Name: "broken",
		Repos: []projectsvc.RepoSpec{
			{Remote: f.frontend},
			{Remote: "file:///does/not/exist/backend.git"},
		},
		Background: true,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "check out backend")

	sessions, err := f.engine.Sessions().ListSessions(ctx)
	require.NoError(t, err)
	assert.Empty(t, sessions)
	all, err := f.engine.Projects().List(ctx)
	require.NoError(t, err)
	assert.Empty(t, all)
	entries, _ := os.ReadDir(filepath.Join(f.dataDir, "projects"))
	assert.Empty(t, entries)
	assert.Empty(t, f.mux.created)
}

func TestCreateReportsEachRepoAndTheRollback(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()

	final := map[string]projectsvc.StepState{}
	var order []string
	_, err := f.engine.Projects().Create(ctx, projectsvc.CreateOptions{
		Name: "steps",
		Repos: []projectsvc.RepoSpec{
			{Remote: f.frontend},
			{Remote: "file:///does/not/exist/backend.git"},
		},
		Background: true,
		OnStep: func(step projectsvc.Step) {
			final[step.Dir] = step.State
			order = append(order, step.Dir+"="+string(step.State))
		},
	})
	require.Error(t, err)

	assert.Equal(t, projectsvc.StepRemoved, final["frontend"], "the repo that checked out is reported removed")
	assert.Equal(t, projectsvc.StepFailed, final["backend"])
	assert.Equal(t, projectsvc.StepPending, final[""], "the root never started")
	assert.Equal(t, []string{"frontend=pending", "backend=pending", "=pending"}, order[:3], "every repo is listed before any starts")
}

func TestCreateReportsTheRootDone(t *testing.T) {
	f := newFixture(t)
	final := map[string]projectsvc.StepState{}
	_, err := f.engine.Projects().Create(t.Context(), projectsvc.CreateOptions{
		Name:       "done",
		Repos:      []projectsvc.RepoSpec{{Remote: f.frontend}},
		Background: true,
		OnStep:     func(step projectsvc.Step) { final[step.Dir] = step.State },
	})
	require.NoError(t, err)
	assert.Equal(t, map[string]projectsvc.StepState{"frontend": projectsvc.StepDone, "": projectsvc.StepDone}, final)
}

func TestCreateRejectsTakenNamesAndDirs(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	f.create(t, "same")

	_, err := f.engine.Projects().Create(ctx, projectsvc.CreateOptions{
		Name: "same", Repos: []projectsvc.RepoSpec{{Remote: f.frontend}}, Background: true,
	})
	require.ErrorIs(t, err, project.ErrDuplicateName)

	_, err = f.engine.Projects().Create(ctx, projectsvc.CreateOptions{
		Name:       "dupe dirs",
		Repos:      []projectsvc.RepoSpec{{Remote: f.frontend}, {Remote: f.frontend}},
		Background: true,
	})
	require.ErrorContains(t, err, `dir "frontend"`)

	_, err = f.engine.Projects().Create(ctx, projectsvc.CreateOptions{
		Name: "two copies", Background: true,
		Repos: []projectsvc.RepoSpec{{Remote: f.frontend, Dir: "web"}, {Remote: f.frontend, Dir: "web-old"}},
	})
	require.NoError(t, err, "the same remote twice works when the dirs differ")
}
