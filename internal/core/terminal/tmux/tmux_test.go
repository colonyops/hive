package tmux

import (
	"context"
	"errors"
	"regexp"
	"sync/atomic"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/colonyops/hive/internal/core/terminal/assess"
	"github.com/colonyops/hive/internal/core/terminal/classifier"
	"github.com/colonyops/hive/internal/core/terminal/process"
	"github.com/colonyops/hive/internal/core/terminal/status"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testToolClaude = "claude"
	testToolCodex  = "codex"

	// Content chosen (and shaped after internal/core/terminal/status's own
	// test fixtures) to land deterministically on one assess.State via the
	// generic rule set: tool "agent" has no dedicated rule set, so it always
	// falls back to genericRules.
	assessContentWorking = "⠋ Thinking… (working)\n"
	assessContentIdle    = "line one\n❯"
)

// newImmediateTracker returns a tracker whose confirm policies publish on the
// first confirming observation, so tests calling GetStatus a fixed number of
// times don't need clock choreography (fake-clock timing tests live in the
// status package itself).
func newImmediateTracker() *status.Tracker {
	return status.NewTracker(assess.NewEngine(), status.Options{
		ConfirmIdle:     status.ConfirmPolicy{Polls: 1},
		ConfirmApproval: status.ConfirmPolicy{Polls: 1},
	})
}

func TestSessionCache_FindPane(t *testing.T) {
	sc := &sessionCache{panes: []cachedPane{
		{input: classifier.PaneInput{PaneID: "%0", WindowName: "bash"}},
		{input: classifier.PaneInput{PaneID: "%1", WindowName: testToolClaude}, result: classifier.Result{IsAgent: true, Tool: testToolClaude}},
	}}

	pane := sc.findPane("%1")
	require.NotNil(t, pane)
	assert.Equal(t, testToolClaude, pane.input.WindowName)
	assert.Nil(t, sc.findPane("%9"))
}

func TestSessionCache_BestAgentPane(t *testing.T) {
	sc := &sessionCache{panes: []cachedPane{
		{input: classifier.PaneInput{PaneID: "%0", Activity: 300}, result: classifier.Result{IsAgent: false}},
		{input: classifier.PaneInput{PaneID: "%1", Activity: 100}, result: classifier.Result{IsAgent: true, Tool: testToolClaude}},
		{input: classifier.PaneInput{PaneID: "%2", Activity: 200}, result: classifier.Result{IsAgent: true, Tool: testToolCodex}},
	}}

	pane := sc.bestAgentPane()
	require.NotNil(t, pane)
	assert.Equal(t, "%2", pane.input.PaneID)
}

func TestDisambiguatePane(t *testing.T) {
	sc := &sessionCache{panes: []cachedPane{
		{input: classifier.PaneInput{PaneID: "%0", WindowName: testToolClaude, WorkDir: "/a", Activity: 100}, result: classifier.Result{IsAgent: true, Tool: testToolClaude}},
		{input: classifier.PaneInput{PaneID: "%1", WindowName: "myslug-work", WorkDir: "/b", Activity: 200}, result: classifier.Result{IsAgent: true, Tool: testToolCodex}},
		{input: classifier.PaneInput{PaneID: "%2", WindowName: "bash", WorkDir: "/c", Activity: 300}, result: classifier.Result{IsAgent: false}},
	}}

	assert.Equal(t, "%1", disambiguatePane(sc, "/b", "other").input.PaneID)
	assert.Equal(t, "%1", disambiguatePane(sc, "/missing", "myslug").input.PaneID)
	assert.Equal(t, "%1", disambiguatePane(sc, "/missing", "none").input.PaneID)
}

func TestSessionInfoFromPane(t *testing.T) {
	pane := &cachedPane{input: classifier.PaneInput{PaneID: "%5", WindowIndex: "2", WindowName: testToolClaude}, result: classifier.Result{Tool: testToolClaude}}
	info := sessionInfoFromPane("mysess", pane)
	require.NotNil(t, info)
	assert.Equal(t, "mysess", info.Name)
	assert.Equal(t, "2", info.WindowIndex)
	assert.Equal(t, "%5", info.PaneID)
	assert.Equal(t, testToolClaude, info.WindowName)
	assert.Equal(t, testToolClaude, info.DetectedTool)
	assert.Nil(t, sessionInfoFromPane("mysess", nil))
}

