package schedule

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSource struct {
	mu         sync.Mutex
	specs      []Spec
	workspaces []string
}

// Snapshot names every workspace the specs come from unless a test set its
// own list, which is how a workspace that is readable but has no schedules --
// and a broken one whose schedules nobody can read -- are told apart here.
func (f *fakeSource) Snapshot() Snapshot {
	f.mu.Lock()
	defer f.mu.Unlock()

	snapshot := Snapshot{Specs: slices.Clone(f.specs), Workspaces: slices.Clone(f.workspaces)}
	if f.workspaces == nil {
		for _, spec := range f.specs {
			if !slices.Contains(snapshot.Workspaces, spec.Workspace) {
				snapshot.Workspaces = append(snapshot.Workspaces, spec.Workspace)
			}
		}
	}
	return snapshot
}

func (f *fakeSource) set(specs ...Spec) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.specs = specs
}

func (f *fakeSource) setWorkspaces(workspaces ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.workspaces = workspaces
}

// pruneCall is one PruneCursors call: the workspaces it was allowed to delete
// inside, and the cursors it had to keep.
type pruneCall struct {
	workspaces []string
	keep       []Cursor
}

type fakeStore struct {
	mu      sync.Mutex
	cursors map[string]Cursor
	saves   int
	pruned  []pruneCall
	runs    []Run
	nextID  int64
	// writes counts every write the scheduler asked for, including the ones a
	// cancelled context refused -- what a test asserts on when the question is
	// whether the scheduler tried at all.
	writes int

	cursorErr error
	insertErr error
}

func newFakeStore() *fakeStore {
	return &fakeStore{cursors: map[string]Cursor{}}
}

func storeKey(workspace, id string) string { return workspace + "\x00" + id }

func (f *fakeStore) Cursor(ctx context.Context, workspace, id string) (Cursor, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Cursor{}, false, err
	}
	if f.cursorErr != nil {
		return Cursor{}, false, f.cursorErr
	}
	cursor, ok := f.cursors[storeKey(workspace, id)]
	return cursor, ok, nil
}

// SaveCursor, like every write here, refuses a cancelled context: database/sql
// checks one before it touches the connection, so a fake that ignored it would
// hide exactly the quit-time failures these tests are about.
func (f *fakeStore) SaveCursor(ctx context.Context, cursor Cursor) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes++
	if err := ctx.Err(); err != nil {
		return err
	}
	f.cursors[storeKey(cursor.Workspace, cursor.ID)] = cursor
	f.saves++
	return nil
}

func (f *fakeStore) PruneCursors(ctx context.Context, workspaces []string, keep []Cursor) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	f.pruned = append(f.pruned, pruneCall{workspaces: slices.Clone(workspaces), keep: slices.Clone(keep)})
	return nil
}

func (f *fakeStore) LastLaunchedRun(ctx context.Context, workspace, id string) (Run, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Run{}, false, err
	}
	for _, v := range slices.Backward(f.runs) {
		run := v
		if run.Workspace == workspace && run.ScheduleID == id && run.Status == StatusLaunched {
			return run, true, nil
		}
	}
	return Run{}, false, nil
}

func (f *fakeStore) InsertRun(ctx context.Context, run Run) (Run, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes++
	if err := ctx.Err(); err != nil {
		return Run{}, err
	}
	if f.insertErr != nil {
		return Run{}, f.insertErr
	}
	f.nextID++
	run.ID = f.nextID
	f.runs = append(f.runs, run)
	return run, nil
}

func (f *fakeStore) seedCursor(cursor Cursor) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cursors[storeKey(cursor.Workspace, cursor.ID)] = cursor
}

func (f *fakeStore) seedRun(run Run) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	run.ID = f.nextID
	f.runs = append(f.runs, run)
}

func (f *fakeStore) allRuns() []Run {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.runs)
}

func (f *fakeStore) allCursors() map[string]Cursor {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[string]Cursor, len(f.cursors))
	maps.Copy(out, f.cursors)
	return out
}

func (f *fakeStore) saveCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.saves
}

func (f *fakeStore) writeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.writes
}

