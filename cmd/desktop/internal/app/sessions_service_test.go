package app

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/colonyops/hive/internal/domain/session"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/platform/promptfile"
	tmuxcc "github.com/colonyops/hive/internal/platform/tmux/control"
	tmuxexec "github.com/colonyops/hive/internal/platform/tmux/exec"
	"github.com/colonyops/hive/pkg/executil/executiltest"
	"github.com/rs/zerolog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/activity"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
)

type defaultAgentFunc func(context.Context) string

func (f defaultAgentFunc) DefaultAgent(ctx context.Context) string { return f(ctx) }

type fakeSessionLauncher struct {
	calls []dispatch.LaunchSessionRequest
	err   error
}

func (f *fakeSessionLauncher) LaunchSession(_ context.Context, req dispatch.LaunchSessionRequest) (dispatch.SessionExecutionOutcome, error) {
	f.calls = append(f.calls, req)
	if f.err != nil {
		return dispatch.SessionExecutionOutcome{}, f.err
	}
	return dispatch.SessionExecutionOutcome{ID: "session-1", Name: req.Name}, nil
}

type fakeWorkspaceLauncher struct {
	options []dispatch.SessionLaunchWorkspace
	calls   []dispatch.LaunchWorkspaceSessionRequest
	err     error
}

func (f *fakeWorkspaceLauncher) SessionLaunchWorkspaces(context.Context) []dispatch.SessionLaunchWorkspace {
	return f.options
}

func (f *fakeWorkspaceLauncher) LaunchWorkspaceSession(_ context.Context, req dispatch.LaunchWorkspaceSessionRequest) (dispatch.SessionExecutionOutcome, error) {
	f.calls = append(f.calls, req)
	return dispatch.SessionExecutionOutcome{ID: "42", Name: req.Name, Slug: "agentws-42"}, f.err
}

type fakeSessionTmux struct {
	renames     [][2]string
	contextErrs []error
	absent      bool
	err         error
	// afterRename runs once the live rename is recorded, to interleave a
	// failure between it and hive's own update.
	afterRename func()
}

func (f *fakeSessionTmux) RenameSessionIfPresent(ctx context.Context, from, to string) (bool, error) {
	f.renames = append(f.renames, [2]string{from, to})
	f.contextErrs = append(f.contextErrs, ctx.Err())
	if f.afterRename != nil {
		f.afterRename()
		f.afterRename = nil
	}
	return !f.absent && f.err == nil, f.err
}

type fakeWindowSource struct {
	results map[string][]tmuxcc.IndexedWindow
	err     error
	seen    []string
}

func (f *fakeWindowSource) ListIndexedWindows(_ context.Context, sessions []string) (map[string][]tmuxcc.IndexedWindow, error) {
	f.seen = append(f.seen, sessions...)
	return f.results, f.err
}

// fakeJobRunner runs the tracked function synchronously so tests can observe
// its outcome deterministically.
type fakeJobRunner struct {
	ran      bool
	label    string
	actionID string
	target   string
	err      error
}

func (f *fakeJobRunner) Track(ctx context.Context, label, actionID, target string, fn func(context.Context) error) int64 {
	f.ran = true
	f.label, f.actionID, f.target = label, actionID, target
	f.err = fn(ctx)
	return 7
}

// Record never fails by contract, so there is nothing to simulate.
type fakeActivityRecorder struct{ events []activity.Event }

func (r *fakeActivityRecorder) Record(_ context.Context, e activity.Event) {
	r.events = append(r.events, e)
}

func activeHarness(t *testing.T) *hiveHarness {
	t.Helper()
	h := newHiveHarness(t, engineOptions{})
	h.save(t, reviewSession())
	return h
}

func (h *hiveHarness) session(t *testing.T, id string) session.Session {
	t.Helper()
	s, err := h.engine.Sessions().GetSession(t.Context(), id)
	require.NoError(t, err)
	return s
}

func TestSessionsService_SessionLaunchWorkspacesDoesNotResolveRepositories(t *testing.T) {
	workspaceOptions := []dispatch.SessionLaunchWorkspace{{Dir: "alerts", Name: "Alerts", SupportsPrompt: true}}
	svc := newSessionsService(SessionsDeps{WorkspaceLauncher: &fakeWorkspaceLauncher{options: workspaceOptions}})

	assert.Equal(t, workspaceOptions, svc.SessionLaunchWorkspaces(t.Context()))
}

func withAgents(agents ...string) func(*config.Config) {
	return func(cfg *config.Config) {
		cfg.Agents.Profiles = map[string]config.AgentProfile{}
		for _, agent := range agents {
			cfg.Agents.Profiles[agent] = config.AgentProfile{}
		}
		cfg.Agents.Default = agents[0]
	}
}

// The form gets each repository's label and remote, never its local path.
func TestSessionsService_SessionLaunchOptions(t *testing.T) {
	h := newHiveHarness(t, engineOptions{cfg: withAgents("claude")})
	checkout := t.TempDir()
	h.save(t, session.Session{
		ID: "s1", Name: "hive", Slug: "hive", Path: checkout,
		Remote: "https://github.com/colonyops/hive.git", State: session.StateActive,
	})
	workspaceOptions := []dispatch.SessionLaunchWorkspace{{Dir: "alerts", Name: "Alerts", SupportsPrompt: true}}
	svc := newSessionsService(SessionsDeps{
		Hive: h.engine, WorkspaceLauncher: &fakeWorkspaceLauncher{options: workspaceOptions},
		Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{}, DefaultAgentEnv: NopDefaultAgentReader{},
	})

	got, err := svc.SessionLaunchOptions(t.Context())
	require.NoError(t, err)
	assert.Equal(t, dispatch.SessionLaunchOptions{
		Repositories:             []dispatch.SessionLaunchRepository{{Name: "hive", Repository: "https://github.com/colonyops/hive.git"}},
		DefaultRepository:        "https://github.com/colonyops/hive.git",
		Workspaces:               workspaceOptions,
		Agents:                   []string{"claude"},
		DefaultAgent:             "claude",
		PromptFileThresholdBytes: promptfile.ThresholdBytes,
	}, got)
}

