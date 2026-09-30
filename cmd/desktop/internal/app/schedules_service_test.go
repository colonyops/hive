package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/agentws"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/schedule"
)

// brokenScheduleManifest is a manifest the loader refuses: its cron is not one.
const brokenScheduleManifest = "version: 5\nname: Broken\ncommand: claude" + agentws.PromptTail + "\nschedules:\n  - id: weekly\n    cron: not a cron\n    prompt: go\n"

// fakeScheduleLauncher stands in for the agent-workspace service: launching a
// chat for real needs tmux and an agent CLI, and neither says anything about
// what this service does with the run it gets back.
type fakeScheduleLauncher struct {
	mu       sync.Mutex
	launched []schedule.LaunchRequest
	nextID   int64
	live     bool
	err      error
}

func (f *fakeScheduleLauncher) Launch(_ context.Context, req schedule.LaunchRequest) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return 0, f.err
	}
	f.launched = append(f.launched, req)
	f.nextID++
	return f.nextID, nil
}

func (f *fakeScheduleLauncher) SessionLive(context.Context, int64) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.live, nil
}

func (f *fakeScheduleLauncher) requests() []schedule.LaunchRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]schedule.LaunchRequest(nil), f.launched...)
}

type scheduleFixture struct {
	svc        *SchedulesService
	db         *queries.DB
	stores     *stores.Stores
	root       string
	workspaces *agentws.Store
	scheduler  *schedule.Scheduler
	launcher   *fakeScheduleLauncher
}

// newTestSchedulesService builds the service over a real workspace root and a
// real database, with only the launcher faked. The scheduler is the real one:
// RunNow's whole behaviour lives in it, and a fake would prove nothing about
// the run this service hands back.
func newTestSchedulesService(t *testing.T) scheduleFixture {
	t.Helper()

	root := t.TempDir()
	writeAgentWorkspaceManifest(t, root, "demo", agentWorkspaceYAML("Demo", "claude"+agentws.PromptTail, ""))

	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	st := stores.New(db, stores.Options{})

	workspaces := agentws.NewStore(root)
	require.NoError(t, workspaces.Reload())

	launcher := &fakeScheduleLauncher{}
	adapters := scheduleWorkspaces{store: workspaces}
	scheduler := schedule.New(schedule.Options{
		Source:   adapters,
		Names:    adapters,
		Store:    scheduleStore{schedules: st.Schedules},
		Launcher: launcher,
		Logger:   zerolog.Nop(),
	})

	svc := newSchedulesService(SchedulesDeps{
		Workspaces: workspaces, Schedules: st.Schedules, Sessions: st.AgentSessions,
		Scheduler: scheduler, Logger: zerolog.Nop(),
	})
	return scheduleFixture{
		svc: svc, db: db, stores: st, root: root, workspaces: workspaces,
		scheduler: scheduler, launcher: launcher,
	}
}

// declare puts specs in the manifest the way the workspace editor does, then
// reloads the readers of it. This service only reads schedules; writing one is
// AgentWorkspacesService's.
func (f scheduleFixture) declare(t *testing.T, specs ...schedule.Spec) {
	t.Helper()
	require.NoError(t, agentws.WriteManifest(f.root, "demo", agentws.ManifestEdit{
		Name: "Demo", Command: "claude" + agentws.PromptTail, Schedules: specs,
	}))
	require.NoError(t, f.workspaces.Reload())
	f.scheduler.Reload()
}

// List is the MCP tools' read and the workspace view's rows: the manifest's
// specs joined with their run state. A workspace that is missing is not_found;
// one whose manifest does not parse is refused with the reason rather than
// answered with rows the file no longer says.
func TestSchedulesServiceListJoinsTheSpecsWithTheirRunState(t *testing.T) {
	f := newTestSchedulesService(t)
	f.declare(t, schedule.Spec{
		ID: "weekly", Name: "Weekly summary", Cron: "0 9 * * 5", Prompt: "Summarize the week.",
	})

	rows, err := f.svc.List(t.Context(), "demo")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "weekly", rows[0].ID)
	assert.Equal(t, "Weekly summary", rows[0].Name)
	assert.Equal(t, "0 9 * * 5", rows[0].Cron)
	assert.Equal(t, "run", rows[0].OnMissed, "the manifest omits the default; the view names it")
	require.NotNil(t, rows[0].NextRunAt)
	assert.Greater(t, *rows[0].NextRunAt, time.Now().UnixMilli())
	assert.Nil(t, rows[0].LastRun, "a schedule that has never fired has no last run")

	// A disabled schedule keeps its cron but has nothing coming.
	f.declare(t, schedule.Spec{
		ID: "weekly", Name: "Weekly summary", Cron: "0 9 * * 5", Prompt: "Summarize the week.",
		Disabled: true, OnMissed: schedule.OnMissedSkip,
	})
	rows, err = f.svc.List(t.Context(), "demo")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.True(t, rows[0].Disabled)
	assert.Equal(t, "skip", rows[0].OnMissed)
	assert.Nil(t, rows[0].NextRunAt)

	_, err = f.svc.List(t.Context(), "missing")
	require.Error(t, err)
	assert.Equal(t, KindNotFound, KindOf(err))

	writeAgentWorkspaceManifest(t, f.root, "demo", brokenScheduleManifest)
	require.NoError(t, f.workspaces.Reload())
	_, err = f.svc.List(t.Context(), "demo")
	require.Error(t, err)
	assert.Equal(t, KindInvalid, KindOf(err))
}