func TestWithStatusTracker(t *testing.T) {
	tracker := status.NewTracker(assess.NewEngine(), status.DefaultOptions())
	integ := NewFromPreviewMatchers(nil, WithStatusTracker(tracker))
	assert.Same(t, tracker, integ.tracker)

	// A nil tracker must not clobber the default.
	integ2 := NewFromPreviewMatchers(nil, WithStatusTracker(nil))
	assert.NotNil(t, integ2.tracker)
}

func TestWithStatusOptions(t *testing.T) {
	integ := NewFromPreviewMatchers(nil, WithStatusOptions(status.Options{ConfirmIdle: status.ConfirmPolicy{Polls: 5}}))
	require.NotNil(t, integ.tracker)
}

func TestWithMissingTolerance(t *testing.T) {
	integ := NewFromPreviewMatchers(nil, WithMissingTolerance(4))
	assert.Equal(t, 4, integ.missingTolerance)

	integ2 := NewFromPreviewMatchers(nil, WithMissingTolerance(0))
	assert.Equal(t, defaultMissingTolerance, integ2.missingTolerance, "a non-positive tolerance must not override the default")
}

func TestRefreshCache_ClassifiesAndCarriesState(t *testing.T) {
	lister := &fakePaneLister{panes: []classifier.PaneInput{
		{SessionName: "sess", PaneID: "%1", PanePID: 101, WindowIndex: "0", WindowName: testToolClaude, PaneTitle: testToolClaude, Activity: 100},
		{SessionName: "sess", PaneID: "%2", PanePID: 102, WindowIndex: "0", WindowName: "bash", Activity: 200},
	}}
	integ := New(classifier.New([]classifier.TitlePattern{titlePattern(testToolClaude, testToolClaude)}, nil, nil, nil), lister)
	integ.cache = map[string]*sessionCache{"sess": {panes: []cachedPane{{
		input: classifier.PaneInput{SessionName: "sess", PaneID: "%1", PanePID: 101},
		state: paneState{paneContent: "old", lastCaptureActive: 100},
	}}}}
	integ.tracker.Observe(paneKey("sess", "%old"), assess.Snapshot{Content: "x", Tool: "agent"})
	integ.limiters[paneKey("sess", "%old")] = terminal.NewRateLimiter(1)

	integ.RefreshCache()

	sc := integ.cache["sess"]
	require.NotNil(t, sc)
	require.Len(t, sc.panes, 2)
	assert.True(t, sc.findPane("%1").result.IsAgent)
	assert.False(t, sc.findPane("%2").result.IsAgent)
	assert.Equal(t, "old", sc.findPane("%1").state.paneContent)
	_, trackedStale := integ.tracker.DebugState(paneKey("sess", "%old"))
	assert.False(t, trackedStale, "tracker state for a pane key no longer in the active set must be pruned")
	assert.Empty(t, integ.limiters)
}

func TestRefreshCache_DoesNotClassifyShellPaneFromWindowName(t *testing.T) {
	lister := &fakePaneLister{panes: []classifier.PaneInput{
		{SessionName: "sess", PaneID: "%1", PanePID: 101, WindowIndex: "0", WindowName: testToolClaude, PaneTitle: testToolClaude},
		{SessionName: "sess", PaneID: "%2", PanePID: 102, WindowIndex: "0", WindowName: testToolClaude, PaneTitle: "bash"},
	}}
	integ := New(classifier.New([]classifier.TitlePattern{titlePattern(testToolClaude, testToolClaude)}, nil, nil, nil), lister)

	integ.RefreshCache()

	sc := integ.cache["sess"]
	require.NotNil(t, sc)
	assert.True(t, sc.findPane("%1").result.IsAgent)
	assert.False(t, sc.findPane("%2").result.IsAgent)
}

func TestRefreshCache_ReclassifiesNegativeResult(t *testing.T) {
	reader := &fakeProcessReader{tpgid: 200, comm: map[int]string{200: "zsh"}}
	lister := &fakePaneLister{panes: []classifier.PaneInput{
		{SessionName: "sess", PaneID: "%1", PanePID: 100, WindowIndex: "0", WindowName: "main"},
	}}
	integ := NewWithReader(classifier.New(toolPatterns(testToolClaude), reader, nil, nil), lister, reader)

	integ.RefreshCache()
	assert.False(t, integ.cache["sess"].findPane("%1").result.IsAgent)

	reader.comm[200] = testToolClaude
	integ.RefreshCache()
	assert.True(t, integ.cache["sess"].findPane("%1").result.IsAgent)
}