// The form preselects what hive itself would run: HIVE_DEFAULT_AGENT when it
// names a configured profile, agents.default otherwise.
func TestSessionsService_SessionLaunchOptionsPrefersTheEnvironmentAgent(t *testing.T) {
	h := newHiveHarness(t, engineOptions{cfg: withAgents("claude", "codex")})
	svc := newSessionsService(SessionsDeps{Hive: h.engine, Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{}})

	svc.defaultAgentEnv = defaultAgentFunc(func(context.Context) string { return " codex " })
	got, err := svc.SessionLaunchOptions(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "codex", got.DefaultAgent)
	assert.Equal(t, []string{"claude", "codex"}, got.Agents, "the choices themselves are hive's")

	svc.defaultAgentEnv = defaultAgentFunc(func(context.Context) string { return "aider" })
	got, err = svc.SessionLaunchOptions(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "claude", got.DefaultAgent, "an agent with no configured profile is not preselected")

	svc.defaultAgentEnv = defaultAgentFunc(func(context.Context) string { return "" })
	got, err = svc.SessionLaunchOptions(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "claude", got.DefaultAgent)
}

func TestSessionsService_CreateSessionValidatesBeforeTracking(t *testing.T) {
	launcher := &fakeSessionLauncher{}
	runner := &fakeJobRunner{}
	h := activeHarness(t)
	svc := newSessionsService(SessionsDeps{Launcher: launcher, Hive: h.engine, Tmux: &fakeSessionTmux{}, Jobs: runner})

	_, err := svc.CreateSession(t.Context(), dispatch.CreateSessionRequest{Name: "review", Prompt: "go"})
	assert.Equal(t, KindInvalid, KindOf(err), "a target is required")

	_, err = svc.CreateSession(t.Context(), dispatch.CreateSessionRequest{Repository: "r", Workspace: "alerts", Name: "review"})
	assert.Equal(t, KindInvalid, KindOf(err), "targets are mutually exclusive")

	_, err = svc.CreateSession(t.Context(), dispatch.CreateSessionRequest{Repository: "r"})
	assert.Equal(t, KindInvalid, KindOf(err), "name is required")

	_, err = svc.CreateSession(t.Context(), dispatch.CreateSessionRequest{Repository: "r", Name: "bad~name"})
	assert.Equal(t, KindInvalid, KindOf(err), "name must be valid")

	assert.False(t, runner.ran, "nothing is tracked until validation passes")
	assert.Empty(t, launcher.calls)
}

func TestSessionsService_CreateSessionLaunchesAsAJob(t *testing.T) {
	launcher := &fakeSessionLauncher{}
	runner := &fakeJobRunner{}
	h := activeHarness(t)
	svc := newSessionsService(SessionsDeps{Launcher: launcher, Hive: h.engine, Tmux: &fakeSessionTmux{}, Jobs: runner})

	jobID, err := svc.CreateSession(t.Context(), dispatch.CreateSessionRequest{
		Repository: "  https://github.com/acme/site.git  ",
		Name:       "  review-81  ",
		Prompt:     "  fix it  ",
		Agent:      " claude ",
	})
	require.NoError(t, err)
	assert.Equal(t, int64(7), jobID)
	assert.True(t, runner.ran)
	assert.Equal(t, "review-81", runner.target)
	require.NoError(t, runner.err)
	require.Len(t, launcher.calls, 1)
	assert.Equal(t, dispatch.LaunchSessionRequest{
		Name:   "review-81",
		Prompt: "fix it",
		Agent:  "claude",
		Repo:   "https://github.com/acme/site.git",
	}, launcher.calls[0])
}

func TestSessionsService_CreateSessionLaunchesAWorkspaceChat(t *testing.T) {
	repositories := &fakeSessionLauncher{}
	workspaces := &fakeWorkspaceLauncher{}
	runner := &fakeJobRunner{}
	h := activeHarness(t)
	alert := models.ItemRef{ProfileID: "p", SourceKind: "github", ExternalID: "acme/site#81"}
	items := &fakeItemSessionStore{refs: map[int64]models.ItemRef{42: alert}}
	svc := newSessionsService(SessionsDeps{
		Launcher: repositories, WorkspaceLauncher: workspaces,
		Hive: h.engine, Tmux: &fakeSessionTmux{}, Jobs: runner, Items: items, Links: items,
	})

	jobID, err := svc.CreateSession(t.Context(), dispatch.CreateSessionRequest{
		Workspace: " alerts ", Name: " incident ", Prompt: " cluster prod is down ", Agent: "ignored", ItemIDs: []int64{42},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(7), jobID)
	require.NoError(t, runner.err)
	assert.Empty(t, repositories.calls)
	assert.Equal(t, []dispatch.LaunchWorkspaceSessionRequest{{
		Workspace: "alerts", Name: "incident", Prompt: "cluster prod is down", Origins: []models.ItemRef{alert},
	}}, workspaces.calls)
}

// CreateSession resolves an unstated agent through the same env override the
// form preselects with (hay-kot/hive-desktop#438): the form and the launch it
// submits must agree on which agent runs.
func TestSessionsService_CreateSessionResolvesTheLaunchAgent(t *testing.T) {
	for _, tt := range []struct {
		name      string
		env       string
		requested string
		want      string
	}{
		{name: "the environment agent when none is requested", env: "codex", want: "codex"},
		{name: "an explicit agent over the environment", env: "codex", requested: "claude", want: "claude"},
		{name: "no agent for an environment agent with no profile", env: "aider"},
		{name: "no agent with no environment default"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := newHiveHarness(t, engineOptions{cfg: withAgents("claude", "codex")})
			launcher := &fakeSessionLauncher{}
			svc := newSessionsService(SessionsDeps{Launcher: launcher, Hive: h.engine, Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{}})
			svc.defaultAgentEnv = defaultAgentFunc(func(context.Context) string { return tt.env })

			_, err := svc.CreateSession(t.Context(), dispatch.CreateSessionRequest{Repository: "r", Name: "review-81", Agent: tt.requested})
			require.NoError(t, err)
			require.Len(t, launcher.calls, 1)
			assert.Equal(t, tt.want, launcher.calls[0].Agent, "hive resolves agents.default itself when handed no agent")
		})
	}
}