func (f *fakeStore) lastPruned() pruneCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.pruned) == 0 {
		return pruneCall{}
	}
	return f.pruned[len(f.pruned)-1]
}

type fakeLauncher struct {
	mu       sync.Mutex
	requests []LaunchRequest
	nextID   int64
	live     map[int64]bool
	err      error

	// awaitQuit holds Launch until the scheduler's context is cancelled, which
	// is what a quit landing in the middle of a launch looks like. failOnQuit
	// then reports the cancellation instead of a chat.
	awaitQuit  bool
	failOnQuit bool
}

func newFakeLauncher() *fakeLauncher {
	return &fakeLauncher{live: map[int64]bool{}}
}

func (f *fakeLauncher) Launch(ctx context.Context, req LaunchRequest) (int64, error) {
	f.mu.Lock()
	await, failOnQuit := f.awaitQuit, f.failOnQuit
	f.mu.Unlock()
	if await {
		<-ctx.Done()
		if failOnQuit {
			return 0, ctx.Err()
		}
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return 0, f.err
	}
	f.requests = append(f.requests, req)
	f.nextID++
	return f.nextID, nil
}

func (f *fakeLauncher) SessionLive(_ context.Context, sessionID int64) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.live[sessionID], nil
}

func (f *fakeLauncher) allRequests() []LaunchRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.requests)
}

type fakeNamer struct{ names map[string]string }

func (f fakeNamer) WorkspaceName(dir string) string { return f.names[dir] }

func hourlySpec() Spec {
	return Spec{Workspace: "product", ID: "hourly", Name: "Hourly digest", Cron: "0 * * * *", Prompt: "go"}
}

// TestSchedulerFiresAtTheDueTime drives the whole loop on synctest's clock: the
// first pass only opens the window, and the occurrence a minute later is what
// launches a chat.
func TestSchedulerFiresAtTheDueTime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		source := &fakeSource{}
		source.set(Spec{Workspace: "product", ID: "minutely", Cron: "* * * * *", Prompt: "hello"})
		store := newFakeStore()
		launcher := newFakeLauncher()

		scheduler := New(Options{Source: source, Store: store, Launcher: launcher})
		scheduler.Start(t.Context())
		synctest.Wait()
		require.Empty(t, store.allRuns(), "a schedule seen for the first time never back-fires")

		time.Sleep(90 * time.Second)
		synctest.Wait()
		scheduler.Stop()

		runs := store.allRuns()
		require.Len(t, runs, 1)
		assert.Equal(t, StatusLaunched, runs[0].Status)
		assert.Equal(t, ReasonDue, runs[0].Reason)
		assert.Equal(t, 0, runs[0].Missed)
		assert.Equal(t, int64(1), runs[0].SessionID)

		requests := launcher.allRequests()
		require.Len(t, requests, 1)
		assert.Equal(t, "product", requests[0].Workspace)
		assert.Equal(t, "hello", requests[0].Prompt)
		assert.True(t, strings.HasPrefix(requests[0].Name, "minutely - "), "got %q", requests[0].Name)
	})
}

// TestSchedulerCatchesUpOnTheFirstPass is the durability property: the app was
// closed over two occurrences, and launching runs the latest of them once,
// counting the rest as missed.
func TestSchedulerCatchesUpOnTheFirstPass(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		spec := Spec{Workspace: "product", ID: "hourly", Cron: "@every 1h", Prompt: "go"}
		source := &fakeSource{}
		source.set(spec)

		store := newFakeStore()
		store.seedCursor(Cursor{
			Workspace: spec.Workspace, ID: spec.ID,
			EvaluatedThrough: time.Now().Add(-150 * time.Minute), Cron: spec.Cron,
		})
		launcher := newFakeLauncher()

		scheduler := New(Options{Source: source, Store: store, Launcher: launcher})
		scheduler.Start(t.Context())
		synctest.Wait()
		scheduler.Stop()

		runs := store.allRuns()
		require.Len(t, runs, 1)
		assert.Equal(t, StatusLaunched, runs[0].Status)
		assert.Equal(t, ReasonCatchUp, runs[0].Reason)
		assert.Equal(t, 1, runs[0].Missed)
	})
}