func TestRefreshCache_InvalidatesOnForegroundPIDChange(t *testing.T) {
	reader := &fakeProcessReader{tpgid: 200, comm: map[int]string{200: testToolClaude, 201: testToolCodex}}
	lister := &fakePaneLister{panes: []classifier.PaneInput{
		{SessionName: "sess", PaneID: "%1", PanePID: 100, WindowIndex: "0", WindowName: "main"},
	}}
	integ := NewWithReader(classifier.New(toolPatterns(testToolClaude, testToolCodex), reader, nil, nil), lister, reader)

	integ.RefreshCache()
	assert.Equal(t, testToolClaude, integ.cache["sess"].findPane("%1").result.Tool)

	reader.tpgid = 201
	integ.RefreshCache()
	assert.Equal(t, testToolCodex, integ.cache["sess"].findPane("%1").result.Tool)
}

func TestRefreshCache_ReclassifiesContentBasedPositive(t *testing.T) {
	// Verify that once the content limiter permits a re-check (simulated by
	// clearing the limiter), changed content causes reclassification.
	reader := &fakeProcessReader{tpgid: 200, comm: map[int]string{200: "bash"}}
	lister := &fakePaneLister{panes: []classifier.PaneInput{
		{SessionName: "sess", PaneID: "%1", PanePID: 100, WindowIndex: "0", WindowName: "main"},
	}}
	capture := &fakeCapture{content: "agent content"}
	scorer := &fakeScorer{scores: map[string]fakeScore{
		"agent content": {score: 6, categories: 3, tool: testToolClaude},
		"shell content": {score: 1, categories: 1},
	}}
	integ := NewWithReader(classifier.New(nil, reader, capture, scorer), lister, reader)

	integ.RefreshCache()
	pane := integ.cache["sess"].findPane("%1")
	require.NotNil(t, pane)
	assert.True(t, pane.result.IsAgent)
	assert.Equal(t, 3, pane.result.Tier)

	// Reset the content limiter to simulate the interval expiring, then change
	// the pane content so the next full Classify returns not-agent.
	integ.contentLimiters = make(map[string]*terminal.RateLimiter)
	capture.content = "shell content"
	integ.RefreshCache()
	pane = integ.cache["sess"].findPane("%1")
	require.NotNil(t, pane)
	assert.False(t, pane.result.IsAgent)
	assert.Equal(t, 2, capture.calls)
}

func TestRefreshCache_ContentLimiterSkipsTier3(t *testing.T) {
	// After the first full Classify (which runs Tier 3), subsequent RefreshCache
	// calls within contentCheckInterval must NOT call capture-pane again.
	reader := &fakeProcessReader{tpgid: 200, comm: map[int]string{200: "bash"}}
	lister := &fakePaneLister{panes: []classifier.PaneInput{
		{SessionName: "sess", PaneID: "%1", PanePID: 100, WindowIndex: "0", WindowName: "main"},
	}}
	capture := &fakeCapture{content: "agent content"}
	scorer := &fakeScorer{scores: map[string]fakeScore{
		"agent content": {score: 6, categories: 3, tool: testToolClaude},
	}}
	integ := NewWithReader(classifier.New(nil, reader, capture, scorer), lister, reader)

	integ.RefreshCache() // first call: Tier 3 runs, capture.calls == 1
	assert.Equal(t, 1, capture.calls)

	integ.RefreshCache() // second call within interval: limiter blocks Tier 3
	integ.RefreshCache() // third call
	assert.Equal(t, 1, capture.calls, "capture-pane must not be called again within contentCheckInterval")
}

func TestRefreshCache_TryLockPreventsStorm(t *testing.T) {
	// If RefreshCache is already running, a concurrent call must return
	// immediately without calling list-panes a second time.
	var listCalls atomic.Int32
	blockRefresh := make(chan struct{})
	lister := &blockingPaneLister{
		listFn: func() ([]classifier.PaneInput, error) {
			n := listCalls.Add(1)
			if n == 1 {
				<-blockRefresh // block the first call
			}
			return nil, nil
		},
	}
	integ := New(nil, lister)

	// Start a refresh that will block inside ListAllPanes.
	done := make(chan struct{})
	go func() {
		integ.RefreshCache()
		close(done)
	}()

	// Give the goroutine time to acquire the lock.
	time.Sleep(10 * time.Millisecond)

	// Second call should return immediately (TryLock fails).
	integ.RefreshCache()
	assert.Equal(t, int32(1), listCalls.Load(), "second RefreshCache must not call list-panes while first is running")

	// Unblock the first refresh.
	close(blockRefresh)
	<-done
}

