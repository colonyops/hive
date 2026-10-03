package dispatch

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/core/eventbus"
	coredb "github.com/colonyops/hive/internal/data/db"
	"github.com/colonyops/hive/internal/data/stores"
	"github.com/colonyops/hive/internal/domain/messaging"
	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/colonyops/hive/internal/domain/session"
	coreterminal "github.com/colonyops/hive/internal/domain/terminal"
	hivesvc "github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/platform/git"
	tmuxexec "github.com/colonyops/hive/internal/platform/tmux/exec"
	"github.com/colonyops/hive/pkg/executil"
	"github.com/colonyops/hive/pkg/tmpl"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type durableMessageServiceTest struct {
	message messaging.Message
	topics  []string
}

func (s *durableMessageServiceTest) Publish(_ context.Context, message messaging.Message, topics []string) (messaging.PublishResult, error) {
	s.message, s.topics = message, topics
	return messaging.PublishResult{Topics: topics}, nil
}

func TestHiveMessagePublisherUsesFixedSenderEmptySessionAndLiteralTopic(t *testing.T) {
	service := &durableMessageServiceTest{}
	topic, err := NewHiveMessagePublisher(service).PublishMessage(t.Context(), "hello", "agent.session.inbox")
	require.NoError(t, err)
	assert.Equal(t, "agent.session.inbox", topic)
	assert.Equal(t, messaging.Message{Payload: "hello", Sender: "hive-desktop", SessionID: ""}, service.message)
	assert.Equal(t, []string{"agent.session.inbox"}, service.topics)
}

func TestHiveMessagePublisherPersistsThroughCoreSQLiteReopen(t *testing.T) {
	dir := t.TempDir()
	first, err := coredb.Open(dir, coredb.DefaultOpenOptions())
	require.NoError(t, err)
	service := hivesvc.NewMessageService(stores.NewMessageStore(first, 0), &config.Config{}, eventbus.New(8))
	publisher := NewHiveMessagePublisher(service)
	const topic = "agent.session.inbox"
	published, err := publisher.PublishMessage(t.Context(), "hello from desktop", topic)
	require.NoError(t, err)
	assert.Equal(t, topic, published)
	require.NoError(t, first.Close())

	reopened, err := coredb.Open(dir, coredb.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })
	reopenedService := hivesvc.NewMessageService(stores.NewMessageStore(reopened, 0), &config.Config{}, eventbus.New(8))
	messages, err := reopenedService.Subscribe(t.Context(), topic, time.Time{})
	require.NoError(t, err)
	require.Len(t, messages, 1)
	assert.Equal(t, "hello from desktop", messages[0].Payload)
	assert.Equal(t, "hive-desktop", messages[0].Sender)
	assert.Empty(t, messages[0].SessionID)
	assert.Equal(t, topic, messages[0].Topic)
}

// newHiveSessions builds the real shared session service over a temporary
// core database. It is what makes these tests worth having: the seam's job is
// to hold against hive's actual implementation, so SessionManagement is
// satisfied structurally here rather than by a fake shaped to fit it.
func newHiveSessions(t *testing.T) (*HiveSessionManager, session.Store) {
	t.Helper()
	return newHiveSessionsWith(t, &config.Config{}, &executil.RealExecutor{})
}

func newHiveSessionsWith(t *testing.T, cfg *config.Config, exec executil.Executor) (*HiveSessionManager, session.Store) {
	t.Helper()
	store := stores.NewSessionStore(openCoreDB(t))
	return NewHiveSessionManager(newHiveSessionServiceOver(t, store, cfg, exec), nil, nil, nil, 0), store
}

func newHiveSessionService(t *testing.T, cfg *config.Config, exec executil.Executor) *hivesvc.SessionService {
	t.Helper()
	return newHiveSessionServiceOver(t, stores.NewSessionStore(openCoreDB(t)), cfg, exec)
}