// Resolving the agent reads the profiles off the config, so the click that
// starts a session never pays for SessionLaunchOptions' workspace scan.
func TestSessionsService_CreateSessionRunsNoSubprocessToResolveTheAgent(t *testing.T) {
	// The workspace holds a repository, because a scan of an empty one runs
	// no subprocess either.
	workspace := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workspace, "repo", ".git"), 0o755))
	h := newHiveHarness(t, engineOptions{cfg: func(cfg *config.Config) {
		withAgents("claude", "codex")(cfg)
		cfg.Workspaces = []string{workspace}
	}})
	svc := newSessionsService(SessionsDeps{Launcher: &fakeSessionLauncher{}, Hive: h.engine, Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{}})
	svc.defaultAgentEnv = defaultAgentFunc(func(context.Context) string { return "codex" })

	_, err := svc.CreateSession(t.Context(), dispatch.CreateSessionRequest{Repository: "r", Name: "review-81"})
	require.NoError(t, err)
	assert.Empty(t, h.exec.Calls())
}

func TestSessionsService_CreateSessionSurfacesDuplicateNameOnTheJob(t *testing.T) {
	runner := &fakeJobRunner{}
	h := activeHarness(t)
	svc := newSessionsService(SessionsDeps{Launcher: &fakeSessionLauncher{err: session.ErrDuplicateName}, Hive: h.engine, Tmux: &fakeSessionTmux{}, Jobs: runner})

	_, err := svc.CreateSession(t.Context(), dispatch.CreateSessionRequest{Repository: "r", Name: "dupe"})
	require.NoError(t, err, "a duplicate name is a job failure, not a validation error")
	require.Error(t, runner.err)
	assert.Contains(t, runner.err.Error(), "already exists")
}

func TestSessionsService_ListSessionsPassesEveryStateThrough(t *testing.T) {
	h := activeHarness(t)
	h.save(t, session.Session{ID: "s2", Name: "old", Slug: "old", State: session.StateRecycled})
	svc := newSessionsService(SessionsDeps{Hive: h.engine, Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{}})

	got, err := svc.ListSessions(t.Context())
	require.NoError(t, err)
	states := map[string]session.State{}
	for _, s := range got {
		states[s.ID] = s.State
	}
	assert.Equal(t, map[string]session.State{"s1": session.StateActive, "s2": session.StateRecycled}, states,
		"a recycled session is unattachable, not unmanageable")
}

func TestSessionsService_SessionDetailReadsWorktreeMetadata(t *testing.T) {
	h := newHiveHarness(t, engineOptions{})
	sess := session.Session{
		ID: "s1", Name: "review 81", Slug: "review-81", Path: "/tmp/review-81",
		Remote: "acme/site", State: session.StateActive, CloneStrategy: session.CloneStrategyWorktree,
		Tags: []string{"pr-81"},
	}
	sess.SetMeta(session.MetaWorktreeBranch, "hive/review-81")
	h.save(t, sess)
	svc := newSessionsService(SessionsDeps{Hive: h.engine})

	got, err := svc.SessionDetail(t.Context(), "s1")
	require.NoError(t, err)
	assert.Equal(t, "hive/review-81", got.GetMeta(session.MetaWorktreeBranch))
	assert.Equal(t, []string{"pr-81"}, got.Tags)
	assert.Equal(t, "/tmp/review-81", got.Path)

	_, err = svc.SessionDetail(t.Context(), "gone")
	assert.Equal(t, KindNotFound, KindOf(err))
	_, err = svc.SessionDetail(t.Context(), " ")
	assert.Equal(t, KindInvalid, KindOf(err))
}

func TestSessionsService_RenameSessionRenamesTmuxBeforeTheStore(t *testing.T) {
	h := activeHarness(t)
	tmux := &fakeSessionTmux{}
	svc := newSessionsService(SessionsDeps{Hive: h.engine, Tmux: tmux, Jobs: &fakeJobRunner{}})

	got, err := svc.RenameSession(t.Context(), "s1", "  Review 82  ")
	require.NoError(t, err)
	assert.Equal(t, "Review 82", got.Name)
	assert.Equal(t, "review-82", got.Slug, "the session carries the new tmux target")
	assert.Equal(t, [][2]string{{"review-81", "review-82"}}, tmux.renames)
	assert.Equal(t, "Review 82", h.session(t, "s1").Name)
}

func TestSessionsService_RenameSessionLeavesTheStoreAloneWhenTmuxFails(t *testing.T) {
	h := activeHarness(t)
	tmux := &fakeSessionTmux{err: errors.New("duplicate session: review-82")}
	svc := newSessionsService(SessionsDeps{Hive: h.engine, Tmux: tmux, Jobs: &fakeJobRunner{}})

	_, err := svc.RenameSession(t.Context(), "s1", "review 82")
	assert.Equal(t, KindConflict, KindOf(err))
	assert.Equal(t, "review 81", h.session(t, "s1").Name, "the slug must not move without its tmux session")
}

// failSessionWrites makes hive.db refuse every later session write, which is
// how a full disk presents to the rename.
func failSessionWrites(t *testing.T, h *hiveHarness) {
	t.Helper()
	_, err := h.db.Conn().ExecContext(t.Context(),
		`CREATE TRIGGER fail_session_writes BEFORE UPDATE ON sessions BEGIN SELECT RAISE(ABORT, 'disk full'); END`)
	require.NoError(t, err)
}

func TestSessionsService_RenameSessionRollsActualTmuxTargetBackWhenTheStoreFails(t *testing.T) {
	h := newHiveHarness(t, engineOptions{})
	sess := reviewSession()
	sess.SetMeta(session.MetaTmuxSession, "actual-review-81")
	h.save(t, sess)
	failSessionWrites(t, h)
	tmux := &fakeSessionTmux{}
	svc := newSessionsService(SessionsDeps{Hive: h.engine, Tmux: tmux, Jobs: &fakeJobRunner{}})

	_, err := svc.RenameSession(t.Context(), "s1", "review 82")
	assert.Equal(t, KindInternal, KindOf(err))
	assert.Equal(t, [][2]string{{"actual-review-81", "review-82"}, {"review-82", "actual-review-81"}}, tmux.renames)
}