func TestRefreshCache_UsesSharedProcessSnapshot(t *testing.T) {
	// Verify that process-tree children are queried once per RefreshCache call
	// regardless of how many panes are classified. Before the snapshot fix,
	// each ClassifyStable call would invoke Children() independently.
	var childrenCalls int
	reader := &countingProcessReader{
		ProcessReader: &fakeProcessReader{tpgid: 200, comm: map[int]string{200: "zsh"}},
		onChildren:    func() { childrenCalls++ },
	}
	lister := &fakePaneLister{panes: []classifier.PaneInput{
		{SessionName: "sess", PaneID: "%1", PanePID: 100, WindowIndex: "0", WindowName: "main"},
		{SessionName: "sess", PaneID: "%2", PanePID: 101, WindowIndex: "1", WindowName: "work"},
		{SessionName: "sess", PaneID: "%3", PanePID: 102, WindowIndex: "2", WindowName: "logs"},
	}}
	integ := NewWithReader(classifier.New(nil, reader, nil, nil), lister, reader)

	integ.RefreshCache()

	// SnapshotReader.Children is served from the in-memory map; only the
	// snapshot construction itself calls the underlying reader. The fake reader
	// always returns no children, so the snapshot map is built but returns nil
	// for every lookup — the important thing is the base reader is not called
	// again per-pane after snapshot construction.
	assert.Equal(t, 0, childrenCalls, "base Children must not be called per-pane when snapshot is available")
}

func TestRefreshCache_ResetsStateOnPIDChange(t *testing.T) {
	lister := &fakePaneLister{panes: []classifier.PaneInput{
		{SessionName: "sess", PaneID: "%1", PanePID: 202, WindowIndex: "0", WindowName: testToolClaude, PaneTitle: testToolClaude, Activity: 200},
	}}
	integ := New(classifier.New([]classifier.TitlePattern{titlePattern(testToolClaude, testToolClaude)}, nil, nil, nil), lister)
	integ.cache = map[string]*sessionCache{"sess": {panes: []cachedPane{{
		input: classifier.PaneInput{SessionName: "sess", PaneID: "%1", PanePID: 101},
		state: paneState{paneContent: "old", lastCaptureActive: 100},
	}}}}

	integ.RefreshCache()

	pane := integ.cache["sess"].findPane("%1")
	require.NotNil(t, pane)
	assert.Empty(t, pane.state.paneContent)
	assert.Zero(t, pane.state.lastCaptureActive)
}

func TestDiscoverSession(t *testing.T) {
	integ := New(nil, nil)
	integ.cache = map[string]*sessionCache{"my-session": {panes: []cachedPane{
		{input: classifier.PaneInput{PaneID: "%1", WindowIndex: "0", WindowName: testToolClaude, WorkDir: "/a", Activity: 100}, result: classifier.Result{IsAgent: true, Tool: testToolClaude}},
		{input: classifier.PaneInput{PaneID: "%2", WindowIndex: "1", WindowName: testToolCodex, WorkDir: "/b", Activity: 200}, result: classifier.Result{IsAgent: true, Tool: testToolCodex}},
	}}}
	integ.cacheTime = time.Now()

	info, err := integ.DiscoverSession(context.Background(), "my-session", map[string]string{SessionPathKey: "/b"})
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "%2", info.PaneID)

	info, err = integ.DiscoverSession(context.Background(), "my-session", map[string]string{"tmux_window": "0"})
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "%1", info.PaneID)
}

func TestDiscoverAllPanes(t *testing.T) {
	integ := New(nil, nil)
	integ.cache = map[string]*sessionCache{"multi-sess": {panes: []cachedPane{
		{input: classifier.PaneInput{PaneID: "%1", WindowIndex: "0", WindowName: testToolClaude}, result: classifier.Result{IsAgent: true, Tool: testToolClaude}},
		{input: classifier.PaneInput{PaneID: "%2", WindowIndex: "0", WindowName: "bash"}, result: classifier.Result{IsAgent: false}},
		{input: classifier.PaneInput{PaneID: "%3", WindowIndex: "1", WindowName: testToolCodex}, result: classifier.Result{IsAgent: true, Tool: testToolCodex}},
	}}}
	integ.cacheTime = time.Now()

	infos, err := integ.DiscoverAllPanes(context.Background(), "multi-sess", nil)
	require.NoError(t, err)
	require.Len(t, infos, 2)
	assert.Equal(t, "%1", infos[0].PaneID)
	assert.Equal(t, "%3", infos[1].PaneID)
}