func TestSchedulerReloadPicksUpANewSpec(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		source := &fakeSource{}
		store := newFakeStore()

		scheduler := New(Options{Source: source, Store: store, Launcher: newFakeLauncher()})
		scheduler.Start(t.Context())
		synctest.Wait()
		require.Empty(t, store.allCursors())

		source.set(hourlySpec())
		scheduler.Reload()
		synctest.Wait()
		scheduler.Stop()

		assert.Contains(t, store.allCursors(), storeKey("product", "hourly"))
	})
}

func TestSchedulerStopJoinsAndIsIdempotent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		source := &fakeSource{}
		source.set(Spec{Workspace: "product", ID: "minutely", Cron: "* * * * *", Prompt: "hello"})
		store := newFakeStore()

		scheduler := New(Options{Source: source, Store: store, Launcher: newFakeLauncher()})
		scheduler.Start(t.Context())
		synctest.Wait()

		scheduler.Stop()
		scheduler.Stop()

		saves := store.saveCount()
		time.Sleep(10 * time.Minute)
		synctest.Wait()
		assert.Equal(t, saves, store.saveCount(), "a stopped loop runs no more passes")
	})
}

// TestStopWithoutStart: Close is the single teardown path and runs even when
// startup never reached Start.
func TestStopWithoutStart(t *testing.T) {
	t.Parallel()

	scheduler := New(Options{Source: &fakeSource{}, Store: newFakeStore(), Launcher: newFakeLauncher()})
	scheduler.Stop()
	scheduler.Stop()
}

// due is a scheduler on a stopped clock whose next Pass has exactly one
// occurrence waiting, at now-30m -- late enough to be a catch-up.
type due struct {
	scheduler *Scheduler
	source    *fakeSource
	store     *fakeStore
	launcher  *fakeLauncher
	now       time.Time
}

func duePass(t *testing.T, spec Spec, opts *Options) due {
	t.Helper()

	now := at(9, 30)
	source := &fakeSource{}
	source.set(spec)
	store := newFakeStore()
	store.seedCursor(Cursor{
		Workspace: spec.Workspace, ID: spec.ID,
		EvaluatedThrough: now.Add(-90 * time.Minute), Cron: spec.Cron,
	})
	launcher := newFakeLauncher()

	options := Options{Source: source, Store: store, Launcher: launcher, Now: func() time.Time { return now }}
	if opts != nil {
		options.OnRun = opts.OnRun
		options.Names = opts.Names
	}
	return due{scheduler: New(options), source: source, store: store, launcher: launcher, now: now}
}

func TestPassSkipsWhileThePreviousChatIsStillRunning(t *testing.T) {
	t.Parallel()

	spec := hourlySpec()
	h := duePass(t, spec, nil)
	h.store.seedRun(Run{Workspace: spec.Workspace, ScheduleID: spec.ID, Status: StatusLaunched, SessionID: 7})
	h.launcher.live[7] = true

	require.NoError(t, h.scheduler.pass(t.Context()))

	runs := h.store.allRuns()
	require.Len(t, runs, 2)
	assert.Equal(t, StatusSkipped, runs[1].Status)
	assert.Equal(t, "the previous run's chat is still running", runs[1].Error)
	assert.Empty(t, h.launcher.allRequests())
}

func TestPassSkipsAMissedRunWhenTheScheduleSaysTo(t *testing.T) {
	t.Parallel()

	spec := hourlySpec()
	spec.OnMissed = OnMissedSkip
	h := duePass(t, spec, nil)

	require.NoError(t, h.scheduler.pass(t.Context()))

	runs := h.store.allRuns()
	require.Len(t, runs, 1)
	assert.Equal(t, StatusSkipped, runs[0].Status)
	assert.Equal(t, ReasonCatchUp, runs[0].Reason)
	assert.Equal(t, "missed while the app was closed (on_missed: skip)", runs[0].Error)
	assert.Empty(t, h.launcher.allRequests())
}