func TestSessionsService_RenameSessionDoesNotRollBackAnAbsentTmuxSession(t *testing.T) {
	h := activeHarness(t)
	failSessionWrites(t, h)
	tmux := &fakeSessionTmux{absent: true}
	svc := newSessionsService(SessionsDeps{Hive: h.engine, Tmux: tmux, Jobs: &fakeJobRunner{}})

	_, err := svc.RenameSession(t.Context(), "s1", "review 82")
	assert.Equal(t, KindInternal, KindOf(err))
	assert.Equal(t, [][2]string{{"review-81", "review-82"}}, tmux.renames)
}

func TestSessionsService_RenameSessionRollbackOutlivesRequestCancellation(t *testing.T) {
	h := activeHarness(t)
	ctx, cancel := context.WithCancel(t.Context())
	tmux := &fakeSessionTmux{afterRename: cancel}
	svc := newSessionsService(SessionsDeps{Hive: h.engine, Tmux: tmux, Jobs: &fakeJobRunner{}})

	_, err := svc.RenameSession(ctx, "s1", "review 82")
	assert.Equal(t, KindInternal, KindOf(err))
	require.Len(t, tmux.contextErrs, 2)
	assert.NoError(t, tmux.contextErrs[0])
	assert.NoError(t, tmux.contextErrs[1])
}

func TestSessionsService_RenameSessionRejectsASlugCollision(t *testing.T) {
	h := activeHarness(t)
	h.save(t, session.Session{ID: "s2", Name: "Review 82", Slug: "review-82", State: session.StateActive})
	tmux := &fakeSessionTmux{}
	svc := newSessionsService(SessionsDeps{Hive: h.engine, Tmux: tmux, Jobs: &fakeJobRunner{}})

	// "review/82" slugifies onto s2's slug, which would give both sessions the
	// same tmux session name and the same directory slug.
	_, err := svc.RenameSession(t.Context(), "s1", "review/82")
	assert.Equal(t, KindInvalid, KindOf(err))
	assert.Empty(t, tmux.renames)
	assert.Equal(t, "review 81", h.session(t, "s1").Name)
}

func TestSessionsService_RenameSessionRejectsAPersistedTmuxTargetCollision(t *testing.T) {
	h := activeHarness(t)
	renamed := session.Session{ID: "s2", Name: "Renamed Elsewhere", Slug: "renamed-elsewhere", State: session.StateActive}
	renamed.SetMeta(session.MetaTmuxSession, "review-82")
	h.save(t, renamed)
	tmux := &fakeSessionTmux{}
	svc := newSessionsService(SessionsDeps{Hive: h.engine, Tmux: tmux, Jobs: &fakeJobRunner{}})

	_, err := svc.RenameSession(t.Context(), "s1", "review 82")
	assert.Equal(t, KindInvalid, KindOf(err))
	assert.Empty(t, tmux.renames)
	assert.Equal(t, "review 81", h.session(t, "s1").Name)
}

func TestSessionsService_RenameSessionValidatesTheName(t *testing.T) {
	h := activeHarness(t)
	tmux := &fakeSessionTmux{}
	svc := newSessionsService(SessionsDeps{Hive: h.engine, Tmux: tmux, Jobs: &fakeJobRunner{}})

	_, err := svc.RenameSession(t.Context(), "s1", "  ")
	assert.Equal(t, KindInvalid, KindOf(err))
	_, err = svc.RenameSession(t.Context(), "s1", "bad~name")
	assert.Equal(t, KindInvalid, KindOf(err))
	assert.Empty(t, tmux.renames)
}

func TestSessionsService_DeleteAndRecycleRunAsJobsLabelledWithTheSessionName(t *testing.T) {
	for _, tt := range []struct {
		name     string
		call     func(*SessionsService, context.Context) (int64, error)
		label    string
		actionID string
		after    func(*testing.T, *hiveHarness)
	}{
		{"delete", func(s *SessionsService, ctx context.Context) (int64, error) { return s.DeleteSession(ctx, "s1") }, "Delete session", deleteSessionJobActionID, func(t *testing.T, h *hiveHarness) {
			_, err := h.engine.Sessions().GetSession(t.Context(), "s1")
			assert.Error(t, err, "the record is gone")
		}},
		{"recycle", func(s *SessionsService, ctx context.Context) (int64, error) { return s.RecycleSession(ctx, "s1") }, "Recycle session", recycleSessionJobActionID, func(t *testing.T, h *hiveHarness) {
			assert.Equal(t, session.StateRecycled, h.session(t, "s1").State)
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := newHiveHarness(t, engineOptions{})
			sess := reviewSession()
			sess.Path = t.TempDir()
			require.NoError(t, os.Mkdir(filepath.Join(sess.Path, ".git"), 0o755))
			h.save(t, sess)
			runner := &fakeJobRunner{}
			svc := newSessionsService(SessionsDeps{Hive: h.engine, Tmux: &fakeSessionTmux{}, Jobs: runner})

			jobID, err := tt.call(svc, t.Context())
			require.NoError(t, err)
			require.NoError(t, runner.err)
			assert.Equal(t, int64(7), jobID)
			assert.Equal(t, tt.label, runner.label)
			assert.Equal(t, tt.actionID, runner.actionID)
			assert.Equal(t, "review 81", runner.target, "the name is read before the session can stop existing")
			tt.after(t, h)
		})
	}
}

func TestSessionsService_DestructiveOperationsRejectAnUnknownSession(t *testing.T) {
	h := activeHarness(t)
	runner := &fakeJobRunner{}
	svc := newSessionsService(SessionsDeps{Hive: h.engine, Tmux: &fakeSessionTmux{}, Jobs: runner})

	_, err := svc.DeleteSession(t.Context(), "gone")
	assert.Equal(t, KindNotFound, KindOf(err))
	_, err = svc.RecycleSession(t.Context(), "gone")
	assert.Equal(t, KindNotFound, KindOf(err))
	assert.False(t, runner.ran)
}

