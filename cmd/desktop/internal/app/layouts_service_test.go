package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
)

func TestParseTabRef(t *testing.T) {
	for in, want := range map[string]TabRef{
		"drive":    {View: TabViewDrive},
		"diff":     {View: TabViewDiff},
		"tasks":    {View: TabViewTasks},
		"terminal": {View: TabViewTerminal},
		"@3":       {View: TabViewTerminal, Window: "@3"},
		"logs":     {View: TabViewTerminal, Window: "logs"},
		"Drive":    {View: TabViewTerminal, Window: "Drive"},
	} {
		got, err := parseTabRef(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
	got, err := parseTabRef(" logs ")
	require.NoError(t, err)
	assert.Equal(t, TabRef{View: TabViewTerminal, Window: "logs"}, got, "spaces around a reference are trimmed")
	_, err = parseTabRef("  ")
	assert.Equal(t, KindInvalid, KindOf(err))
}

func TestTabToolsResolveWindowsPerCaller(t *testing.T) {
	f := newDriveFixture(t, siteSession)
	ctx := t.Context()
	code := siteSession.Path

	require.NoError(t, f.layouts.FocusTab(ctx, code, "logs"))
	require.NoError(t, f.layouts.FocusTab(ctx, code, "diff"))

	// The UI selects a focused window in tmux itself, and only while the
	// session is on screen; Go publishes the ask and touches no window.
	assert.Equal(t, []events.SessionLayoutRequested{
		{HiveSession: "abc123", Space: "acme/site", Verb: "focus", View: "terminal", Window: "@4"},
		{HiveSession: "abc123", Space: "acme/site", Verb: "focus", View: "diff"},
	}, f.signals.waitTabs(t, 2), "a window name reaches the UI as its id")

	for name, tc := range map[string]struct {
		call func() error
		kind Kind
	}{
		"terminal alone in a Code session": {func() error { return f.layouts.FocusTab(ctx, code, "terminal") }, KindInvalid},
		"a window in a chat":               {func() error { return f.layouts.FocusTab(ctx, "1", "@3") }, KindInvalid},
		"diff in a chat":                   {func() error { return f.layouts.ClosePanel(ctx, "1", "diff") }, KindInvalid},
		"a ghost window":                   {func() error { return f.layouts.TurnSplit(ctx, code, "ghost") }, KindNotFound},
		"an empty reference":               {func() error { return f.layouts.TurnSplit(ctx, code, "") }, KindInvalid},
		"the shared space":                 {func() error { return f.layouts.FocusTab(ctx, "global", "drive") }, KindInvalid},
	} {
		assert.Equal(t, tc.kind, KindOf(tc.call()), name)
	}
	assert.Len(t, f.signals.Tabs(), 2, "a refused call publishes nothing")

	require.NoError(t, f.layouts.FocusTab(ctx, "1", "terminal"), "a chat names its one terminal")
	require.NoError(t, f.layouts.FocusTab(ctx, "1", "drive"))
}

func TestMoveTabPublishesTheVerbAndAnchor(t *testing.T) {
	f := newDriveFixture(t, siteSession)
	ctx := t.Context()

	require.NoError(t, f.layouts.MoveTab(ctx, MoveTabRequest{Session: siteSession.Path, Tab: "logs", Placement: "below", Anchor: "diff"}))
	require.NoError(t, f.layouts.MoveTab(ctx, MoveTabRequest{Session: siteSession.Path, Tab: "diff"}))
	assert.Equal(t, []events.SessionLayoutRequested{
		{HiveSession: "abc123", Space: "acme/site", Verb: "move", View: "terminal", Window: "@4", Placement: "below", AnchorView: "diff"},
		{HiveSession: "abc123", Space: "acme/site", Verb: "move", View: "diff", Placement: "behind"},
	}, f.signals.waitTabs(t, 2))

	err := f.layouts.MoveTab(ctx, MoveTabRequest{Session: siteSession.Path, Tab: "diff", Placement: "above"})
	assert.Equal(t, KindInvalid, KindOf(err))
	err = f.layouts.MoveTab(ctx, MoveTabRequest{Session: siteSession.Path, Tab: "diff", Anchor: "terminal"})
	assert.Equal(t, KindInvalid, KindOf(err), "an anchor window must be named")
}

func TestOpenTabPublishesTheAnchorAndBelow(t *testing.T) {
	f := newDriveFixture(t, siteSession)

	_, err := f.layouts.OpenTab(t.Context(), OpenTabRequest{Session: siteSession.Path, View: "tasks", Placement: "below", Anchor: "@3"})
	require.NoError(t, err)
	assert.Equal(t, []events.SessionLayoutRequested{
		{HiveSession: "abc123", Space: "acme/site", Verb: "open", View: "tasks", Placement: "below", AnchorView: "terminal", AnchorWindow: "@3"},
	}, f.signals.waitTabs(t, 1))

	_, err = f.layouts.OpenTab(t.Context(), OpenTabRequest{Session: siteSession.Path, View: "terminal", Anchor: "ghost"})
	assert.Equal(t, KindNotFound, KindOf(err))
	assert.Empty(t, f.terminals.created, "a refused anchor starts no window")
}

func TestResizeSplitKeepsTheShareInRange(t *testing.T) {
	f := newDriveFixture(t, siteSession)
	ctx := t.Context()

	for _, share := range []float64{0.1, 0.19, 0.81, 1, 0, -1} {
		assert.Equal(t, KindInvalid, KindOf(f.layouts.ResizeSplit(ctx, siteSession.Path, "diff", share)), "%v", share)
	}
	require.NoError(t, f.layouts.ResizeSplit(ctx, siteSession.Path, "diff", 0.2))
	require.NoError(t, f.layouts.ResizeSplit(ctx, siteSession.Path, "diff", 0.8))
	require.NoError(t, f.layouts.ClosePanel(ctx, siteSession.Path, "tasks"))
	require.NoError(t, f.layouts.TurnSplit(ctx, siteSession.Path, "tasks"))
	assert.Equal(t, []events.SessionLayoutRequested{
		{HiveSession: "abc123", Space: "acme/site", Verb: "resize", View: "diff", Share: 0.2},
		{HiveSession: "abc123", Space: "acme/site", Verb: "resize", View: "diff", Share: 0.8},
		{HiveSession: "abc123", Space: "acme/site", Verb: "close_panel", View: "tasks"},
		{HiveSession: "abc123", Space: "acme/site", Verb: "turn", View: "tasks"},
	}, f.signals.waitTabs(t, 4))
}

func TestCloseTabKillsATerminalWindowWithoutAnEvent(t *testing.T) {
	f := newDriveFixture(t, siteSession)
	ctx := t.Context()
	code := siteSession.Path

	require.NoError(t, f.layouts.CloseTab(ctx, code, "terminal", "logs"))
	assert.Equal(t, []string{"fix-login @4"}, f.terminals.closed, "a name resolves to the id before the kill")
	f.signals.requireNoUpdates(t, "the reconcile removes the tab, not an event")
	assert.Empty(t, f.signals.Tabs())

	assert.Equal(t, KindInvalid, KindOf(f.layouts.CloseTab(ctx, code, "terminal", "")), "a terminal needs its window")
	assert.Equal(t, KindInvalid, KindOf(f.layouts.CloseTab(ctx, code, "diff", "@3")), "window applies to the terminal view only")
	err := f.layouts.CloseTab(ctx, "1", "terminal", "")
	assert.Equal(t, KindInvalid, KindOf(err))
	assert.Contains(t, err.Error(), "stop it from its row")
	assert.Equal(t, KindNotFound, KindOf(f.layouts.CloseTab(ctx, code, "terminal", "ghost")))
	assert.Len(t, f.terminals.closed, 1)
}

func TestLayoutIsUnknownBeforeAReportAndJoinsNamesAfterOne(t *testing.T) {
	f := newDriveFixture(t, siteSession)
	ctx := t.Context()
	code := siteSession.Path

	view, err := f.layouts.Layout(ctx, code)
	require.NoError(t, err)
	assert.Equal(t, LayoutView{Panels: []LayoutPanel{}, Splits: []LayoutSplit{}, Note: layoutUnknownNote}, view)

	require.NoError(t, f.layouts.Report(LayoutKindCode, "abc123", map[string]PanelLayoutReport{
		"abc123": {
			FocusedPanel: "p1",
			Root: PanelNodeReport{
				Kind: "split", ID: "s1", Axis: "row", Ratio: 0.5,
				Children: []PanelNodeReport{
					{Kind: "panel", ID: "p0", ActiveTab: "t1", Tabs: []PanelTabReport{
						{ID: "t0", Kind: "docs"},
						{ID: "t1", Kind: "window", WindowID: "@4"},
					}},
					{Kind: "panel", ID: "p1", ActiveTab: "t2", Tabs: []PanelTabReport{
						{ID: "t2", Kind: "diff"},
						{ID: "t3", Kind: "window", WindowID: "@9"},
					}},
				},
			},
		},
	}))

	view, err = f.layouts.Layout(ctx, code)
	require.NoError(t, err)
	assert.Equal(t, LayoutView{
		Known: true, OnScreen: true, FocusedPanel: "p1",
		Panels: []LayoutPanel{
			{ID: "p0", Tabs: []LayoutTab{
				{Tab: "drive", Kind: "drive"},
				{Tab: "@4", Kind: "terminal", Name: "logs", Shown: true},
			}},
			{ID: "p1", Tabs: []LayoutTab{
				{Tab: "diff", Kind: "diff", Shown: true},
				{Tab: "@9", Kind: "terminal"},
			}},
		},
		Splits: []LayoutSplit{{ID: "s1", Axis: "row", Ratio: 0.5, First: "p0", Second: "p1"}},
	}, view, "a window tmux does not list keeps an empty name")

	require.NoError(t, f.layouts.Report(LayoutKindCode, "other", map[string]PanelLayoutReport{"abc123": {Root: PanelNodeReport{Kind: "panel", ID: "p0"}}}))
	view, err = f.layouts.Layout(ctx, code)
	require.NoError(t, err)
	assert.True(t, view.Known)
	assert.False(t, view.OnScreen, "another session is on screen")

	require.NoError(t, f.layouts.Report(LayoutKindCode, "", nil))
	view, err = f.layouts.Layout(ctx, code)
	require.NoError(t, err)
	assert.False(t, view.Known, "a report replaces the whole kind")
}

func TestLayoutOfAChatReadsTheChatsKind(t *testing.T) {
	f := newDriveFixture(t, siteSession)
	require.NoError(t, f.layouts.Report(LayoutKindChats, "1", map[string]PanelLayoutReport{
		"1": {FocusedPanel: "p0", Root: PanelNodeReport{Kind: "panel", ID: "p0", ActiveTab: "t0", Tabs: []PanelTabReport{
			{ID: "t0", Kind: "window", WindowID: "%1"}, {ID: "t1", Kind: "docs"},
		}}},
	}))

	view, err := f.layouts.Layout(t.Context(), "1")
	require.NoError(t, err)
	assert.Equal(t, []LayoutPanel{{ID: "p0", Tabs: []LayoutTab{
		{Tab: "terminal", Kind: "terminal", Shown: true},
		{Tab: "drive", Kind: "drive"},
	}}}, view.Panels, "a chat's terminal is spelled terminal and has no name here")

	view, err = f.layouts.Layout(t.Context(), siteSession.Path)
	require.NoError(t, err)
	assert.False(t, view.Known, "a hive session reads the code kind, not the chats one")

	_, err = f.layouts.Layout(t.Context(), "global")
	assert.Equal(t, KindInvalid, KindOf(err))
}

func TestLayoutsReportRefusesAnUnknownKind(t *testing.T) {
	s := &LayoutsService{}
	assert.Equal(t, KindInvalid, KindOf(s.Report("tabs", "", nil)))
	_, _, known := s.Reported("tabs", "x")
	assert.False(t, known)
}