func TestDiscoverAllPanes_Matching(t *testing.T) {
	integ := New(nil, nil)
	integ.cache = map[string]*sessionCache{
		"multi-sess": {panes: []cachedPane{
			agentCachedPane("%1", "0", testToolClaude),
			agentCachedPane("%2", "1", testToolCodex),
		}},
		"foo-bar": {panes: []cachedPane{
			agentCachedPane("%3", "0", testToolClaude),
		}},
	}
	integ.cacheTime = time.Now()

	ctx := context.Background()

	t.Run("unknown session returns nil", func(t *testing.T) {
		infos, err := integ.DiscoverAllPanes(ctx, "nonexistent", nil)
		require.NoError(t, err)
		assert.Nil(t, infos)
	})

	t.Run("stale cache returns nil", func(t *testing.T) {
		integ.cacheTime = time.Now().Add(-5 * time.Second)
		infos, err := integ.DiscoverAllPanes(ctx, "multi-sess", nil)
		require.NoError(t, err)
		assert.Nil(t, infos)
		integ.cacheTime = time.Now()
	})

	t.Run("similar slug does not cross match", func(t *testing.T) {
		infos, err := integ.DiscoverAllPanes(ctx, "foo", nil)
		require.NoError(t, err)
		assert.Nil(t, infos, "slug foo must not match tmux session foo-bar")
	})

	t.Run("hyphenated exact slug still found", func(t *testing.T) {
		infos, err := integ.DiscoverAllPanes(ctx, "foo-bar", nil)
		require.NoError(t, err)
		require.Len(t, infos, 1)
		assert.Equal(t, "foo-bar", infos[0].Name)
		assert.Equal(t, "%3", infos[0].PaneID)
	})

	t.Run("metadata tmux_session match returns named session", func(t *testing.T) {
		infos, err := integ.DiscoverAllPanes(ctx, "myslug", map[string]string{"tmux_session": "multi-sess"})
		require.NoError(t, err)
		require.Len(t, infos, 2)
		assert.Equal(t, "multi-sess", infos[0].Name)
		assert.Equal(t, "%1", infos[0].PaneID)
	})
}

func TestDiscoverSession_MetaTmuxSessionCompatibility(t *testing.T) {
	ctx := context.Background()

	t.Run("explicit display name differs from slug", func(t *testing.T) {
		integ := New(nil, nil)
		integ.cache = map[string]*sessionCache{
			"My Feature": {panes: []cachedPane{agentCachedPane("%1", "0", testToolClaude)}},
		}
		integ.cacheTime = time.Now()

		info, err := integ.DiscoverSession(ctx, "my-feature", map[string]string{})
		require.NoError(t, err)
		assert.Nil(t, info, "slug lookup should fail when tmux session name differs from slug")

		info, err = integ.DiscoverSession(ctx, "my-feature", map[string]string{
			SessionPathKey: "/some/path",
			"tmux_session": "My Feature",
		})
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "My Feature", info.Name)
		assert.Equal(t, "%1", info.PaneID)
	})

	t.Run("stale metadata falls back to slug lookup", func(t *testing.T) {
		integ := New(nil, nil)
		integ.cache = map[string]*sessionCache{
			"new-name": {panes: []cachedPane{agentCachedPane("%2", "0", testToolClaude)}},
		}
		integ.cacheTime = time.Now()

		info, err := integ.DiscoverSession(ctx, "new-name", map[string]string{"tmux_session": "old-name"})
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "new-name", info.Name)
		assert.Equal(t, "%2", info.PaneID)
	})

	t.Run("hive session tag maps renamed tmux session", func(t *testing.T) {
		integ := New(nil, nil)
		pane := agentCachedPane("%3", "0", testToolClaude)
		pane.input.HiveSession = "my-feature"
		integ.cache = map[string]*sessionCache{"My Feature": {panes: []cachedPane{pane}}}
		integ.cacheTime = time.Now()

		info, err := integ.DiscoverSession(ctx, "my-feature", map[string]string{})
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "My Feature", info.Name)
		assert.Equal(t, "%3", info.PaneID)
	})
}