func TestSessionsService_PruneRunsAsAJobAndDeletesEveryRecycledSession(t *testing.T) {
	h := activeHarness(t)
	h.save(t,
		session.Session{ID: "s2", Name: "old", Slug: "old", Path: t.TempDir(), State: session.StateRecycled},
		session.Session{ID: "s3", Name: "broken", Slug: "broken", Path: t.TempDir(), State: session.StateCorrupted},
	)
	runner := &fakeJobRunner{}
	svc := newSessionsService(SessionsDeps{Hive: h.engine, Tmux: &fakeSessionTmux{}, Jobs: runner})

	jobID, err := svc.PruneSessions(t.Context())
	require.NoError(t, err)
	require.NoError(t, runner.err)
	assert.Equal(t, int64(7), jobID)
	assert.Equal(t, pruneSessionsJobActionID, runner.actionID)
	got, err := svc.ListSessions(t.Context())
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "s1", got[0].ID)
}

// The delete and recycle confirmations warn about unsaved work, so the risk of
// a live checkout has to reach the caller. A checkout that git cannot read
// counts as dirty.
func TestSessionsService_SessionRiskReportsUnsavedWork(t *testing.T) {
	for _, tt := range []struct {
		name      string
		responses []executiltest.Response // git status, then the unpushed count
		want      SessionRisk
	}{
		{"dirty with unpushed commits", []executiltest.Response{{Out: []byte(" M README.md\n")}, {Out: []byte("2\n")}}, SessionRisk{UncommittedChanges: true, UnpushedCommits: true}},
		{"clean and pushed", []executiltest.Response{{}, {Out: []byte("0\n")}}, SessionRisk{}},
		{"unreadable checkout", []executiltest.Response{{Err: errors.New("not a git repository")}, {Out: []byte("0\n")}}, SessionRisk{UncommittedChanges: true}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := newHiveHarness(t, engineOptions{})
			h.exec.Responses = tt.responses
			sess := reviewSession()
			sess.Path = "/tmp/review-81"
			h.save(t, sess)
			svc := newSessionsService(SessionsDeps{Hive: h.engine, Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{}})

			risk, err := svc.SessionRisk(t.Context(), "s1")
			require.NoError(t, err)
			assert.Equal(t, tt.want, risk)
		})
	}
}

func TestSessionsService_SessionRiskCarriesTheWorktreeRecycleWarning(t *testing.T) {
	h := newHiveHarness(t, engineOptions{})
	h.save(t, session.Session{ID: "s1", Name: "old", Slug: "old", Path: "/tmp/old", State: session.StateRecycled, CloneStrategy: session.CloneStrategyWorktree})
	svc := newSessionsService(SessionsDeps{Hive: h.engine, Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{}})

	risk, err := svc.SessionRisk(t.Context(), "s1")
	require.NoError(t, err)
	// No live clone left to hold unsaved work, but recycling a worktree
	// session still deletes it, which the confirmation has to say.
	assert.Equal(t, SessionRisk{RecycleDeletes: true}, risk)

	_, err = svc.SessionRisk(t.Context(), " ")
	assert.Equal(t, KindInvalid, KindOf(err))
	_, err = svc.SessionRisk(t.Context(), "gone")
	assert.Equal(t, KindNotFound, KindOf(err))
}

func TestSessionsService_SessionGitStatusReadsTheCheckout(t *testing.T) {
	h := newHiveHarness(t, engineOptions{})
	h.exec.Responses = []executiltest.Response{
		{Out: []byte("feat/bar\n")},
		{Out: []byte(" M README.md\n")},
	}
	sess := reviewSession()
	sess.Path = "/tmp/review-81"
	sess.Remote = "git@github.com:acme/site.git"
	h.save(t, sess)
	svc := newSessionsService(SessionsDeps{Hive: h.engine})

	got, err := svc.SessionGitStatus(t.Context(), "s1")
	require.NoError(t, err)
	assert.True(t, got.Resolved)
	assert.Equal(t, "feat/bar", got.Branch)
	assert.True(t, got.Dirty)
	assert.Equal(t, [3]string{"github.com", "acme", "site"}, [3]string{got.Host, got.Owner, got.Repo})

	_, err = svc.SessionGitStatus(t.Context(), "gone")
	assert.Equal(t, KindNotFound, KindOf(err))
}

func TestSessionsService_StartTmuxSessionSpawnsFromTheSessionsOwnCheckout(t *testing.T) {
	h := newHiveHarness(t, engineOptions{})
	sess := reviewSession()
	sess.Path = "/repos/site-wt-ab12"
	h.save(t, sess)
	svc := newSessionsService(SessionsDeps{Hive: h.engine, Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{}})

	require.NoError(t, svc.StartTmuxSession(t.Context(), "review-81"))
	require.Len(t, h.mux.opened, 1, "the spawn is hive's, so it gets the name, checkout and remote hive spawns from")
	assert.Equal(t, "review-81", h.mux.opened[0].Target.Session)
	assert.Equal(t, "/repos/site-wt-ab12", h.mux.opened[0].WorkingDirectory)
	assert.True(t, h.mux.opened[0].Background, "the desktop attaches over control mode; the spawn must stay detached")
}

// recordingTmux stands in for the tmux binary hive's own client drives, so the
// spawn is checked against the real shared spawner with no tmux server
// running. absent fails has-session, which is how tmux answers for a session it
// does not hold.
type recordingTmux struct {
	absent bool
	runs   [][]string
}

func (r *recordingTmux) record(args []string) error {
	r.runs = append(r.runs, append([]string{"tmux"}, args...))
	if r.absent && len(args) > 0 && args[0] == "has-session" {
		return errors.New("can't find session")
	}
	return nil
}

func (*recordingTmux) Available() bool { return true }

func (r *recordingTmux) Capture(_ context.Context, args ...string) ([]byte, []byte, error) {
	return nil, nil, r.record(args)
}

func (r *recordingTmux) Input(_ context.Context, _ io.Reader, args ...string) ([]byte, []byte, error) {
	return nil, nil, r.record(args)
}

func (r *recordingTmux) Interactive(_ context.Context, _ multiplexer.AttachStreams, args ...string) error {
	return r.record(args)
}

