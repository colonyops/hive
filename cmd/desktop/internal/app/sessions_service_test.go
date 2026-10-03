package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/domain/session"

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
	opts      dispatch.SessionLaunchOptions
	calls     []dispatch.LaunchSessionRequest
	optsCalls int
	err       error
}

func (f *fakeSessionLauncher) LaunchSession(_ context.Context, req dispatch.LaunchSessionRequest) (dispatch.SessionExecutionOutcome, error) {
	f.calls = append(f.calls, req)
	if f.err != nil {
		return dispatch.SessionExecutionOutcome{}, f.err
	}
	return dispatch.SessionExecutionOutcome{ID: "session-1", Name: req.Name}, nil
}

func (f *fakeSessionLauncher) SessionLaunchOptions(context.Context) (dispatch.SessionLaunchOptions, error) {
	f.optsCalls++
	return f.opts, nil
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

// fakeSessionManager stands in for the hive seam. details is keyed by session
// id; sessions is what the list returns.
type fakeSessionManager struct {
	sessions []dispatch.SessionSummary
	statuses dispatch.SessionStatusSnapshot
	details  map[string]dispatch.SessionDetail
	gitByID  map[string]dispatch.SessionGitStatus
	risk     dispatch.SessionRisk
	running  map[string]bool
	err      error
	// runningErr fails only the liveness probe, which is how tmux being
	// unreachable presents behind a hive listing that succeeded.
	runningErr error

	renamed     [][2]string
	deleted     []string
	recycled    []string
	pruned      int
	renameErr   error
	afterRename func()
	spawned     [][3]string
	spawnErr    error
}

func (f *fakeSessionManager) ListSessions(context.Context) ([]dispatch.SessionSummary, error) {
	return f.sessions, f.err
}

func (f *fakeSessionManager) SessionStatuses(context.Context) (dispatch.SessionStatusSnapshot, error) {
	return f.statuses, f.err
}

func (f *fakeSessionManager) SessionDetail(_ context.Context, id string) (dispatch.SessionDetail, error) {
	detail, ok := f.details[id]
	if !ok {
		return dispatch.SessionDetail{}, errors.New("no such session")
	}
	return detail, nil
}

func (f *fakeSessionManager) SessionGitStatus(_ context.Context, id string) (dispatch.SessionGitStatus, error) {
	status, ok := f.gitByID[id]
	if !ok {
		return dispatch.SessionGitStatus{}, errors.New("no such session")
	}
	return status, nil
}

func (f *fakeSessionManager) RunningSessions(_ context.Context, ids []string) (map[string]bool, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.runningErr != nil {
		return nil, f.runningErr
	}
	running := map[string]bool{}
	for _, id := range ids {
		running[id] = f.running[id]
	}
	return running, nil
}

func (f *fakeSessionManager) SessionRisk(context.Context, string) (dispatch.SessionRisk, error) {
	return f.risk, f.err
}

func (f *fakeSessionManager) RenameSession(_ context.Context, id, name string) error {
	if f.afterRename != nil {
		f.afterRename()
	}
	if f.renameErr != nil {
		return f.renameErr
	}
	f.renamed = append(f.renamed, [2]string{id, name})
	detail := f.details[id]
	detail.Name = name
	detail.Slug = session.Slugify(name)
	f.details[id] = detail
	return nil
}

func (f *fakeSessionManager) DeleteSession(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	return f.err
}

func (f *fakeSessionManager) RecycleSession(_ context.Context, id string) error {
	f.recycled = append(f.recycled, id)
	return f.err
}

func (f *fakeSessionManager) PruneSessions(context.Context) (int, error) {
	f.pruned++
	return 3, f.err
}

func (f *fakeSessionManager) SpawnTmuxSession(_ context.Context, name, path, repo string) error {
	if f.spawnErr != nil {
		return f.spawnErr
	}
	f.spawned = append(f.spawned, [3]string{name, path, repo})
	return nil
}

type fakeSessionTmux struct {
	renames     [][2]string
	contextErrs []error
	absent      bool
	err         error
}

func (f *fakeSessionTmux) RenameSessionIfPresent(ctx context.Context, from, to string) (bool, error) {
	f.renames = append(f.renames, [2]string{from, to})
	f.contextErrs = append(f.contextErrs, ctx.Err())
	return !f.absent && f.err == nil, f.err
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

func activeSession() (*fakeSessionManager, dispatch.SessionDetail) {
	detail := dispatch.SessionDetail{ID: "s1", Name: "review 81", Slug: "review-81", Repo: "acme/site", State: "active"}
	return &fakeSessionManager{
		sessions: []dispatch.SessionSummary{{ID: "s1", Name: detail.Name, Slug: detail.Slug, Repo: detail.Repo, State: "active"}},
		details:  map[string]dispatch.SessionDetail{"s1": detail},
	}, detail
}

func TestSessionsService_SessionLaunchWorkspacesDoesNotResolveRepositories(t *testing.T) {
	repositories := &fakeSessionLauncher{}
	workspaceOptions := []dispatch.SessionLaunchWorkspace{{Dir: "alerts", Name: "Alerts", SupportsPrompt: true}}
	svc := newSessionsService(SessionsDeps{
		Launcher: repositories, WorkspaceLauncher: &fakeWorkspaceLauncher{options: workspaceOptions},
	})

	assert.Equal(t, workspaceOptions, svc.SessionLaunchWorkspaces(t.Context()))
	assert.Zero(t, repositories.optsCalls)
}

func TestSessionsService_SessionLaunchOptions(t *testing.T) {
	expected := dispatch.SessionLaunchOptions{
		Repositories:      []dispatch.SessionLaunchRepository{{Name: "hive", Repository: "https://github.com/colonyops/hive.git"}},
		DefaultRepository: "https://github.com/colonyops/hive.git",
		Agents:            []string{"claude"},
		DefaultAgent:      "claude",
	}
	manager, _ := activeSession()
	workspaceOptions := []dispatch.SessionLaunchWorkspace{{Dir: "alerts", Name: "Alerts", SupportsPrompt: true}}
	svc := newSessionsService(SessionsDeps{
		Launcher: &fakeSessionLauncher{opts: expected}, WorkspaceLauncher: &fakeWorkspaceLauncher{options: workspaceOptions},
		Manager: manager, Statuses: manager, Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{}, DefaultAgentEnv: NopDefaultAgentReader{},
	})
	got, err := svc.SessionLaunchOptions(t.Context())
	require.NoError(t, err)
	expected.Workspaces = workspaceOptions
	assert.Equal(t, expected, got)
}

// The form preselects what hive itself would run: HIVE_DEFAULT_AGENT when it
// names a configured profile, agents.default otherwise.
func TestSessionsService_SessionLaunchOptionsPrefersTheEnvironmentAgent(t *testing.T) {
	opts := dispatch.SessionLaunchOptions{Agents: []string{"claude", "codex"}, DefaultAgent: "claude"}
	manager, _ := activeSession()
	svc := newSessionsService(SessionsDeps{Launcher: &fakeSessionLauncher{opts: opts}, Manager: manager, Statuses: manager, Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{}})

	svc.defaultAgentEnv = defaultAgentFunc(func(context.Context) string { return " codex " })
	got, err := svc.SessionLaunchOptions(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "codex", got.DefaultAgent)
	assert.Equal(t, opts.Agents, got.Agents, "the choices themselves are hive's")

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
	manager, _ := activeSession()
	svc := newSessionsService(SessionsDeps{Launcher: launcher, Manager: manager, Statuses: manager, Tmux: &fakeSessionTmux{}, Jobs: runner})

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
	manager, _ := activeSession()
	svc := newSessionsService(SessionsDeps{Launcher: launcher, Manager: manager, Statuses: manager, Tmux: &fakeSessionTmux{}, Jobs: runner})

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
	manager, _ := activeSession()
	alert := models.ItemRef{ProfileID: "p", SourceKind: "github", ExternalID: "acme/site#81"}
	items := &fakeItemSessionStore{refs: map[int64]models.ItemRef{42: alert}}
	svc := newSessionsService(SessionsDeps{
		Launcher: repositories, WorkspaceLauncher: workspaces,
		Manager: manager, Statuses: manager, Tmux: &fakeSessionTmux{}, Jobs: runner, Items: items, Links: items,
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
	assert.Zero(t, repositories.optsCalls, "a workspace command selects its own agent")
}

// CreateSession resolves an unstated agent through the same env override the
// form preselects with (hay-kot/hive-desktop#438): the form and the launch it submits must agree
// on which agent runs.
func TestSessionsService_CreateSessionResolvesTheEnvironmentAgentWhenNoneIsRequested(t *testing.T) {
	launcher := &fakeSessionLauncher{opts: dispatch.SessionLaunchOptions{Agents: []string{"claude", "codex"}}}
	runner := &fakeJobRunner{}
	manager, _ := activeSession()
	svc := newSessionsService(SessionsDeps{Launcher: launcher, Manager: manager, Statuses: manager, Tmux: &fakeSessionTmux{}, Jobs: runner})
	svc.defaultAgentEnv = defaultAgentFunc(func(context.Context) string { return "codex" })

	_, err := svc.CreateSession(t.Context(), dispatch.CreateSessionRequest{Repository: "r", Name: "review-81"})
	require.NoError(t, err)
	require.Len(t, launcher.calls, 1)
	assert.Equal(t, "codex", launcher.calls[0].Agent)
}

func TestSessionsService_CreateSessionKeepsAnExplicitAgentOverTheEnvironment(t *testing.T) {
	launcher := &fakeSessionLauncher{opts: dispatch.SessionLaunchOptions{Agents: []string{"claude", "codex"}}}
	runner := &fakeJobRunner{}
	manager, _ := activeSession()
	svc := newSessionsService(SessionsDeps{Launcher: launcher, Manager: manager, Statuses: manager, Tmux: &fakeSessionTmux{}, Jobs: runner})
	svc.defaultAgentEnv = defaultAgentFunc(func(context.Context) string { return "codex" })

	_, err := svc.CreateSession(t.Context(), dispatch.CreateSessionRequest{Repository: "r", Name: "review-81", Agent: "claude"})
	require.NoError(t, err)
	require.Len(t, launcher.calls, 1)
	assert.Equal(t, "claude", launcher.calls[0].Agent, "an explicit choice is never overridden")
}

func TestSessionsService_CreateSessionIgnoresAnEnvironmentAgentWithNoConfiguredProfile(t *testing.T) {
	launcher := &fakeSessionLauncher{opts: dispatch.SessionLaunchOptions{Agents: []string{"claude"}}}
	runner := &fakeJobRunner{}
	manager, _ := activeSession()
	svc := newSessionsService(SessionsDeps{Launcher: launcher, Manager: manager, Statuses: manager, Tmux: &fakeSessionTmux{}, Jobs: runner})
	svc.defaultAgentEnv = defaultAgentFunc(func(context.Context) string { return "aider" })

	_, err := svc.CreateSession(t.Context(), dispatch.CreateSessionRequest{Repository: "r", Name: "review-81"})
	require.NoError(t, err)
	require.Len(t, launcher.calls, 1)
	assert.Empty(t, launcher.calls[0].Agent, "hive resolves agents.default itself when handed no agent")
}

func TestSessionsService_CreateSessionLeavesAgentEmptyWithNoEnvironmentDefault(t *testing.T) {
	launcher := &fakeSessionLauncher{opts: dispatch.SessionLaunchOptions{Agents: []string{"claude"}}}
	runner := &fakeJobRunner{}
	manager, _ := activeSession()
	svc := newSessionsService(SessionsDeps{Launcher: launcher, Manager: manager, Statuses: manager, Tmux: &fakeSessionTmux{}, Jobs: runner})

	_, err := svc.CreateSession(t.Context(), dispatch.CreateSessionRequest{Repository: "r", Name: "review-81"})
	require.NoError(t, err)
	require.Len(t, launcher.calls, 1)
	assert.Empty(t, launcher.calls[0].Agent)
	assert.Zero(t, launcher.optsCalls, "no override means nothing to validate, so the workspace scan behind SessionLaunchOptions must not run on the create click")
}

func TestSessionsService_CreateSessionSurfacesDuplicateNameOnTheJob(t *testing.T) {
	runner := &fakeJobRunner{}
	manager, _ := activeSession()
	svc := newSessionsService(SessionsDeps{Launcher: &fakeSessionLauncher{err: session.ErrDuplicateName}, Manager: manager, Statuses: manager, Tmux: &fakeSessionTmux{}, Jobs: runner})

	_, err := svc.CreateSession(t.Context(), dispatch.CreateSessionRequest{Repository: "r", Name: "dupe"})
	require.NoError(t, err, "a duplicate name is a job failure, not a validation error")
	require.Error(t, runner.err)
	assert.Contains(t, runner.err.Error(), "already exists")
}

func TestSessionsService_ListSessionsPassesEveryStateThrough(t *testing.T) {
	manager, _ := activeSession()
	manager.sessions = append(manager.sessions, dispatch.SessionSummary{ID: "s2", Name: "old", Slug: "old", State: "recycled"})
	svc := newSessionsService(SessionsDeps{Launcher: &fakeSessionLauncher{}, Manager: manager, Statuses: manager, Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{}})
	got, err := svc.ListSessions(t.Context())
	require.NoError(t, err)
	assert.Equal(t, manager.sessions, got, "a recycled session is unattachable, not unmanageable")
}

func TestSessionsService_SessionStatusesPassesSnapshotThrough(t *testing.T) {
	manager, _ := activeSession()
	manager.statuses = dispatch.SessionStatusSnapshot{
		Items: []dispatch.SessionStatus{{
			SessionID: "s1",
			Running:   true,
			Windows:   []dispatch.SessionWindowStatus{{WindowID: "@1", Status: "ready", Tool: "codex"}},
		}},
		PollInterval: 1500 * time.Millisecond,
	}
	svc := newSessionsService(SessionsDeps{Launcher: &fakeSessionLauncher{}, Manager: manager, Statuses: manager, Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{}})

	got, err := svc.SessionStatuses(t.Context())
	require.NoError(t, err)
	assert.Equal(t, manager.statuses, got)
}

func TestSessionsService_RenameSessionRenamesTmuxBeforeTheStore(t *testing.T) {
	manager, _ := activeSession()
	tmux := &fakeSessionTmux{}
	svc := newSessionsService(SessionsDeps{Launcher: &fakeSessionLauncher{}, Manager: manager, Statuses: manager, Tmux: tmux, Jobs: &fakeJobRunner{}})

	got, err := svc.RenameSession(t.Context(), "s1", "  Review 82  ")
	require.NoError(t, err)
	assert.Equal(t, "Review 82", got.Name)
	assert.Equal(t, "review-82", got.Slug, "the summary carries the new tmux target")
	assert.Equal(t, [][2]string{{"review-81", "review-82"}}, tmux.renames)
	assert.Equal(t, [][2]string{{"s1", "Review 82"}}, manager.renamed)
}

func TestSessionsService_RenameSessionLeavesTheStoreAloneWhenTmuxFails(t *testing.T) {
	manager, _ := activeSession()
	tmux := &fakeSessionTmux{err: errors.New("duplicate session: review-82")}
	svc := newSessionsService(SessionsDeps{Launcher: &fakeSessionLauncher{}, Manager: manager, Statuses: manager, Tmux: tmux, Jobs: &fakeJobRunner{}})

	_, err := svc.RenameSession(t.Context(), "s1", "review 82")
	assert.Equal(t, KindConflict, KindOf(err))
	assert.Empty(t, manager.renamed, "the slug must not move without its tmux session")
}

func TestSessionsService_RenameSessionRollsActualTmuxTargetBackWhenTheStoreFails(t *testing.T) {
	manager, _ := activeSession()
	detail := manager.details["s1"]
	detail.TmuxSession = "actual-review-81"
	manager.details["s1"] = detail
	manager.renameErr = errors.New("disk full")
	tmux := &fakeSessionTmux{}
	svc := newSessionsService(SessionsDeps{Launcher: &fakeSessionLauncher{}, Manager: manager, Statuses: manager, Tmux: tmux, Jobs: &fakeJobRunner{}})

	_, err := svc.RenameSession(t.Context(), "s1", "review 82")
	assert.Equal(t, KindInternal, KindOf(err))
	assert.Equal(t, [][2]string{{"actual-review-81", "review-82"}, {"review-82", "actual-review-81"}}, tmux.renames)
}

func TestSessionsService_RenameSessionDoesNotRollBackAnAbsentTmuxSession(t *testing.T) {
	manager, _ := activeSession()
	manager.renameErr = errors.New("disk full")
	tmux := &fakeSessionTmux{absent: true}
	svc := newSessionsService(SessionsDeps{Launcher: &fakeSessionLauncher{}, Manager: manager, Statuses: manager, Tmux: tmux, Jobs: &fakeJobRunner{}})

	_, err := svc.RenameSession(t.Context(), "s1", "review 82")
	assert.Equal(t, KindInternal, KindOf(err))
	assert.Equal(t, [][2]string{{"review-81", "review-82"}}, tmux.renames)
}

func TestSessionsService_RenameSessionRollbackOutlivesRequestCancellation(t *testing.T) {
	manager, _ := activeSession()
	manager.renameErr = context.Canceled
	ctx, cancel := context.WithCancel(t.Context())
	manager.afterRename = cancel
	tmux := &fakeSessionTmux{}
	svc := newSessionsService(SessionsDeps{Launcher: &fakeSessionLauncher{}, Manager: manager, Statuses: manager, Tmux: tmux, Jobs: &fakeJobRunner{}})

	_, err := svc.RenameSession(ctx, "s1", "review 82")
	assert.Equal(t, KindInternal, KindOf(err))
	require.Len(t, tmux.contextErrs, 2)
	assert.NoError(t, tmux.contextErrs[0])
	assert.NoError(t, tmux.contextErrs[1])
}

func TestSessionsService_RenameSessionRejectsASlugCollision(t *testing.T) {
	manager, _ := activeSession()
	manager.sessions = append(manager.sessions, dispatch.SessionSummary{ID: "s2", Name: "Review 82", Slug: "review-82"})
	tmux := &fakeSessionTmux{}
	svc := newSessionsService(SessionsDeps{Launcher: &fakeSessionLauncher{}, Manager: manager, Statuses: manager, Tmux: tmux, Jobs: &fakeJobRunner{}})

	// "review/82" slugifies onto s2's slug, which would give both sessions the
	// same tmux session name and the same directory slug.
	_, err := svc.RenameSession(t.Context(), "s1", "review/82")
	assert.Equal(t, KindInvalid, KindOf(err))
	assert.Empty(t, tmux.renames)
	assert.Empty(t, manager.renamed)
}

func TestSessionsService_RenameSessionRejectsAPersistedTmuxTargetCollision(t *testing.T) {
	manager, _ := activeSession()
	manager.sessions = append(manager.sessions, dispatch.SessionSummary{
		ID: "s2", Name: "Renamed Elsewhere", Slug: "renamed-elsewhere", TmuxSession: "review-82",
	})
	tmux := &fakeSessionTmux{}
	svc := newSessionsService(SessionsDeps{Launcher: &fakeSessionLauncher{}, Manager: manager, Statuses: manager, Tmux: tmux, Jobs: &fakeJobRunner{}})

	_, err := svc.RenameSession(t.Context(), "s1", "review 82")
	assert.Equal(t, KindInvalid, KindOf(err))
	assert.Empty(t, tmux.renames)
	assert.Empty(t, manager.renamed)
}

func TestSessionsService_RenameSessionValidatesTheName(t *testing.T) {
	manager, _ := activeSession()
	tmux := &fakeSessionTmux{}
	svc := newSessionsService(SessionsDeps{Launcher: &fakeSessionLauncher{}, Manager: manager, Statuses: manager, Tmux: tmux, Jobs: &fakeJobRunner{}})

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
		seen     func(*fakeSessionManager) []string
	}{
		{"delete", func(s *SessionsService, ctx context.Context) (int64, error) { return s.DeleteSession(ctx, "s1") }, "Delete session", deleteSessionJobActionID, func(m *fakeSessionManager) []string { return m.deleted }},
		{"recycle", func(s *SessionsService, ctx context.Context) (int64, error) { return s.RecycleSession(ctx, "s1") }, "Recycle session", recycleSessionJobActionID, func(m *fakeSessionManager) []string { return m.recycled }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			manager, _ := activeSession()
			runner := &fakeJobRunner{}
			svc := newSessionsService(SessionsDeps{Launcher: &fakeSessionLauncher{}, Manager: manager, Statuses: manager, Tmux: &fakeSessionTmux{}, Jobs: runner})

			jobID, err := tt.call(svc, t.Context())
			require.NoError(t, err)
			assert.Equal(t, int64(7), jobID)
			assert.Equal(t, tt.label, runner.label)
			assert.Equal(t, tt.actionID, runner.actionID)
			assert.Equal(t, "review 81", runner.target, "the name is read before the session can stop existing")
			assert.Equal(t, []string{"s1"}, tt.seen(manager))
		})
	}
}

func TestSessionsService_DestructiveOperationsRejectAnUnknownSession(t *testing.T) {
	manager, _ := activeSession()
	runner := &fakeJobRunner{}
	svc := newSessionsService(SessionsDeps{Launcher: &fakeSessionLauncher{}, Manager: manager, Statuses: manager, Tmux: &fakeSessionTmux{}, Jobs: runner})

	_, err := svc.DeleteSession(t.Context(), "gone")
	assert.Equal(t, KindNotFound, KindOf(err))
	_, err = svc.RecycleSession(t.Context(), "gone")
	assert.Equal(t, KindNotFound, KindOf(err))
	assert.False(t, runner.ran)
}

func TestSessionsService_PruneRunsAsAJob(t *testing.T) {
	manager, _ := activeSession()
	runner := &fakeJobRunner{}
	svc := newSessionsService(SessionsDeps{Launcher: &fakeSessionLauncher{}, Manager: manager, Statuses: manager, Tmux: &fakeSessionTmux{}, Jobs: runner})

	jobID, err := svc.PruneSessions(t.Context())
	require.NoError(t, err)
	assert.Equal(t, int64(7), jobID)
	assert.Equal(t, pruneSessionsJobActionID, runner.actionID)
	assert.Equal(t, 1, manager.pruned)
}

func TestSessionsService_SessionRiskCarriesTheWorktreeRecycleWarning(t *testing.T) {
	manager, _ := activeSession()
	manager.risk = dispatch.SessionRisk{UncommittedChanges: true, RecycleDeletes: true}
	svc := newSessionsService(SessionsDeps{Launcher: &fakeSessionLauncher{}, Manager: manager, Statuses: manager, Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{}})

	risk, err := svc.SessionRisk(t.Context(), "s1")
	require.NoError(t, err)
	assert.Equal(t, manager.risk, risk)

	_, err = svc.SessionRisk(t.Context(), " ")
	assert.Equal(t, KindInvalid, KindOf(err))
}

func TestSessionsService_SessionDetail(t *testing.T) {
	manager, detail := activeSession()
	svc := newSessionsService(SessionsDeps{Launcher: &fakeSessionLauncher{}, Manager: manager, Statuses: manager, Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{}})

	got, err := svc.SessionDetail(t.Context(), "s1")
	require.NoError(t, err)
	assert.Equal(t, detail, got)

	_, err = svc.SessionDetail(t.Context(), "gone")
	assert.Equal(t, KindNotFound, KindOf(err))
}

func TestSessionsService_StartTmuxSessionSpawnsFromTheSessionsOwnCheckout(t *testing.T) {
	manager, detail := activeSession()
	detail.Path = "/repos/site-wt-ab12"
	manager.details["s1"] = detail
	svc := newSessionsService(SessionsDeps{Launcher: &fakeSessionLauncher{}, Manager: manager, Statuses: manager, Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{}})

	require.NoError(t, svc.StartTmuxSession(t.Context(), "review-81"))
	assert.Equal(t, [][3]string{{"review 81", "/repos/site-wt-ab12", "acme/site"}}, manager.spawned,
		"the spawn is hive's, so it gets the name, checkout and remote hive spawns from")
}

func TestSessionsService_StartTmuxSessionRejectsASlugNoSessionCarries(t *testing.T) {
	manager, _ := activeSession()
	svc := newSessionsService(SessionsDeps{Launcher: &fakeSessionLauncher{}, Manager: manager, Statuses: manager, Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{}})

	// A tmux session made by hand is attachable, but there is nothing to
	// create one from when it is gone.
	assert.Equal(t, KindNotFound, KindOf(svc.StartTmuxSession(t.Context(), "hand-rolled")))
	assert.Equal(t, KindInvalid, KindOf(svc.StartTmuxSession(t.Context(), "  ")))
	assert.Empty(t, manager.spawned)
}

func TestSessionsService_StartTmuxSessionRefusesASessionWithNoCheckout(t *testing.T) {
	recycled := dispatch.SessionDetail{ID: "s2", Name: "old", Slug: "old", Repo: "acme/site", State: "recycled"}
	manager := &fakeSessionManager{
		sessions: []dispatch.SessionSummary{{ID: "s2", Name: "old", Slug: "old", Repo: "acme/site", State: "recycled"}},
		details:  map[string]dispatch.SessionDetail{"s2": recycled},
	}
	svc := newSessionsService(SessionsDeps{Launcher: &fakeSessionLauncher{}, Manager: manager, Statuses: manager, Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{}})

	assert.Equal(t, KindConflict, KindOf(svc.StartTmuxSession(t.Context(), "old")))
	assert.Empty(t, manager.spawned, "a recycled session's directory is gone; a terminal in it would be one too")
}

func TestSessionsService_StartTmuxSessionRefusesASlugItsNameWouldNotSpawn(t *testing.T) {
	// Hive spawns under the slug it derives from the name, so a record whose two
	// have drifted apart would create a session under a name nothing attaches to.
	drifted := dispatch.SessionDetail{ID: "s1", Name: "review 82", Slug: "review-81", Repo: "acme/site", State: "active"}
	manager := &fakeSessionManager{
		sessions: []dispatch.SessionSummary{{ID: "s1", Name: "review 82", Slug: "review-81", Repo: "acme/site", State: "active"}},
		details:  map[string]dispatch.SessionDetail{"s1": drifted},
	}
	svc := newSessionsService(SessionsDeps{Launcher: &fakeSessionLauncher{}, Manager: manager, Statuses: manager, Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{}})

	assert.Equal(t, KindConflict, KindOf(svc.StartTmuxSession(t.Context(), "review-81")))
	assert.Empty(t, manager.spawned)
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
	failure := &dispatch.SessionCreateError{
		Name:          "review-81",
		Remote:        "https://github.com/acme/site.git",
		CloneStrategy: "full",
		Step:          "Cloning repository...",
		Output:        "Clone strategy: full\nCloning repository...",
		Err:           errors.New("clone repository: git clone: exec git: exit status 1"),
	}
	manager, _ := activeSession()
	bus := newTestBus(t)
	failed := subscribeEvents[events.SessionCreateFailed](t, bus)
	items := &fakeItemSessionStore{refs: map[int64]models.ItemRef{42: {ProfileID: "p", SourceKind: "github", ExternalID: "acme/site#81"}}}
	svc := newSessionsService(SessionsDeps{
		Launcher: &fakeSessionLauncher{err: failure}, Manager: manager, Statuses: manager,
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
	manager, _ := activeSession()
	svc := newSessionsService(SessionsDeps{
		Launcher: &fakeSessionLauncher{err: errors.New("tmux unavailable")}, Manager: manager,
		Statuses: manager, Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{},
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
	manager, _ := activeSession()
	svc := newSessionsService(SessionsDeps{
		Launcher: &fakeSessionLauncher{}, Manager: manager, Statuses: manager,
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
	manager, _ := activeSession()
	svc := newSessionsService(SessionsDeps{
		Launcher: launcher, Manager: manager, Statuses: manager,
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
	manager, _ := activeSession()
	svc := newSessionsService(SessionsDeps{
		Launcher: &fakeSessionLauncher{err: errors.New("nope")}, Manager: manager, Statuses: manager,
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
	manager, _ := activeSession()
	svc := newSessionsService(SessionsDeps{
		Launcher: &fakeSessionLauncher{err: session.ErrDuplicateName}, Manager: manager,
		Statuses: manager, Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{},
	})

	_, err := svc.CreateSession(t.Context(), dispatch.CreateSessionRequest{Repository: "r", Name: "dupe"})
	require.NoError(t, err)
	draft, err := svc.FailedSessionDraft(t.Context())
	require.NoError(t, err)
	assert.Nil(t, draft.Failure)
}

// The row is the half that survives a restart, so it carries the form.
func TestSessionsService_CreateSessionRecordsARetryableActivityRow(t *testing.T) {
	manager, _ := activeSession()
	recorder := &fakeActivityRecorder{}
	svc := newSessionsService(SessionsDeps{
		Launcher: &fakeSessionLauncher{err: &dispatch.SessionCreateError{
			Step: "Cloning repository...",
			Err:  errors.New("clone repository: git clone: exec git: exit status 1"),
		}},
		Manager: manager, Statuses: manager, Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{},
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
	manager, _ := activeSession()
	recorder := &fakeActivityRecorder{}
	items := &fakeItemSessionStore{}
	svc := newSessionsService(SessionsDeps{
		Launcher: &fakeSessionLauncher{}, WorkspaceLauncher: &fakeWorkspaceLauncher{err: errors.New("agent exited")},
		Manager: manager, Statuses: manager, Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{}, Recorder: recorder,
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
	manager, _ := activeSession()
	recorder := &fakeActivityRecorder{}
	svc := newSessionsService(SessionsDeps{
		Launcher: &fakeSessionLauncher{}, Manager: manager, Statuses: manager,
		Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{}, Recorder: recorder,
	})

	_, err := svc.CreateSession(t.Context(), dispatch.CreateSessionRequest{Repository: "r", Name: "fix-crash"})
	require.NoError(t, err)
	assert.Empty(t, recorder.events, "the launcher records the success; this path records only failures")
}

func TestSessionsService_SessionDraftFromActivityRefusesAnUnrelatedRow(t *testing.T) {
	manager, _ := activeSession()
	svc := newSessionsService(SessionsDeps{
		Launcher: &fakeSessionLauncher{}, Manager: manager, Statuses: manager,
		Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{},
	})

	_, err := svc.SessionDraftFromActivity(t.Context(), map[string]string{"rule": "auto-triage"})
	assert.Equal(t, KindInvalid, KindOf(err), "a row with nothing to retry is refused, not silently empty")
}