func TestGetStatus_ExplicitNonAgentPaneMissing(t *testing.T) {
	recorder := &fakeCaptureRecorder{}
	integ := New(nil, nil)
	integ.tracker = newImmediateTracker()
	integ.recorder = recorder
	integ.cache = map[string]*sessionCache{"sess": {panes: []cachedPane{
		{input: classifier.PaneInput{PaneID: "%1"}, result: classifier.Result{IsAgent: false}},
		{input: classifier.PaneInput{PaneID: "%2"}, result: classifier.Result{IsAgent: true, Tool: testToolClaude}},
	}}}
	integ.cacheTime = time.Now()

	status, err := integ.GetStatus(context.Background(), &terminal.SessionInfo{Name: "sess", PaneID: "%1"})
	require.NoError(t, err)
	assert.Equal(t, terminal.StatusMissing, status)
	assert.Empty(t, recorder.observations)
}

func TestGetStatus_UsesPaneKeysAndCapture(t *testing.T) {
	capture := &fakeCapture{content: "❯"}
	integ := New(nil, nil)
	integ.tracker = newImmediateTracker()
	integ.capture = capture
	integ.cache = map[string]*sessionCache{"sess": {panes: []cachedPane{{
		input:  classifier.PaneInput{PaneID: "%1", WindowIndex: "0", WindowName: testToolClaude, Activity: 10},
		result: classifier.Result{IsAgent: true, Tool: testToolClaude},
	}}}}
	integ.cacheTime = time.Now()

	info := &terminal.SessionInfo{Name: "sess", PaneID: "%1"}
	got, err := integ.GetStatus(context.Background(), info)
	require.NoError(t, err)
	assert.Equal(t, terminal.StatusReady, got)
	assert.Equal(t, "❯", info.PaneContent)
	assert.Equal(t, testToolClaude, info.DetectedTool)
	_, tracked := integ.tracker.DebugState(paneKey("sess", "%1"))
	assert.True(t, tracked)
	assert.NotNil(t, integ.limiters[paneKey("sess", "%1")])
	assert.Equal(t, 1, capture.calls)
}

func TestGetStatus_RecordsFreshCapture(t *testing.T) {
	capture := &fakeCapture{content: "❯"}
	recorder := &fakeCaptureRecorder{}
	integ := New(nil, nil)
	integ.tracker = newImmediateTracker()
	integ.capture = capture
	integ.recorder = recorder
	integ.cache = map[string]*sessionCache{"sess": {panes: []cachedPane{{
		input:  classifier.PaneInput{PaneID: "%1", Activity: 10},
		result: classifier.Result{IsAgent: true, Tool: testToolClaude},
	}}}}

	got, err := integ.GetStatus(context.Background(), &terminal.SessionInfo{Name: "sess", PaneID: "%1"})
	require.NoError(t, err)
	assert.Equal(t, terminal.StatusReady, got)
	require.Len(t, recorder.observations, 1)
	assert.Equal(t, CaptureObservation{
		SessionName: "sess",
		PaneID:      "%1",
		Tool:        testToolClaude,
		Content:     "❯",
		Status:      terminal.StatusReady,
		RuleID:      "claude/prompt-glyph",
		Signals:     []assess.Signal{{RuleID: "claude/prompt-glyph", Region: "bottomLines", Matched: "❯"}},
	}, recorder.observations[0])

	_, err = integ.GetStatus(context.Background(), &terminal.SessionInfo{Name: "sess", PaneID: "%1"})
	require.NoError(t, err)
	assert.Len(t, recorder.observations, 1, "cached content must not be recorded again")
}

func TestGetStatus_RecorderErrorIsNonFatal(t *testing.T) {
	integ := New(nil, nil)
	integ.tracker = newImmediateTracker()
	integ.capture = &fakeCapture{content: "❯"}
	integ.recorder = &fakeCaptureRecorder{err: errors.New("disk full")}
	integ.cache = map[string]*sessionCache{"sess": {panes: []cachedPane{{
		input:  classifier.PaneInput{PaneID: "%1", Activity: 10},
		result: classifier.Result{IsAgent: true, Tool: testToolClaude},
	}}}}

	got, err := integ.GetStatus(context.Background(), &terminal.SessionInfo{Name: "sess", PaneID: "%1"})
	require.NoError(t, err)
	assert.Equal(t, terminal.StatusReady, got)
}