// spawnRule is a rule whose windows are distinguishable from hive's defaults,
// so a test can tell "hive rendered the configured spawn" from "something here
// built a window set of its own".
func spawnRule(cfg *config.Config) {
	cfg.Rules = []config.Rule{{
		Windows: []config.WindowConfig{
			{Name: "agent", Command: "run {{ .Slug }}", Focus: true},
			{Name: "shell"},
		},
	}}
}

func TestSessionsService_StartTmuxSessionSpawnsTheConfiguredWindowsDetached(t *testing.T) {
	runner := &recordingTmux{absent: true}
	h := newHiveHarness(t, engineOptions{cfg: spawnRule, mux: tmuxexec.New(zerolog.Nop(), runner)})
	sess := reviewSession()
	sess.Path = "/tmp/review-81"
	h.save(t, sess)
	svc := newSessionsService(SessionsDeps{Hive: h.engine})

	require.NoError(t, svc.StartTmuxSession(t.Context(), "review-81"))

	assert.Contains(t, runner.runs, []string{"tmux", "has-session", "-t", "=review-81"})
	assert.Contains(t, runner.runs, []string{"tmux", "new-session", "-d", "-s", "review-81", "-n", "agent", "-P", "-F", "#{pane_id}", "-c", "/tmp/review-81", "--", "cat"})
	assert.Contains(t, runner.runs, []string{"tmux", "respawn-pane", "-k", "-t", "=review-81:agent", "-c", "/tmp/review-81", "--", "sh", "-c", "run review-81"})
	assert.Contains(t, runner.runs, []string{"tmux", "new-window", "-t", "=review-81:", "-n", "shell", "-P", "-F", "#{pane_id}", "-c", "/tmp/review-81"})
	for _, run := range runner.runs {
		assert.NotContains(t, run, "attach-session", "the desktop attaches over control mode; the spawn must stay detached")
		assert.NotContains(t, run, "switch-client")
	}
}

type failedCommandTmux struct{}

func (failedCommandTmux) Available() bool { return true }

func (failedCommandTmux) Capture(_ context.Context, args ...string) ([]byte, []byte, error) {
	switch args[0] {
	case "has-session":
		return nil, nil, errors.New("can't find session")
	case "new-session":
		return []byte("%0\n"), nil, nil
	case "list-panes":
		return []byte("%0 1 127\n"), nil, nil
	case "capture-pane":
		return []byte("sh: missing-agent: command not found\nPane is dead (status 127)\n"), nil, nil
	default:
		return nil, nil, nil
	}
}

func (f failedCommandTmux) Input(ctx context.Context, _ io.Reader, args ...string) ([]byte, []byte, error) {
	return f.Capture(ctx, args...)
}

func (f failedCommandTmux) Interactive(ctx context.Context, _ multiplexer.AttachStreams, args ...string) error {
	_, _, err := f.Capture(ctx, args...)
	return err
}

func TestSessionsService_StartTmuxSessionReturnsCommandFailureForDesktop(t *testing.T) {
	h := newHiveHarness(t, engineOptions{
		cfg: func(cfg *config.Config) {
			cfg.Rules = []config.Rule{{Windows: []config.WindowConfig{{Name: "agent", Command: "missing-agent"}}}}
		},
		mux: tmuxexec.New(zerolog.Nop(), failedCommandTmux{}),
	})
	h.save(t, reviewSession())
	svc := newSessionsService(SessionsDeps{Hive: h.engine})

	err := svc.StartTmuxSession(t.Context(), "review-81")
	require.Error(t, err)
	assert.Equal(t, KindUnavailable, KindOf(err))
	assert.Contains(t, err.Error(), `command "missing-agent" was not found while starting window "agent"`)
	assert.Contains(t, err.Error(), "sh: missing-agent: command not found")
	var appErr *Error
	require.ErrorAs(t, err, &appErr)
	assert.Contains(t, appErr.Msg, "status 127")
	assert.Contains(t, appErr.Msg, "sh: missing-agent: command not found")
}

func TestSessionsService_StartTmuxSessionLeavesALiveSessionAlone(t *testing.T) {
	runner := &recordingTmux{}
	h := newHiveHarness(t, engineOptions{cfg: spawnRule, mux: tmuxexec.New(zerolog.Nop(), runner)})
	sess := reviewSession()
	sess.Path = "/tmp/review-81"
	h.save(t, sess)
	svc := newSessionsService(SessionsDeps{Hive: h.engine})

	require.NoError(t, svc.StartTmuxSession(t.Context(), "review-81"))
	assert.Equal(t, [][]string{{"tmux", "has-session", "-t", "=review-81"}}, runner.runs,
		"a session tmux already holds is not respawned, so every cold attach can ask for one")
}

func TestSessionsService_StartTmuxSessionRejectsASlugNoSessionCarries(t *testing.T) {
	h := activeHarness(t)
	svc := newSessionsService(SessionsDeps{Hive: h.engine, Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{}})

	// A tmux session made by hand is attachable, but there is nothing to
	// create one from when it is gone.
	assert.Equal(t, KindNotFound, KindOf(svc.StartTmuxSession(t.Context(), "hand-rolled")))
	assert.Equal(t, KindInvalid, KindOf(svc.StartTmuxSession(t.Context(), "  ")))
	assert.Empty(t, h.mux.opened)
}

func TestSessionsService_StartTmuxSessionRefusesASessionWithNoCheckout(t *testing.T) {
	h := newHiveHarness(t, engineOptions{})
	h.save(t, session.Session{ID: "s2", Name: "old", Slug: "old", Remote: "acme/site", State: session.StateRecycled})
	svc := newSessionsService(SessionsDeps{Hive: h.engine, Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{}})

	assert.Equal(t, KindConflict, KindOf(svc.StartTmuxSession(t.Context(), "old")))
	assert.Empty(t, h.mux.opened, "a recycled session's directory is gone; a terminal in it would be one too")
}