func TestPassRecordsALaunchFailure(t *testing.T) {
	t.Parallel()

	h := duePass(t, hourlySpec(), nil)
	h.launcher.err = errors.New("tmux is not installed")

	require.NoError(t, h.scheduler.pass(t.Context()))

	runs := h.store.allRuns()
	require.Len(t, runs, 1)
	assert.Equal(t, StatusFailed, runs[0].Status)
	assert.Equal(t, int64(0), runs[0].SessionID)
	assert.Contains(t, runs[0].Error, "tmux is not installed")
}

func TestPassRecordsAPromptFailure(t *testing.T) {
	t.Parallel()

	spec := hourlySpec()
	// A template can validate at edit time and still fail on the data a run
	// hands it, and the run has to say so rather than vanish.
	spec.Prompt = "{{ .Nope }}"
	h := duePass(t, spec, nil)

	require.NoError(t, h.scheduler.pass(t.Context()))

	runs := h.store.allRuns()
	require.Len(t, runs, 1)
	assert.Equal(t, StatusFailed, runs[0].Status)
	assert.Empty(t, h.launcher.allRequests())
}

func TestPassRendersThePromptWithTheLastRunAndWorkspaceName(t *testing.T) {
	t.Parallel()

	spec := hourlySpec()
	spec.Prompt = `{{ .Workspace.Name }}: since {{ date "2006-01-02 15:04" .LastRun }}, reason {{ .Reason }}, missed {{ .Missed }}`
	var seen []Run
	h := duePass(t, spec, &Options{
		Names: fakeNamer{names: map[string]string{"product": "Product"}},
		OnRun: func(run Run) { seen = append(seen, run) },
	})
	h.store.seedRun(Run{
		Workspace: spec.Workspace, ScheduleID: spec.ID, Status: StatusLaunched, SessionID: 4,
		ScheduledFor: at(8, 0),
	})

	require.NoError(t, h.scheduler.pass(t.Context()))

	requests := h.launcher.allRequests()
	require.Len(t, requests, 1)
	assert.Equal(t, "Product: since 2026-09-04 08:00, reason catch_up, missed 0", requests[0].Prompt)
	assert.Equal(t, "Hourly digest - Sep 4 09:00", requests[0].Name)

	require.Len(t, seen, 1)
	assert.NotZero(t, seen[0].ID, "OnRun receives the stored run, not the one handed to InsertRun")
	assert.Equal(t, StatusLaunched, seen[0].Status)
}

// The prune scope is the workspaces the snapshot could read, not the ones the
// specs happen to name: a readable workspace that declares nothing still has
// its leftover cursors pruned, and one whose manifest did not parse is left
// alone rather than read as "every schedule here was deleted".
func TestPassPrunesTheCursorsOfTheSpecsItSaw(t *testing.T) {
	t.Parallel()

	h := duePass(t, hourlySpec(), nil)
	h.source.setWorkspaces("product", "emptied")

	require.NoError(t, h.scheduler.pass(t.Context()))

	pruned := h.store.lastPruned()
	assert.Equal(t, []string{"product", "emptied"}, pruned.workspaces)
	assert.Equal(t, []Cursor{{
		Workspace: "product", ID: "hourly", EvaluatedThrough: h.now, Cron: "0 * * * *",
	}}, pruned.keep)
}

// TestStopRecordsAChatItAlreadyLaunched: Stop cancels the pass's context, and
// a chat that already exists still has to be written down. Without the run row
// and the closed cursor the next start launches the same occurrence again.
func TestStopRecordsAChatItAlreadyLaunched(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		spec := hourlySpec()
		source := &fakeSource{}
		source.set(spec)
		store := newFakeStore()
		store.seedCursor(Cursor{
			Workspace: spec.Workspace, ID: spec.ID,
			EvaluatedThrough: time.Now().Add(-90 * time.Minute), Cron: spec.Cron,
		})
		launcher := newFakeLauncher()
		launcher.awaitQuit = true
		started := time.Now()

		scheduler := New(Options{Source: source, Store: store, Launcher: launcher})
		scheduler.Start(t.Context())
		synctest.Wait()
		scheduler.Stop()

		runs := store.allRuns()
		require.Len(t, runs, 1)
		assert.Equal(t, StatusLaunched, runs[0].Status)
		assert.Equal(t, int64(1), runs[0].SessionID)

		cursor, ok := store.allCursors()[storeKey("product", "hourly")]
		require.True(t, ok)
		assert.True(t, cursor.EvaluatedThrough.Equal(started), "the closed cursor is what stops a second launch on the next start")
	})
}