// The run history is a decoration on a row the manifest already fully
// describes, so a database that cannot answer for it costs the row its
// lastRun, not the caller its listing.
func TestScheduleRowsSurviveAnUnreadableRunHistory(t *testing.T) {
	f := newTestSchedulesService(t)

	f.declare(t, schedule.Spec{ID: "weekly", Name: "Weekly summary", Cron: "@daily", Prompt: "go"})
	require.NoError(t, f.db.Close())

	rows, err := f.svc.List(t.Context(), "demo")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "weekly", rows[0].ID)
	require.NotNil(t, rows[0].NextRunAt, "everything the manifest says is still there")
	assert.Nil(t, rows[0].LastRun)
}

func TestSchedulesServiceRunNowRecordsAManualRun(t *testing.T) {
	f := newTestSchedulesService(t)

	f.declare(t, schedule.Spec{
		ID: "weekly", Name: "Weekly summary",
		Cron: "0 9 * * 5", Prompt: "Summarize {{ .Workspace.Name }}.",
	})

	run, err := f.svc.RunNow(t.Context(), "demo", "weekly")
	require.NoError(t, err)
	assert.Equal(t, "launched", run.Status)
	assert.Equal(t, "Summarize Demo.", run.Prompt, "the workspace name comes from the manifest, not the directory")
	require.NotNil(t, run.SessionID)
	assert.Equal(t, int64(1), *run.SessionID)

	requests := f.launcher.requests()
	require.Len(t, requests, 1)
	assert.Equal(t, "demo", requests[0].Workspace)
	assert.Contains(t, requests[0].Name, "Weekly summary")

	// The manual run is the schedule's newest, so it shows as its last run.
	rows, err := f.svc.List(t.Context(), "demo")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.NotNil(t, rows[0].LastRun)
	assert.Equal(t, run.ID, rows[0].LastRun.ID)

	_, err = f.svc.RunNow(t.Context(), "demo", "nope")
	require.Error(t, err)
	assert.Equal(t, KindNotFound, KindOf(err))
}

// History is per schedule, and a schedule that no longer exists still answers
// with what it did.
func TestSchedulesServiceRunsAnswerForARemovedSchedule(t *testing.T) {
	f := newTestSchedulesService(t)

	for i, id := range []string{"alpha", "beta"} {
		for n := range 2 {
			_, err := f.stores.Schedules.InsertRun(t.Context(), stores.ScheduleRun{
				Workspace: "demo", ScheduleID: id, ScheduleName: id,
				ScheduledFor: int64(1_000 + i*10 + n), StartedAt: int64(1_000 + i*10 + n),
				Reason: "due", Status: "launched", SessionID: int64(1 + n),
			})
			require.NoError(t, err)
		}
	}

	scoped, err := f.svc.Runs(t.Context(), "demo", "alpha", 0)
	require.NoError(t, err)
	require.Len(t, scoped, 2)
	for _, run := range scoped {
		assert.Equal(t, "alpha", run.ScheduleID)
	}

	gone, err := f.svc.Runs(t.Context(), "demo", "beta", 0)
	require.NoError(t, err)
	assert.Len(t, gone, 2)

	_, err = f.svc.Runs(t.Context(), "demo", "", 0)
	require.Error(t, err)
	assert.Equal(t, KindInvalid, KindOf(err))
}