func TestSessionsService_StartTmuxSessionRefusesASlugItsNameWouldNotSpawn(t *testing.T) {
	// Hive spawns under the slug it derives from the name, so a record whose two
	// have drifted apart would create a session under a name nothing attaches to.
	h := newHiveHarness(t, engineOptions{})
	h.save(t, session.Session{ID: "s1", Name: "review 82", Slug: "review-81", Remote: "acme/site", State: session.StateActive})
	svc := newSessionsService(SessionsDeps{Hive: h.engine, Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{}})

	assert.Equal(t, KindConflict, KindOf(svc.StartTmuxSession(t.Context(), "review-81")))
	assert.Empty(t, h.mux.opened)
}

// A nil reader in SessionsDeps is substituted at construction, so the service
// never guards either port; each no-op must answer the empty value rather
// than silently succeeding at the real work it stands in for.

func TestNewSessionsService_SubstitutesNopReadersForNil(t *testing.T) {
	svc := newSessionsService(SessionsDeps{})
	assert.Equal(t, NopEditorCommandReader{}, svc.editorCommand)
	assert.Equal(t, NopDefaultAgentReader{}, svc.defaultAgentEnv)
}

func TestNopDefaultAgentReaderAnswersNoPreferredAgent(t *testing.T) {
	assert.Empty(t, NopDefaultAgentReader{}.DefaultAgent(t.Context()))
}

func TestNopEditorCommandReaderAnswersNoConfiguredEditor(t *testing.T) {
	command, err := NopEditorCommandReader{}.Editor(t.Context())
	require.NoError(t, err)
	assert.Empty(t, command)
}

func TestSessionsService_CreateSessionKeepsTheFormWhenCreationFails(t *testing.T) {
	failure := &sessionsvc.LaunchError{
		Name:          "review-81",
		Remote:        "https://github.com/acme/site.git",
		CloneStrategy: "full",
		Step:          "Cloning repository...",
		Output:        "Clone strategy: full\nCloning repository...",
		Err:           errors.New("clone repository: git clone: exec git: exit status 1"),
	}
	h := activeHarness(t)
	bus := newTestBus(t)
	failed := subscribeEvents[events.SessionCreateFailed](t, bus)
	items := &fakeItemSessionStore{refs: map[int64]models.ItemRef{42: {ProfileID: "p", SourceKind: "github", ExternalID: "acme/site#81"}}}
	svc := newSessionsService(SessionsDeps{
		Launcher: &fakeSessionLauncher{err: failure}, Hive: h.engine,
		Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{}, Items: items, Links: items, Events: bus,
	})

	_, err := svc.CreateSession(t.Context(), dispatch.CreateSessionRequest{
		Repository: " https://github.com/acme/site.git ",
		Name:       " review-81 ",
		Prompt:     " fix the clone ",
		Agent:      "claude",
		ItemIDs:    []int64{42},
	})
	require.NoError(t, err, "the create is a job, so its failure is not a validation error")

	assert.Equal(t, []events.SessionCreateFailed{{Name: "review-81"}}, requireEvents(t, failed, 1))

	draft, err := svc.FailedSessionDraft(t.Context())
	require.NoError(t, err)
	require.NotNil(t, draft.Failure)
	assert.Equal(t, "https://github.com/acme/site.git", draft.Repository)
	assert.Equal(t, "review-81", draft.Name)
	assert.Equal(t, "fix the clone", draft.Prompt)
	assert.Equal(t, "claude", draft.Agent)
	assert.Equal(t, []int64{42}, draft.ItemIDs, "a retry re-links to the items the form was drafted from")
	assert.Equal(t, "clone repository: git clone: exec git: exit status 1", draft.Failure.Reason)
	assert.Equal(t, "Cloning repository...", draft.Failure.Step)
	assert.Equal(t, "Clone strategy: full\nCloning repository...", draft.Failure.Output)
	assert.Equal(t, "full", draft.Failure.CloneStrategy)
	assert.False(t, draft.Failure.At.IsZero())
}

// An untyped error is the reason, and the rest is absent rather than invented.
func TestSessionsService_CreateSessionKeepsTheFormForAnUntypedFailure(t *testing.T) {
	h := activeHarness(t)
	svc := newSessionsService(SessionsDeps{
		Launcher: &fakeSessionLauncher{err: errors.New("tmux unavailable")}, Hive: h.engine,
		Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{},
	})

	_, err := svc.CreateSession(t.Context(), dispatch.CreateSessionRequest{Repository: "r", Name: "review-81"})
	require.NoError(t, err)

	draft, err := svc.FailedSessionDraft(t.Context())
	require.NoError(t, err)
	require.NotNil(t, draft.Failure)
	assert.Equal(t, "tmux unavailable", draft.Failure.Reason)
	assert.Empty(t, draft.Failure.Step)
}

func TestSessionsService_FailedSessionDraftIsEmptyUntilSomethingFails(t *testing.T) {
	h := activeHarness(t)
	svc := newSessionsService(SessionsDeps{
		Launcher: &fakeSessionLauncher{}, Hive: h.engine,
		Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{},
	})

	draft, err := svc.FailedSessionDraft(t.Context())
	require.NoError(t, err)
	assert.Nil(t, draft.Failure, "nothing has failed, so there is nothing to restore")

	_, err = svc.CreateSession(t.Context(), dispatch.CreateSessionRequest{Repository: "r", Name: "review-81"})
	require.NoError(t, err)
	draft, err = svc.FailedSessionDraft(t.Context())
	require.NoError(t, err)
	assert.Nil(t, draft.Failure, "a create that worked leaves nothing pending")
}

func TestSessionsService_SubmittingAgainRetiresThePendingFailure(t *testing.T) {
	launcher := &fakeSessionLauncher{err: errors.New("clone repository: git clone: exec git: exit status 1")}
	h := activeHarness(t)
	svc := newSessionsService(SessionsDeps{
		Launcher: launcher, Hive: h.engine,
		Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{},
	})

	_, err := svc.CreateSession(t.Context(), dispatch.CreateSessionRequest{Repository: "r", Name: "review-81"})
	require.NoError(t, err)
	draft, err := svc.FailedSessionDraft(t.Context())
	require.NoError(t, err)
	require.NotNil(t, draft.Failure)

	launcher.err = nil
	_, err = svc.CreateSession(t.Context(), dispatch.CreateSessionRequest{Repository: "r", Name: "review-81"})
	require.NoError(t, err)
	draft, err = svc.FailedSessionDraft(t.Context())
	require.NoError(t, err)
	assert.Nil(t, draft.Failure, "the retry worked, so the form has nothing left to restore")
}

