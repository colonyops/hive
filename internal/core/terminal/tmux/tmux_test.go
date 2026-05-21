package tmux

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/colonyops/hive/internal/core/terminal/classifier"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testToolClaude = "claude"
	testToolCodex  = "codex"
)

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

func TestRefreshCache_ClassifiesAndCarriesState(t *testing.T) {
	lister := &fakePaneLister{panes: []classifier.PaneInput{
		{SessionName: "sess", PaneID: "%1", PanePID: 101, WindowIndex: "0", WindowName: testToolClaude, PaneTitle: testToolClaude, Activity: 100},
		{SessionName: "sess", PaneID: "%2", PanePID: 102, WindowIndex: "0", WindowName: "bash", Activity: 200},
	}}
	integ := New(classifier.New([]classifier.TitlePattern{titlePattern(testToolClaude, testToolClaude)}, nil, nil, nil), lister)
	integ.cache = map[string]*sessionCache{"sess": {panes: []cachedPane{{
		input: classifier.PaneInput{SessionName: "sess", PaneID: "%1", PanePID: 101},
		state: paneState{paneContent: "old", cachedStatus: terminal.StatusReady, lastCaptureActive: 100},
	}}}}
	integ.trackers[paneKey("sess", "%old")] = terminal.NewStateTracker()
	integ.limiters[paneKey("sess", "%old")] = terminal.NewRateLimiter(1)

	integ.RefreshCache()

	sc := integ.cache["sess"]
	require.NotNil(t, sc)
	require.Len(t, sc.panes, 2)
	assert.True(t, sc.findPane("%1").result.IsAgent)
	assert.False(t, sc.findPane("%2").result.IsAgent)
	assert.Equal(t, "old", sc.findPane("%1").state.paneContent)
	assert.Empty(t, integ.trackers)
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
	integ := NewWithReader(classifier.New(nil, reader, nil, nil), lister, reader)

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
	integ := NewWithReader(classifier.New(nil, reader, nil, nil), lister, reader)

	integ.RefreshCache()
	assert.Equal(t, testToolClaude, integ.cache["sess"].findPane("%1").result.Tool)

	reader.tpgid = 201
	integ.RefreshCache()
	assert.Equal(t, testToolCodex, integ.cache["sess"].findPane("%1").result.Tool)
}

func TestRefreshCache_ResetsStateOnPIDChange(t *testing.T) {
	lister := &fakePaneLister{panes: []classifier.PaneInput{
		{SessionName: "sess", PaneID: "%1", PanePID: 202, WindowIndex: "0", WindowName: testToolClaude, PaneTitle: testToolClaude, Activity: 200},
	}}
	integ := New(classifier.New([]classifier.TitlePattern{titlePattern(testToolClaude, testToolClaude)}, nil, nil, nil), lister)
	integ.cache = map[string]*sessionCache{"sess": {panes: []cachedPane{{
		input: classifier.PaneInput{SessionName: "sess", PaneID: "%1", PanePID: 101},
		state: paneState{paneContent: "old", cachedStatus: terminal.StatusReady, lastCaptureActive: 100},
	}}}}

	integ.RefreshCache()

	pane := integ.cache["sess"].findPane("%1")
	require.NotNil(t, pane)
	assert.Empty(t, pane.state.paneContent)
	assert.Empty(t, pane.state.cachedStatus)
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
	integ := New(nil, nil)
	integ.cache = map[string]*sessionCache{"sess": {panes: []cachedPane{
		{input: classifier.PaneInput{PaneID: "%1"}, result: classifier.Result{IsAgent: false}},
		{input: classifier.PaneInput{PaneID: "%2"}, result: classifier.Result{IsAgent: true, Tool: testToolClaude}},
	}}}
	integ.cacheTime = time.Now()

	status, err := integ.GetStatus(context.Background(), &terminal.SessionInfo{Name: "sess", PaneID: "%1"})
	require.NoError(t, err)
	assert.Equal(t, terminal.StatusMissing, status)
}

func TestGetStatus_UsesPaneKeysAndCapture(t *testing.T) {
	capture := &fakeCapture{content: "❯"}
	integ := New(nil, nil)
	integ.capture = capture
	integ.cache = map[string]*sessionCache{"sess": {panes: []cachedPane{{
		input:  classifier.PaneInput{PaneID: "%1", WindowIndex: "0", WindowName: testToolClaude, Activity: 10},
		result: classifier.Result{IsAgent: true, Tool: testToolClaude},
	}}}}
	integ.cacheTime = time.Now()

	info := &terminal.SessionInfo{Name: "sess", PaneID: "%1"}
	status, err := integ.GetStatus(context.Background(), info)
	require.NoError(t, err)
	assert.Equal(t, terminal.StatusReady, status)
	assert.Equal(t, "❯", info.PaneContent)
	assert.Equal(t, testToolClaude, info.DetectedTool)
	assert.NotNil(t, integ.trackers[paneKey("sess", "%1")])
	assert.NotNil(t, integ.limiters[paneKey("sess", "%1")])
	assert.Equal(t, 1, capture.calls)
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