func newHiveSessionServiceOver(t *testing.T, store session.Store, cfg *config.Config, exec executil.Executor) *hivesvc.SessionService {
	t.Helper()
	runner, ok := exec.(tmuxexec.Runner)
	if !ok {
		runner = noopTmuxRunner{}
	}
	tmuxClient := tmuxexec.New(runner, zerolog.Nop())
	return hivesvc.NewSessionService(
		store,
		git.NewExecutor("git", exec),
		cfg,
		eventbus.New(8),
		exec,
		tmpl.New(tmpl.Config{}),
		hivesvc.PlainStyler{},
		zerolog.Nop(),
		io.Discard,
		io.Discard,
		tmuxClient,
	)
}

type noopTmuxRunner struct{}

func (noopTmuxRunner) Available() bool { return true }

func (noopTmuxRunner) Capture(context.Context, ...string) ([]byte, []byte, error) {
	return nil, nil, nil
}

func (noopTmuxRunner) Input(context.Context, io.Reader, ...string) ([]byte, []byte, error) {
	return nil, nil, nil
}

func (noopTmuxRunner) Interactive(context.Context, multiplexer.AttachStreams, ...string) error {
	return nil
}

func openCoreDB(t *testing.T) *coredb.DB {
	t.Helper()
	database, err := coredb.Open(t.TempDir(), coredb.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	return database
}

type listingSessionManagement struct {
	SessionManagement
	sessions []session.Session
}

func (m listingSessionManagement) ListSessions(context.Context) ([]session.Session, error) {
	return m.sessions, nil
}

type fakeSessionStatusSource struct {
	available bool
	results   map[string]hivesvc.TerminalStatus
	seen      []*session.Session
}

func (f *fakeSessionStatusSource) Available() bool { return f.available }

func (f *fakeSessionStatusSource) FetchBatch(_ context.Context, sessions []*session.Session, _ []hivesvc.RootRepoTarget) map[string]hivesvc.TerminalStatus {
	f.seen = sessions
	return f.results
}

type fakeSessionWindowSource struct {
	results map[string][]SessionWindowRef
	seen    []string
}

func (f *fakeSessionWindowSource) ListIndexedWindows(_ context.Context, slugs []string) (map[string][]SessionWindowRef, error) {
	f.seen = slugs
	return f.results, nil
}

// recordingExecutor stands in for the shell hive spawns tmux through, so the
// seam can be checked against the real shared spawner with no tmux server
// running. absent fails has-session, which is how tmux answers for a session it
// does not hold.
type recordingExecutor struct {
	absent bool
	runs   [][]string
}

func (e *recordingExecutor) record(cmd string, args []string) error {
	e.runs = append(e.runs, append([]string{cmd}, args...))
	if e.absent && len(args) > 0 && args[0] == "has-session" {
		return errors.New("can't find session")
	}
	return nil
}

func (e *recordingExecutor) Run(_ context.Context, cmd string, args ...string) ([]byte, error) {
	return nil, e.record(cmd, args)
}

func (e *recordingExecutor) RunDir(_ context.Context, _, cmd string, args ...string) ([]byte, error) {
	return nil, e.record(cmd, args)
}

func (e *recordingExecutor) RunStream(_ context.Context, _, _ io.Writer, cmd string, args ...string) error {
	return e.record(cmd, args)
}

func (e *recordingExecutor) RunDirStream(_ context.Context, _ string, _, _ io.Writer, cmd string, args ...string) error {
	return e.record(cmd, args)
}

func (*recordingExecutor) Available() bool { return true }

func (e *recordingExecutor) Capture(_ context.Context, args ...string) ([]byte, []byte, error) {
	return nil, nil, e.record("tmux", args)
}

func (e *recordingExecutor) Input(_ context.Context, _ io.Reader, args ...string) ([]byte, []byte, error) {
	return nil, nil, e.record("tmux", args)
}

func (e *recordingExecutor) Interactive(_ context.Context, _ multiplexer.AttachStreams, args ...string) error {
	return e.record("tmux", args)
}

// spawnConfig is a rule whose windows are distinguishable from hive's defaults,
// so the test can tell "hive rendered the configured spawn" from "something here
// built a window set of its own".
func spawnConfig() *config.Config {
	return &config.Config{Rules: []config.Rule{{
		Windows: []config.WindowConfig{
			{Name: "agent", Command: "run {{ .Slug }}", Focus: true},
			{Name: "shell"},
		},
	}}}
}

func TestHiveSessionManagerSpawnsTheConfiguredWindowsDetached(t *testing.T) {
	exec := &recordingExecutor{absent: true}
	manager, _ := newHiveSessionsWith(t, spawnConfig(), exec)

	require.NoError(t, manager.SpawnTmuxSession(t.Context(), "review 81", "/tmp/review-81", "acme/site"))

	assert.Contains(t, exec.runs, []string{"tmux", "has-session", "-t", "review-81"})
	// The session is created under the slug, in the session's own directory,
	// running the configured command — hive's spawn semantics, not a second
	// definition of them here.
	assert.Contains(t, exec.runs, []string{"tmux", "new-session", "-d", "-s", "review-81", "-n", "agent", "-c", "/tmp/review-81", "--", "sh", "-c", "run review-81"})
	assert.Contains(t, exec.runs, []string{"tmux", "new-window", "-t", "review-81", "-n", "shell", "-c", "/tmp/review-81"})
	for _, run := range exec.runs {
		assert.NotContains(t, run, "attach-session", "the desktop attaches over control mode; the spawn must stay detached")
		assert.NotContains(t, run, "switch-client")
	}
}

func TestHiveSessionManagerSpawnLeavesALiveSessionAlone(t *testing.T) {
	exec := &recordingExecutor{}
	manager, _ := newHiveSessionsWith(t, spawnConfig(), exec)

	require.NoError(t, manager.SpawnTmuxSession(t.Context(), "review 81", "/tmp/review-81", "acme/site"))

	assert.Equal(t, [][]string{{"tmux", "has-session", "-t", "review-81"}}, exec.runs,
		"a session tmux already holds is not respawned, so every cold attach can ask for one")
}

func TestHiveSessionManagerListsEveryState(t *testing.T) {
	manager, store := newHiveSessions(t)
	now := time.Now().UTC().Truncate(time.Second)

	active := session.Session{ID: "s1", Name: "review 81", Slug: "review-81", Path: "/tmp/review-81", Remote: "acme/site", State: session.StateActive, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, store.Save(t.Context(), active))
	require.NoError(t, store.Save(t.Context(), session.Session{ID: "s2", Name: "old", Slug: "old", Path: "/tmp/old", State: session.StateRecycled, CreatedAt: now, UpdatedAt: now}))

	got, err := manager.ListSessions(t.Context())
	require.NoError(t, err)
	assert.ElementsMatch(t, []SessionSummary{
		{ID: "s1", Name: "review 81", Slug: "review-81", Repo: "acme/site", State: "active", TmuxSession: "review-81"},
		{ID: "s2", Name: "old", Slug: "old", State: "recycled", TmuxSession: "old"},
	}, got)
}

func TestHiveSessionManagerProjectsLiveStatusForActiveSessions(t *testing.T) {
	statuses := &fakeSessionStatusSource{
		available: true,
		results: map[string]hivesvc.TerminalStatus{
			"s1": {Windows: []hivesvc.WindowStatus{
				{WindowIndex: "0", Status: coreterminal.StatusApproval, Tool: "claude"},
				{WindowIndex: "2", Status: coreterminal.StatusActive, Tool: "pi"},
				{WindowIndex: "9", WindowName: "departed", Status: coreterminal.StatusActive, Tool: "codex"},
			}},
			"s2": {WindowName: "codex", Status: coreterminal.StatusQuestion, Tool: "codex"},
			"s4": {Status: coreterminal.StatusMissing, Error: errors.New("session disappeared")},
		},
	}
	windows := &fakeSessionWindowSource{results: map[string][]SessionWindowRef{
		"actual-one": {{ID: "@1", Index: "0", Name: "claude"}, {ID: "@2", Index: "2", Name: "pi"}},
		"two":        {{ID: "@3", Index: "1", Name: "codex"}},
	}}
	manager := NewHiveSessionManager(listingSessionManagement{sessions: []session.Session{
		{ID: "s1", Slug: "one", State: session.StateActive, Metadata: map[string]string{session.MetaTmuxSession: "actual-one"}},
		{ID: "s2", Slug: "two", State: session.StateActive},
		{ID: "s3", Slug: "three", State: session.StateRecycled},
		{ID: "s4", Slug: "four", State: session.StateActive},
	}}, statuses, windows, nil, 1750*time.Millisecond)

	got, err := manager.SessionStatuses(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1750*time.Millisecond, got.PollInterval)
	assert.Equal(t, []SessionStatus{
		{SessionID: "s1", Running: true, Windows: []SessionWindowStatus{
			{WindowID: "@1", Status: "approval", Tool: "claude"},
			{WindowID: "@2", Status: "active", Tool: "pi"},
		}},
		{SessionID: "s2", Running: true, Windows: []SessionWindowStatus{{WindowID: "@3", Status: "approval", Tool: "codex"}}},
		{SessionID: "s4", Windows: []SessionWindowStatus{}},
	}, got.Items)
	require.Len(t, statuses.seen, 3)
	assert.Equal(t, "s1", statuses.seen[0].ID)
	assert.Equal(t, "s2", statuses.seen[1].ID)
	assert.Equal(t, "s4", statuses.seen[2].ID)
	assert.Equal(t, []string{"actual-one", "two", "four"}, windows.seen)
}

func TestStableWindowIDFallsBackToNameWhenAnIndexWasReused(t *testing.T) {
	t.Parallel()

	refs := []SessionWindowRef{
		{ID: "@shell", Index: "0", Name: "shell"},
		{ID: "@agent", Index: "2", Name: "agent"},
	}
	assert.Equal(t, "@agent", stableWindowID("0", "agent", refs))
}

func TestStableWindowIDDoesNotMapADepartedIndexedWindowToTheSoleSurvivor(t *testing.T) {
	t.Parallel()

	refs := []SessionWindowRef{{ID: "@shell", Index: "0", Name: "shell"}}
	assert.Empty(t, stableWindowID("2", "agent", refs))
}

// An inbox item asks about the one or two sessions it spawned, so the probe
// must be scoped to those — a full sweep would pay a tmux round trip for every
// active session in the install to answer it.
func TestHiveSessionManagerRunningSessionsProbesOnlyTheNamedActiveSessions(t *testing.T) {
	statuses := &fakeSessionStatusSource{available: true}
	windows := &fakeSessionWindowSource{results: map[string][]SessionWindowRef{
		"actual-one": {{ID: "@1", Index: "0"}},
	}}
	manager := NewHiveSessionManager(listingSessionManagement{sessions: []session.Session{
		{ID: "s1", Slug: "one", State: session.StateActive, Metadata: map[string]string{session.MetaTmuxSession: "actual-one"}},
		{ID: "s2", Slug: "two", State: session.StateRecycled},
		{ID: "s3", Slug: "three", State: session.StateActive},
	}}, statuses, windows, nil, time.Second)

	got, err := manager.RunningSessions(t.Context(), []string{"s1", "s2"})
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"s1": true}, got)
	// s2 is recycled and s3 was not asked about, so neither is probed.
	assert.Equal(t, []string{"actual-one"}, windows.seen)
}