// TestGetStatus_UnchangedContentStillObserves pins the central fix of this
// redesign: an unchanged frame between polls is itself a confirming
// observation for a pending idle candidate, not a short-circuited no-op.
// RefreshCache bumps refreshGeneration every successful poll regardless of
// whether tmux output changed, and GetStatus must forward every one of those
// generations to Tracker.Observe.
func TestGetStatus_UnchangedContentStillObserves(t *testing.T) {
	capture := &fakeCapture{content: assessContentWorking}
	integ := New(nil, nil)
	integ.capture = capture
	integ.tracker = status.NewTracker(assess.NewEngine(), status.Options{
		ConfirmIdle: status.ConfirmPolicy{Polls: 2},
	})
	integ.cache = map[string]*sessionCache{"sess": {panes: []cachedPane{{
		input:  classifier.PaneInput{PaneID: "%1", Activity: 1},
		result: classifier.Result{IsAgent: true, Tool: "agent"},
	}}}}
	info := &terminal.SessionInfo{Name: "sess", PaneID: "%1"}

	// Poll 1 (generation 0): brand-new key, busy content -> first observation
	// publishes immediately, no debounce needed yet.
	got, err := integ.GetStatus(context.Background(), info)
	require.NoError(t, err)
	require.Equal(t, terminal.StatusActive, got)

	// Poll 2 (generation 1): content switches to idle and pane activity
	// advances, forcing a fresh capture. This starts the 2-poll idle
	// candidate -- one confirming poll is not enough to publish yet.
	capture.content = assessContentIdle
	integ.cache["sess"].panes[0].input.Activity = 2
	integ.limiters = make(map[string]*terminal.RateLimiter) // simulate the capture rate limiter's interval elapsing
	integ.refreshGeneration = 1
	got, err = integ.GetStatus(context.Background(), info)
	require.NoError(t, err)
	require.Equal(t, terminal.StatusActive, got, "one idle poll must not yet flip the published status")

	// Poll 3 (generation 2): activity is unchanged from poll 2, so GetStatus
	// serves the cached content through the cheap path -- no new
	// capture-pane call happens. Before this redesign, identical content
	// short-circuited before ever reaching the tracker, so the idle
	// candidate's second confirming poll would never happen. RefreshCache
	// still ran again (a new generation) even though nothing changed on the
	// wire, and Observe must still see it.
	integ.refreshGeneration = 2
	got, err = integ.GetStatus(context.Background(), info)
	require.NoError(t, err)
	assert.Equal(t, terminal.StatusReady, got, "unchanged content across a new refresh generation must still confirm the idle candidate")
}

// TestRefreshCache_TransientFailureServesStaleCache pins the transport-level
// missing tolerance: one list-panes failure must serve the last-known cache
// (no missing flash), and recovery resets the failure counter.
func TestRefreshCache_TransientFailureServesStaleCache(t *testing.T) {
	lister := &flakyPaneLister{panes: []classifier.PaneInput{
		{SessionName: "sess", PaneID: "%1", PanePID: 101, WindowIndex: "0", WindowName: testToolClaude, PaneTitle: testToolClaude},
	}}
	integ := New(classifier.New([]classifier.TitlePattern{titlePattern(testToolClaude, testToolClaude)}, nil, nil, nil), lister)
	require.Equal(t, 2, integ.missingTolerance, "default tolerance should match terminal.status.confirm.missing.polls' default of 2")

	integ.RefreshCache()
	info, err := integ.DiscoverSession(context.Background(), "sess", nil)
	require.NoError(t, err)
	require.NotNil(t, info, "cache must be fresh after a successful refresh")

	lister.fail = true
	integ.RefreshCache() // failure #1: below missingTolerance, tolerated

	info, err = integ.DiscoverSession(context.Background(), "sess", nil)
	require.NoError(t, err)
	assert.NotNil(t, info, "a single transient list-panes failure must not flash the session missing")
	assert.NotNil(t, integ.cache["sess"], "cache must be kept across a tolerated failure")

	lister.fail = false
	integ.RefreshCache() // success resets the failure counter
	assert.Equal(t, 0, integ.refreshFailures)

	info, err = integ.DiscoverSession(context.Background(), "sess", nil)
	require.NoError(t, err)
	assert.NotNil(t, info, "recovery after a tolerated failure must never have flashed missing")
}