// TestStopLeavesTheOccurrenceOpenWhenNothingLaunched is the other half: the
// quit stopped the launch itself, so there is no chat to account for and the
// occurrence has to survive to the next start.
func TestStopLeavesTheOccurrenceOpenWhenNothingLaunched(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		spec := hourlySpec()
		source := &fakeSource{}
		source.set(spec)
		store := newFakeStore()
		seeded := time.Now().Add(-90 * time.Minute)
		store.seedCursor(Cursor{Workspace: spec.Workspace, ID: spec.ID, EvaluatedThrough: seeded, Cron: spec.Cron})
		launcher := newFakeLauncher()
		launcher.awaitQuit, launcher.failOnQuit = true, true

		scheduler := New(Options{Source: source, Store: store, Launcher: launcher})
		scheduler.Start(t.Context())
		synctest.Wait()
		scheduler.Stop()

		assert.Empty(t, store.allRuns(), "a launch the quit stopped is not a failed run")
		assert.Zero(t, store.writeCount(), "nothing is written down, not even an attempt the database would refuse")
		cursor, ok := store.allCursors()[storeKey("product", "hourly")]
		require.True(t, ok)
		assert.True(t, cursor.EvaluatedThrough.Equal(seeded), "the occurrence is evaluated again on the next start")
	})
}

// TestPassKeepsGoingAfterASpecFails: one broken schedule must not stop the
// rest, and the returned error is only the first failure.
func TestPassKeepsGoingAfterASpecFails(t *testing.T) {
	t.Parallel()

	now := at(9, 30)
	broken := Spec{Workspace: "product", ID: "broken", Cron: "0 * * * *", Prompt: "go"}
	fine := Spec{Workspace: "product", ID: "fine", Cron: "0 * * * *", Prompt: "go"}

	source := &fakeSource{}
	source.set(broken, fine)
	store := newFakeStore()
	store.cursorErr = errors.New("the database is locked")

	scheduler := New(Options{Source: source, Store: store, Launcher: newFakeLauncher(), Now: func() time.Time { return now }})
	err := scheduler.pass(t.Context())

	require.ErrorContains(t, err, "the database is locked")
	assert.Len(t, store.lastPruned().keep, 2, "a spec that failed still keeps its cursor")
}

func TestRunNowIsManualAndLeavesTheCursorAlone(t *testing.T) {
	t.Parallel()

	h := duePass(t, hourlySpec(), nil)
	before := h.store.allCursors()

	run, err := h.scheduler.RunNow(t.Context(), "product", "hourly")
	require.NoError(t, err)

	assert.Equal(t, StatusLaunched, run.Status)
	assert.Equal(t, ReasonManual, run.Reason)
	assert.Equal(t, h.now, run.ScheduledFor)
	assert.Equal(t, h.now, run.StartedAt)
	assert.Equal(t, 0, run.Missed)
	assert.Len(t, h.launcher.allRequests(), 1)
	assert.Equal(t, before, h.store.allCursors(), "a manual run must not consume the next occurrence")
	assert.Equal(t, 0, h.store.saveCount())

	_, err = h.scheduler.RunNow(t.Context(), "product", "nope")
	require.ErrorIs(t, err, ErrNotFound)
}

// TestPassClosesTheWindowAfterAFailedRun: an occurrence is never retried. A
// run that launched but could not be recorded would otherwise start a second
// chat on the next pass.
func TestPassClosesTheWindowAfterAFailedRun(t *testing.T) {
	t.Parallel()

	h := duePass(t, hourlySpec(), nil)
	h.store.insertErr = errors.New("the database is locked")

	require.Error(t, h.scheduler.pass(t.Context()))

	cursor, ok := h.store.allCursors()[storeKey("product", "hourly")]
	require.True(t, ok)
	assert.Equal(t, h.now, cursor.EvaluatedThrough)
}