// An empty list means the schedule has not run, never that the caller named
// the wrong workspace or id: those are not_found, the way the rest of the
// surface answers for a thing that does not exist.
func TestSchedulesServiceRunsTellAMissingScheduleFromOneThatNeverRan(t *testing.T) {
	f := newTestSchedulesService(t)
	f.declare(t, schedule.Spec{ID: "alpha", Cron: "@daily", Prompt: "go"})

	fresh, err := f.svc.Runs(t.Context(), "demo", "alpha", 0)
	require.NoError(t, err)
	assert.Empty(t, fresh, "a declared schedule that has not run answers with nothing")

	_, err = f.svc.Runs(t.Context(), "demo", "never", 0)
	require.Error(t, err)
	assert.Equal(t, KindNotFound, KindOf(err), "an id that is neither declared nor has run")

	_, err = f.svc.Runs(t.Context(), "nowhere", "alpha", 0)
	require.Error(t, err)
	assert.Equal(t, KindNotFound, KindOf(err), "a workspace that does not exist")

	_, err = f.svc.Runs(t.Context(), "../demo", "alpha", 0)
	require.Error(t, err)
	assert.Equal(t, KindInvalid, KindOf(err))
}

// A run points at the chat it launched only while that chat exists: a
// scheduled chat deletes itself when its task is done, and a history entry
// must not offer to open a chat nobody can.
func TestSchedulesServiceRunsDropAChatThatWasDeleted(t *testing.T) {
	f := newTestSchedulesService(t)
	f.declare(t, schedule.Spec{ID: "alpha", Cron: "@daily", Prompt: "go"})

	chat, err := f.stores.AgentSessions.Create(t.Context(), stores.AgentSessionCreate{
		Workspace: "demo", Name: "s1", Agent: "claude", AgentSessionID: "a",
	})
	require.NoError(t, err)
	for _, run := range []stores.ScheduleRun{
		{Workspace: "demo", ScheduleID: "alpha", ScheduleName: "alpha", ScheduledFor: 100, StartedAt: 100, Reason: "due", Status: "launched", SessionID: chat.ID},
		{Workspace: "demo", ScheduleID: "alpha", ScheduleName: "alpha", ScheduledFor: 200, StartedAt: 200, Reason: "due", Status: "launched", SessionID: chat.ID + 1000},
	} {
		_, err := f.stores.Schedules.InsertRun(t.Context(), run)
		require.NoError(t, err)
	}

	runs, err := f.svc.Runs(t.Context(), "demo", "alpha", 0)
	require.NoError(t, err)
	require.Len(t, runs, 2)
	assert.Nil(t, runs[0].SessionID, "the newest run's chat is gone")
	require.NotNil(t, runs[1].SessionID)
	assert.Equal(t, chat.ID, *runs[1].SessionID, "a chat that still exists is still pointed at")

	rows, err := f.svc.List(t.Context(), "demo")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.NotNil(t, rows[0].LastRun)
	assert.Nil(t, rows[0].LastRun.SessionID)
}

func TestSchedulesServicePreviewReportsErrorsAsFields(t *testing.T) {
	f := newTestSchedulesService(t)

	preview, err := f.svc.Preview(t.Context(), PreviewRequest{
		Workspace: "demo", Cron: "0 9 * * 5", Prompt: "Summarize {{ .Workspace.Name }} since {{ if .LastRun }}{{ date \"2006-01-02\" .LastRun }}{{ else }}the start{{ end }}.",
	})
	require.NoError(t, err)
	assert.Len(t, preview.Next, previewOccurrences)
	assert.Empty(t, preview.CronError)
	assert.Empty(t, preview.PromptError)
	assert.Contains(t, preview.Prompt, "Summarize Demo since 20")
	assert.Equal(t, "Summarize Demo since the start.", preview.FirstRunPrompt, "the first run, with no previous one, is previewed too")

	firstRunOnly, err := f.svc.Preview(t.Context(), PreviewRequest{
		Workspace: "demo", Cron: "0 9 * * 5", Prompt: "Since {{ .LastRun.Format \"2006-01-02\" }}.",
	})
	require.NoError(t, err)
	assert.Contains(t, firstRunOnly.PromptError, "on the first run", "a template that only breaks without a previous run is reported before it is saved")
	assert.Empty(t, firstRunOnly.Prompt)

	broken, err := f.svc.Preview(t.Context(), PreviewRequest{
		Workspace: "demo", Cron: "every friday", Prompt: "{{ .Nope }}",
	})
	require.NoError(t, err, "a bad edit is the answer, not a failure")
	assert.NotEmpty(t, broken.CronError)
	assert.NotEmpty(t, broken.PromptError)
	assert.Empty(t, broken.Next)
	assert.Empty(t, broken.Prompt)
}