// TestRefreshCache_ClearsCacheAfterToleranceExceeded pins the other half of
// the missing-tolerance policy: reaching missingTolerance clears the cache
// and publishes missing, same as the pre-Phase-4 unconditional-clear behavior.
func TestRefreshCache_ClearsCacheAfterToleranceExceeded(t *testing.T) {
	lister := &flakyPaneLister{panes: []classifier.PaneInput{
		{SessionName: "sess", PaneID: "%1", PanePID: 101, WindowIndex: "0", WindowName: testToolClaude, PaneTitle: testToolClaude},
	}}
	integ := New(classifier.New([]classifier.TitlePattern{titlePattern(testToolClaude, testToolClaude)}, nil, nil, nil), lister)

	integ.RefreshCache()
	require.NotNil(t, integ.cache["sess"])

	lister.fail = true
	integ.RefreshCache() // failure #1: tolerated
	integ.RefreshCache() // failure #2: >= missingTolerance, cache cleared

	info, err := integ.DiscoverSession(context.Background(), "sess", nil)
	require.NoError(t, err)
	assert.Nil(t, info, "two consecutive failures (missingTolerance=2) must clear the cache and publish missing")
	assert.Empty(t, integ.cache)
}

func toolPatterns(tools ...string) []classifier.TitlePattern {
	out := make([]classifier.TitlePattern, 0, len(tools))
	for _, t := range tools {
		out = append(out, titlePattern(t, t))
	}
	return out
}

func titlePattern(pattern, tool string) classifier.TitlePattern {
	return classifier.TitlePattern{Pattern: regexp.MustCompile(pattern), Tool: tool}
}

func agentCachedPane(paneID, windowIndex, tool string) cachedPane {
	return cachedPane{
		input: classifier.PaneInput{
			PaneID:      paneID,
			WindowIndex: windowIndex,
			WindowName:  tool,
		},
		result: classifier.Result{IsAgent: true, Tool: tool},
	}
}

type fakePaneLister struct{ panes []classifier.PaneInput }

func (f *fakePaneLister) ListAllPanes() ([]classifier.PaneInput, error) { return f.panes, nil }

type blockingPaneLister struct {
	listFn func() ([]classifier.PaneInput, error)
}

func (b *blockingPaneLister) ListAllPanes() ([]classifier.PaneInput, error) { return b.listFn() }

// flakyPaneLister returns panes normally, or a transport error while fail is true.
type flakyPaneLister struct {
	panes []classifier.PaneInput
	fail  bool
}

func (f *flakyPaneLister) ListAllPanes() ([]classifier.PaneInput, error) {
	if f.fail {
		return nil, errors.New("list-panes failed")
	}
	return f.panes, nil
}

// countingProcessReader wraps a ProcessReader and invokes a callback on each
// Children call so tests can assert how many times the OS is queried.
type countingProcessReader struct {
	process.ProcessReader
	onChildren func()
}

func (c *countingProcessReader) Children(pid int) ([]int, error) {
	c.onChildren()
	return c.ProcessReader.Children(pid)
}

type fakeProcessReader struct {
	tpgid int
	comm  map[int]string
}

func (f *fakeProcessReader) TPGID(int) (int, error) { return f.tpgid, nil }
func (f *fakeProcessReader) Comm(pid int) string    { return f.comm[pid] }
func (f *fakeProcessReader) Cmdline(pid int) ([]string, error) {
	if comm := f.comm[pid]; comm != "" {
		return []string{comm}, nil
	}
	return nil, nil
}
func (f *fakeProcessReader) Environ(int) map[string]string { return nil }
func (f *fakeProcessReader) Children(int) ([]int, error)   { return nil, nil }

type fakeCapture struct {
	content string
	calls   int
}

func (f *fakeCapture) CapturePane(context.Context, string) (string, error) {
	f.calls++
	return f.content, nil
}

type fakeCaptureRecorder struct {
	observations []CaptureObservation
	err          error
}

func (f *fakeCaptureRecorder) Record(observation CaptureObservation) error {
	f.observations = append(f.observations, observation)
	return f.err
}

type fakeScore struct {
	score      int
	categories int
	tool       string
}

type fakeScorer struct {
	scores map[string]fakeScore
}

func (f *fakeScorer) Score(content string) (int, int, string) {
	score := f.scores[content]
	return score.score, score.categories, score.tool
}