// Terminal mode ships dark, so this is the default install: no liveness source
// is data, not a failure — nothing reads as running and the caller still gets
// its sessions.
func TestHiveSessionManagerRunningSessionsReportsNothingWhenTerminalUnavailable(t *testing.T) {
	manager := NewHiveSessionManager(listingSessionManagement{sessions: []session.Session{
		{ID: "s1", State: session.StateActive},
	}}, &fakeSessionStatusSource{}, nil, nil, time.Second)

	got, err := manager.RunningSessions(t.Context(), []string{"s1"})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestHiveSessionManagerReturnsEmptyStatusWhenTerminalUnavailable(t *testing.T) {
	manager := NewHiveSessionManager(listingSessionManagement{}, &fakeSessionStatusSource{}, nil, nil, 1500*time.Millisecond)

	got, err := manager.SessionStatuses(t.Context())
	require.NoError(t, err)
	assert.Empty(t, got.Items)
	assert.Equal(t, 1500*time.Millisecond, got.PollInterval)
}

func TestHiveSessionManagerDetailReadsWorktreeMetadata(t *testing.T) {
	manager, store := newHiveSessions(t)
	now := time.Now().UTC().Truncate(time.Second)

	sess := session.Session{
		ID: "s1", Name: "review 81", Slug: "review-81", Path: "/tmp/review-81",
		Remote: "acme/site", State: session.StateActive, CloneStrategy: session.CloneStrategyWorktree,
		Tags: []string{"pr-81"}, CreatedAt: now, UpdatedAt: now,
	}
	sess.SetMeta(session.MetaWorktreeBranch, "hive/review-81")
	require.NoError(t, store.Save(t.Context(), sess))

	detail, err := manager.SessionDetail(t.Context(), "s1")
	require.NoError(t, err)
	// Timestamps come back in the local zone, so they are compared by instant.
	assert.True(t, detail.CreatedAt.Equal(now))
	assert.True(t, detail.UpdatedAt.Equal(now))
	detail.CreatedAt, detail.UpdatedAt = now, now
	assert.Equal(t, SessionDetail{
		ID: "s1", Name: "review 81", Slug: "review-81", Repo: "acme/site", State: "active",
		Path: "/tmp/review-81", CloneStrategy: session.CloneStrategyWorktree,
		WorktreeBranch: "hive/review-81", TmuxSession: "review-81", Tags: []string{"pr-81"}, CreatedAt: now, UpdatedAt: now,
	}, detail)

	_, err = manager.SessionDetail(t.Context(), "missing")
	require.Error(t, err)
}

func TestHiveSessionManagerRenameReSlugsTheSession(t *testing.T) {
	manager, store := newHiveSessions(t)
	now := time.Now().UTC()
	require.NoError(t, store.Save(t.Context(), session.Session{ID: "s1", Name: "review 81", Slug: "review-81", Path: "/tmp/review-81", State: session.StateActive, CreatedAt: now, UpdatedAt: now}))

	require.NoError(t, manager.RenameSession(t.Context(), "s1", "Review 82"))

	detail, err := manager.SessionDetail(t.Context(), "s1")
	require.NoError(t, err)
	assert.Equal(t, "Review 82", detail.Name)
	assert.Equal(t, "review-82", detail.Slug)
	assert.Equal(t, "/tmp/review-81", detail.Path, "the directory keeps the slug it was cloned under")
}

func TestHiveSessionManagerRiskIsEmptyForANonActiveSession(t *testing.T) {
	manager, store := newHiveSessions(t)
	now := time.Now().UTC()
	require.NoError(t, store.Save(t.Context(), session.Session{ID: "s1", Name: "old", Slug: "old", Path: "/tmp/old", State: session.StateRecycled, CloneStrategy: session.CloneStrategyWorktree, CreatedAt: now, UpdatedAt: now}))

	risk, err := manager.SessionRisk(t.Context(), "s1")
	require.NoError(t, err)
	// No live clone left to hold unsaved work — but recycling a worktree
	// session still deletes it, which the confirmation has to say.
	assert.Equal(t, SessionRisk{RecycleDeletes: true}, risk)
}

// newHiveLauncher builds the launcher over the real shared session service,
// for the same reason newHiveSessions does: a fake shaped to fit the seam would
// report whatever the test wanted to hear.
func newHiveLauncher(t *testing.T, cfg *config.Config) *HiveSessionLauncher {
	t.Helper()
	return NewHiveSessionLauncher(newHiveSessionService(t, cfg, &executil.RealExecutor{}))
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

// failingPostCheckoutRepo is the shape of the incident: a clonable remote plus
// a global post-checkout hook that exits non-zero. git propagates the hook's
// exit code, so the checkout is complete and the clone still failed.
func failingPostCheckoutRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	src := filepath.Join(root, "site")
	require.NoError(t, os.MkdirAll(src, 0o755))
	runGit(t, src, "init", "-q")
	require.NoError(t, os.WriteFile(filepath.Join(src, "a.txt"), []byte("hi"), 0o644))
	runGit(t, src, "add", ".")
	runGit(t, src, "-c", "user.email=a@b.c", "-c", "user.name=a", "commit", "-qm", "init")

	hooks := filepath.Join(root, "hooks")
	require.NoError(t, os.MkdirAll(hooks, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(hooks, "post-checkout"), []byte("#!/bin/sh\nexit 1\n"), 0o755))
	// GIT_CONFIG_* reaches the clone the way a global hook would, without
	// touching the machine.
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.hooksPath")
	t.Setenv("GIT_CONFIG_VALUE_0", hooks)
	return src
}

func TestHiveSessionLauncherReportsTheFailedOperationAndItsCheckout(t *testing.T) {
	cfg := &config.Config{DataDir: t.TempDir()}
	remote := failingPostCheckoutRepo(t)

	_, err := newHiveLauncher(t, cfg).LaunchSession(t.Context(), LaunchSessionRequest{Name: "review-81", Repo: remote})

	var failure *SessionCreateError
	require.ErrorAs(t, err, &failure)
	assert.Equal(t, "review-81", failure.Name)
	assert.Equal(t, remote, failure.Remote)

	// Off hive's own CreateSessionError, not derived here.
	assert.Equal(t, "clone repository", failure.Step)
	assert.Equal(t, "full", failure.CloneStrategy)
	assert.Equal(t, cfg.ReposDir(), filepath.Dir(failure.Destination))
	assert.True(t, failure.LeftoverCheckout, "a hook that fails after checkout leaves it complete")
	assert.DirExists(t, failure.Destination, "the complete checkout the failed clone left behind")

	// The progress tail is still the desktop's own: hive's error names one
	// operation, the tail names the sequence that led to it.
	assert.Contains(t, failure.Output, "Clone strategy: full")
	assert.Contains(t, failure.Output, "Cloning repository...")
	assert.Contains(t, err.Error(), "clone repository", "the step travels in the surfaced error")
}

// git's own reason survives hive's wrapping.
func TestHiveSessionLauncherReportsWhyACloneWasRefused(t *testing.T) {
	cfg := &config.Config{DataDir: t.TempDir()}
	missing := filepath.Join(t.TempDir(), "no-such-repo")

	_, err := newHiveLauncher(t, cfg).LaunchSession(t.Context(), LaunchSessionRequest{Name: "review-81", Repo: missing})

	var failure *SessionCreateError
	require.ErrorAs(t, err, &failure)
	assert.Equal(t, "clone repository", failure.Step)
	assert.Equal(t, "full", failure.CloneStrategy)
	assert.Contains(t, err.Error(), "does not exist", "git's words, not just its exit status")

	// hive names a destination either way; git removed this one, so there is
	// nothing to tell the user to delete.
	assert.NotEmpty(t, failure.Destination)
	assert.False(t, failure.LeftoverCheckout)
	assert.NoDirExists(t, failure.Destination)
}