func TestSessionsService_DismissFailedSessionClearsIt(t *testing.T) {
	h := activeHarness(t)
	svc := newSessionsService(SessionsDeps{
		Launcher: &fakeSessionLauncher{err: errors.New("nope")}, Hive: h.engine,
		Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{},
	})
	_, err := svc.CreateSession(t.Context(), dispatch.CreateSessionRequest{Repository: "r", Name: "review-81"})
	require.NoError(t, err)

	require.NoError(t, svc.DismissFailedSession(t.Context()))
	draft, err := svc.FailedSessionDraft(t.Context())
	require.NoError(t, err)
	assert.Nil(t, draft.Failure)
}

// The form is still open and the field is what is wrong, so handing the whole
// attempt back would replace an editable error with a retry.
func TestSessionsService_DuplicateNameIsNotAPendingFailure(t *testing.T) {
	h := activeHarness(t)
	svc := newSessionsService(SessionsDeps{
		Launcher: &fakeSessionLauncher{err: session.ErrDuplicateName}, Hive: h.engine,
		Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{},
	})

	_, err := svc.CreateSession(t.Context(), dispatch.CreateSessionRequest{Repository: "r", Name: "dupe"})
	require.NoError(t, err)
	draft, err := svc.FailedSessionDraft(t.Context())
	require.NoError(t, err)
	assert.Nil(t, draft.Failure)
}

// The row is the half that survives a restart, so it carries the form.
func TestSessionsService_CreateSessionRecordsARetryableActivityRow(t *testing.T) {
	h := activeHarness(t)
	recorder := &fakeActivityRecorder{}
	svc := newSessionsService(SessionsDeps{
		Launcher: &fakeSessionLauncher{err: &sessionsvc.LaunchError{
			Step: "Cloning repository...",
			Err:  errors.New("clone repository: git clone: exec git: exit status 1"),
		}},
		Hive: h.engine, Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{},
		Recorder: recorder,
	})

	_, err := svc.CreateSession(t.Context(), dispatch.CreateSessionRequest{
		Repository: "https://github.com/acme/site.git", Name: "fix-crash", Prompt: "Fix the crash", Agent: "claude",
	})
	require.NoError(t, err)

	require.Len(t, recorder.events, 1)
	row := recorder.events[0]
	assert.Equal(t, activity.CategorySession, row.Category)
	assert.Equal(t, activity.SeverityError, row.Severity)
	assert.Contains(t, row.Title, "fix-crash")
	assert.Contains(t, row.Body, "Cloning repository...")
	assert.Contains(t, row.Body, "exit status 1")

	restored, err := svc.SessionDraftFromActivity(t.Context(), row.Metadata)
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/acme/site.git", restored.Repository)
	assert.Equal(t, "fix-crash", restored.Name)
	assert.Equal(t, "Fix the crash", restored.Prompt)
	assert.Equal(t, "claude", restored.Agent)
	require.NotNil(t, restored.Failure)
	assert.Equal(t, "Cloning repository...", restored.Failure.Step)
}

func TestSessionsService_WorkspaceFailureRecordsARetryableDraft(t *testing.T) {
	h := activeHarness(t)
	recorder := &fakeActivityRecorder{}
	items := &fakeItemSessionStore{}
	svc := newSessionsService(SessionsDeps{
		Launcher: &fakeSessionLauncher{}, WorkspaceLauncher: &fakeWorkspaceLauncher{err: errors.New("agent exited")},
		Hive: h.engine, Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{}, Recorder: recorder,
		Items: items, Links: items,
	})

	_, err := svc.CreateSession(t.Context(), dispatch.CreateSessionRequest{
		Workspace: "alerts", Name: "triage-alert", Prompt: "Triage the alert", ItemIDs: []int64{41, 42},
	})
	require.NoError(t, err)

	draft, err := svc.FailedSessionDraft(t.Context())
	require.NoError(t, err)
	assert.Empty(t, draft.Repository)
	assert.Equal(t, "alerts", draft.Workspace)
	assert.Equal(t, "triage-alert", draft.Name)
	assert.Equal(t, "Triage the alert", draft.Prompt)
	assert.Equal(t, []int64{41, 42}, draft.ItemIDs)
	require.NotNil(t, draft.Failure)
	assert.Equal(t, "agent exited", draft.Failure.Reason)

	require.Len(t, recorder.events, 1)
	restored, err := svc.SessionDraftFromActivity(t.Context(), recorder.events[0].Metadata)
	require.NoError(t, err)
	assert.Empty(t, restored.Repository)
	assert.Equal(t, "alerts", restored.Workspace)
	assert.Equal(t, "triage-alert", restored.Name)
	assert.Equal(t, "Triage the alert", restored.Prompt)
	assert.Equal(t, []int64{41, 42}, restored.ItemIDs)
}

func TestSessionsService_CreateSessionRecordsNoFailureRowWhenItWorks(t *testing.T) {
	h := activeHarness(t)
	recorder := &fakeActivityRecorder{}
	svc := newSessionsService(SessionsDeps{
		Launcher: &fakeSessionLauncher{}, Hive: h.engine,
		Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{}, Recorder: recorder,
	})

	_, err := svc.CreateSession(t.Context(), dispatch.CreateSessionRequest{Repository: "r", Name: "fix-crash"})
	require.NoError(t, err)
	assert.Empty(t, recorder.events, "the launcher records the success; this path records only failures")
}

func TestSessionsService_SessionDraftFromActivityRefusesAnUnrelatedRow(t *testing.T) {
	h := activeHarness(t)
	svc := newSessionsService(SessionsDeps{
		Launcher: &fakeSessionLauncher{}, Hive: h.engine,
		Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{},
	})

	_, err := svc.SessionDraftFromActivity(t.Context(), map[string]string{"rule": "auto-triage"})
	assert.Equal(t, KindInvalid, KindOf(err), "a row with nothing to retry is refused, not silently empty")
}
