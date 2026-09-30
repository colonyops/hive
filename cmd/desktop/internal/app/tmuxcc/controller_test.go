package tmuxcc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func seededController() *controller {
	c := newController()
	c.set([]Window{
		{ID: "@1", Name: "claude", Active: true, ActivePane: "%1", Width: 120, Height: 40, Layout: singlePaneLayout("%1", 120, 40)},
		{ID: "@2", Name: "shell", ActivePane: "%2", Width: 120, Height: 40, Layout: singlePaneLayout("%2", 120, 40)},
	})
	return c
}

func TestControllerNotifications(t *testing.T) {
	t.Parallel()

	t.Run("window add is a placeholder until reconcile", func(t *testing.T) {
		t.Parallel()
		c := seededController()
		require.Equal(t,
			[]Event{WindowChanged{Kind: WindowAdded, Window: Window{ID: "@3"}}},
			c.apply(WindowAddNotification{Window: "@3"}))
		require.Len(t, c.Windows(), 3)
	})

	t.Run("window close drops the window and its pane", func(t *testing.T) {
		t.Parallel()
		c := seededController()
		events := c.apply(WindowCloseNotification{Window: "@2"})
		require.Equal(t, []Event{WindowChanged{
			Kind:   WindowClosed,
			Window: Window{ID: "@2", Name: "shell", ActivePane: "%2", Width: 120, Height: 40, Layout: singlePaneLayout("%2", 120, 40)},
		}}, events)

		_, ok := c.windowForPane("%2")
		require.False(t, ok)
	})

	t.Run("rename updates the window", func(t *testing.T) {
		t.Parallel()
		c := seededController()
		events := c.apply(WindowRenamedNotification{Window: "@2", Name: "logs"})
		require.Equal(t, []Event{WindowChanged{
			Kind:   WindowRenamed,
			Window: Window{ID: "@2", Name: "logs", ActivePane: "%2", Width: 120, Height: 40, Layout: singlePaneLayout("%2", 120, 40)},
		}}, events)
	})

	t.Run("pane change inside the layout is an active change", func(t *testing.T) {
		t.Parallel()
		c := seededController()
		split, err := ParseLayout("f91d,120x40,0,0{60x40,0,0,2,59x40,61,0,7}")
		require.NoError(t, err)
		require.Len(t, c.apply(LayoutChanged{Window: "@2", Layout: split}), 1)

		events := c.apply(WindowPaneChanged{Window: "@2", Pane: "%7"})
		require.Equal(t, []Event{WindowChanged{
			Kind:   WindowActiveChanged,
			Window: Window{ID: "@2", Name: "shell", ActivePane: "%7", Width: 120, Height: 40, Layout: split},
		}}, events)

		w, ok := c.windowForPane("%7")
		require.True(t, ok)
		require.Equal(t, "@2", w.ID)
	})

	t.Run("session window change moves the active flag", func(t *testing.T) {
		t.Parallel()
		c := seededController()
		events := c.apply(SessionWindowChanged{Session: "$1", Window: "@2"})
		require.Equal(t, []Event{WindowChanged{
			Kind:   WindowActiveChanged,
			Window: Window{ID: "@2", Name: "shell", Active: true, ActivePane: "%2", Width: 120, Height: 40, Layout: singlePaneLayout("%2", 120, 40)},
		}}, events)

		windows := c.Windows()
		require.False(t, windows[0].Active)
		require.True(t, windows[1].Active)
	})

	t.Run("pause and resume carry the window", func(t *testing.T) {
		t.Parallel()
		c := seededController()
		require.Equal(t,
			[]Event{LifecycleChanged{Kind: LifecyclePaused, WindowID: "@1"}},
			c.apply(PauseNotification{Pane: "%1"}))
		require.Equal(t,
			[]Event{LifecycleChanged{Kind: LifecycleResumed, WindowID: "@1"}},
			c.apply(ContinueNotification{Pane: "%1"}))
	})

	t.Run("layout change carries the window's new tree and size", func(t *testing.T) {
		t.Parallel()
		c := seededController()
		require.Equal(t, []Event{WindowChanged{
			Kind:   WindowLayoutChanged,
			Window: Window{ID: "@1", Name: "claude", Active: true, ActivePane: "%1", Width: 80, Height: 24, Layout: singlePaneLayout("%1", 80, 24)},
		}}, c.apply(LayoutChanged{Window: "@1", Layout: singlePaneLayout("%1", 80, 24)}))

		require.Nil(t, c.apply(LayoutChanged{Window: "@1", Layout: singlePaneLayout("%1", 80, 24)}),
			"a layout change that moves no boundary is not a change")
		require.Nil(t, c.apply(LayoutChanged{Window: "@1"}),
			"an unreadable layout leaves the tree alone; the reconcile it triggers carries it")

		split, err := ParseLayout("f91d,80x24,0,0{40x24,0,0,1,39x24,41,0,9}")
		require.NoError(t, err)
		events := c.apply(LayoutChanged{Window: "@1", Layout: split})
		require.Len(t, events, 1)
		changed, ok := events[0].(WindowChanged)
		require.True(t, ok)
		require.Equal(t, WindowLayoutChanged, changed.Kind)
		w, ok := c.windowForPane("%9")
		require.True(t, ok, "every pane in the layout resolves to its window")
		require.Equal(t, "@1", w.ID)
		require.Equal(t, []string{"%1", "%9"}, w.Panes())

		require.Equal(t, []Event{WindowChanged{
			Kind:   WindowLayoutChanged,
			Window: Window{ID: "@1", Name: "claude", Active: true, ActivePane: "%1", Width: 80, Height: 24, Layout: split, Zoomed: true},
		}}, c.apply(LayoutChanged{Window: "@1", Layout: split, Zoomed: true}), "a zoom is a layout change too")
	})

	t.Run("a pane that left the layout still routes", func(t *testing.T) {
		t.Parallel()
		c := seededController()
		split, err := ParseLayout("f91d,120x40,0,0{60x40,0,0,1,59x40,61,0,9}")
		require.NoError(t, err)
		require.Len(t, c.apply(LayoutChanged{Window: "@1", Layout: split}), 1)
		require.Len(t, c.apply(LayoutChanged{Window: "@1", Layout: singlePaneLayout("%1", 120, 40)}), 1)

		w, ok := c.windowForPane("%9")
		require.True(t, ok, "the index is additive: the pane's last bytes still have a window to land in")
		require.Equal(t, "@1", w.ID)
		require.NotContains(t, w.Panes(), "%9", "but it is not one of the window's panes any more")
	})

	t.Run("notifications for unknown windows are inert", func(t *testing.T) {
		t.Parallel()
		c := seededController()
		require.Nil(t, c.apply(WindowCloseNotification{Window: "@99"}))
		require.Nil(t, c.apply(WindowRenamedNotification{Window: "@99", Name: "x"}))
		require.Nil(t, c.apply(SessionWindowChanged{Session: "$1", Window: "@99"}))
		require.Nil(t, c.apply(LayoutChanged{Window: "@99", Layout: singlePaneLayout("%99", 80, 24)}))
		require.Len(t, c.Windows(), 2)
	})
}

// tmux announces a split or a closed pane as two notifications, and after the
// first the window's active pane is outside its layout. A consumer reads every
// event as a whole snapshot, so nothing is published until the second lands.
func TestControllerHoldsAWindowWhoseActivePaneIsOutsideItsLayout(t *testing.T) {
	t.Parallel()

	split, err := ParseLayout("f91d,120x40,0,0{60x40,0,0,1,59x40,61,0,9}")
	require.NoError(t, err)
	single := singlePaneLayout("%1", 120, 40)

	splitController := func() *controller {
		c := newController()
		c.set([]Window{{ID: "@1", Name: "claude", Active: true, ActivePane: "%9", Width: 120, Height: 40, Layout: split}})
		return c
	}

	t.Run("a closed pane: the layout lands first, the pane second", func(t *testing.T) {
		t.Parallel()
		c := splitController()
		require.Nil(t, c.apply(LayoutChanged{Window: "@1", Layout: single}), "the active pane is gone from the layout")
		require.Equal(t, []Event{WindowChanged{
			Kind:   WindowLayoutChanged,
			Window: Window{ID: "@1", Name: "claude", Active: true, ActivePane: "%1", Width: 120, Height: 40, Layout: single},
		}}, c.apply(WindowPaneChanged{Window: "@1", Pane: "%1"}))
	})

	t.Run("a split: the pane lands first, the layout second", func(t *testing.T) {
		t.Parallel()
		c := seededController()
		require.Nil(t, c.apply(WindowPaneChanged{Window: "@1", Pane: "%9"}), "the active pane is not in the layout yet")
		require.Equal(t, []Event{WindowChanged{
			Kind:   WindowLayoutChanged,
			Window: Window{ID: "@1", Name: "claude", Active: true, ActivePane: "%9", Width: 120, Height: 40, Layout: split},
		}}, c.apply(LayoutChanged{Window: "@1", Layout: split}))
	})

	t.Run("a rename while held rides the release", func(t *testing.T) {
		t.Parallel()
		c := splitController()
		require.Nil(t, c.apply(LayoutChanged{Window: "@1", Layout: single}))
		require.Nil(t, c.apply(WindowRenamedNotification{Window: "@1", Name: "logs"}))
		require.Equal(t, []Event{WindowChanged{
			Kind:   WindowLayoutChanged,
			Window: Window{ID: "@1", Name: "logs", Active: true, ActivePane: "%1", Width: 120, Height: 40, Layout: single},
		}}, c.apply(WindowPaneChanged{Window: "@1", Pane: "%1"}))
	})

	t.Run("a close drops the hold", func(t *testing.T) {
		t.Parallel()
		c := splitController()
		require.Nil(t, c.apply(LayoutChanged{Window: "@1", Layout: single}))
		events := c.apply(WindowCloseNotification{Window: "@1"})
		require.Len(t, events, 1)
		closed, ok := events[0].(WindowChanged)
		require.True(t, ok)
		require.Equal(t, WindowClosed, closed.Kind)
		require.Nil(t, c.apply(WindowPaneChanged{Window: "@1", Pane: "%1"}))
		require.Empty(t, c.Windows())
		require.Empty(t, c.held)
	})

	t.Run("a reconcile lifts the hold", func(t *testing.T) {
		t.Parallel()
		c := splitController()
		since := c.mark()
		require.Nil(t, c.apply(LayoutChanged{Window: "@1", Layout: single}))
		events := c.reconcile([]Window{
			{ID: "@1", Name: "claude", Active: true, ActivePane: "%1", Width: 120, Height: 40, Layout: single},
		}, since)
		require.Len(t, events, 1)
		changed, ok := events[0].(WindowChanged)
		require.True(t, ok)
		require.Equal(t, "%1", changed.Window.ActivePane)
		require.Empty(t, c.held)
	})

	t.Run("a placeholder is consistent with any pane", func(t *testing.T) {
		t.Parallel()
		c := seededController()
		c.apply(WindowAddNotification{Window: "@3"})
		require.Equal(t, []Event{WindowChanged{
			Kind:   WindowActiveChanged,
			Window: Window{ID: "@3", ActivePane: "%3"},
		}}, c.apply(WindowPaneChanged{Window: "@3", Pane: "%3"}))
	})
}

func TestControllerReconcileDiffs(t *testing.T) {
	t.Parallel()

	c := seededController()
	since := c.mark()
	events := c.reconcile([]Window{
		{ID: "@1", Name: "claude", Active: true, ActivePane: "%1", Width: 80, Height: 24, Layout: singlePaneLayout("%1", 80, 24)},
		{ID: "@3", Name: "logs", ActivePane: "%3", Width: 80, Height: 24, Layout: singlePaneLayout("%3", 80, 24)},
	}, since)

	require.Equal(t, []Event{
		WindowChanged{Kind: WindowLayoutChanged, Window: Window{ID: "@1", Name: "claude", Active: true, ActivePane: "%1", Width: 80, Height: 24, Layout: singlePaneLayout("%1", 80, 24)}},
		WindowChanged{Kind: WindowAdded, Window: Window{ID: "@3", Name: "logs", ActivePane: "%3", Width: 80, Height: 24, Layout: singlePaneLayout("%3", 80, 24)}},
		WindowChanged{Kind: WindowClosed, Window: Window{ID: "@2", Name: "shell", ActivePane: "%2", Width: 120, Height: 40, Layout: singlePaneLayout("%2", 120, 40)}},
	}, events)

	require.Equal(t, []string{"@1", "@3"}, windowIDs(c.Windows()))
}

// A reorder changes no window, so the snapshot's order is the only evidence it
// happened — and the only thing that carries it.
func TestControllerReconcileAdoptsTmuxOrder(t *testing.T) {
	t.Parallel()

	c := seededController()
	since := c.mark()
	events := c.reconcile([]Window{
		{ID: "@2", Name: "shell", ActivePane: "%2", Width: 120, Height: 40, Layout: singlePaneLayout("%2", 120, 40)},
		{ID: "@1", Name: "claude", Active: true, ActivePane: "%1", Width: 120, Height: 40, Layout: singlePaneLayout("%1", 120, 40)},
	}, since)

	require.Empty(t, events, "a pure reorder changes nothing about any window")
	require.Equal(t, []string{"@2", "@1"}, windowIDs(c.Windows()))
}

// A %window-add placeholder gains its name and pane from the reconcile that
// follows it.
func TestControllerReconcileFillsPlaceholder(t *testing.T) {
	t.Parallel()

	c := seededController()
	c.apply(WindowAddNotification{Window: "@3"})
	since := c.mark()

	events := c.reconcile([]Window{
		{ID: "@1", Name: "claude", Active: true, ActivePane: "%1", Width: 120, Height: 40, Layout: singlePaneLayout("%1", 120, 40)},
		{ID: "@2", Name: "shell", ActivePane: "%2", Width: 120, Height: 40, Layout: singlePaneLayout("%2", 120, 40)},
		{ID: "@3", Name: "logs", ActivePane: "%3", Width: 120, Height: 40, Layout: singlePaneLayout("%3", 120, 40)},
	}, since)

	require.Equal(t, []Event{WindowChanged{
		Kind:   WindowRenamed,
		Window: Window{ID: "@3", Name: "logs", ActivePane: "%3", Width: 120, Height: 40, Layout: singlePaneLayout("%3", 120, 40)},
	}}, events)

	w, ok := c.windowForPane("%3")
	require.True(t, ok)
	require.Equal(t, "logs", w.Name)
}

func TestParseWindowLine(t *testing.T) {
	t.Parallel()

	w, ok := parseWindowLine("@275 1 %512 213 55 0 b25f,213x55,0,0,512 claude session")
	require.True(t, ok)
	require.Equal(t, Window{ID: "@275", Name: "claude session", Active: true, ActivePane: "%512", Width: 213, Height: 55, Layout: singlePaneLayout("%512", 213, 55)}, w)

	w, ok = parseWindowLine("@276 0 %513 80 24 0 b25f,80x24,0,0,513")
	require.True(t, ok)
	require.Equal(t, Window{ID: "@276", ActivePane: "%513", Width: 80, Height: 24, Layout: singlePaneLayout("%513", 80, 24)}, w)

	w, ok = parseWindowLine("@278 0 %515 80 24 0 garbage my logs")
	require.True(t, ok, "an unreadable layout does not fail the row")
	require.Equal(t, Window{ID: "@278", Name: "my logs", ActivePane: "%515", Width: 80, Height: 24}, w)

	_, ok = parseWindowLine("nonsense")
	require.False(t, ok)
	_, ok = parseWindowLine("@277 0 %514 shell")
	require.False(t, ok, "a row without dimensions is not this format")
}

// reconcile's snapshot is fetched before it is merged, so a window added in
// between is in the set and not in the snapshot. Treating that absence as a
// close drops a window tmux is still holding (#278). Only the removal pass is
// unsafe against a stale snapshot; the add and update pass is idempotent.
func TestControllerReconcileKeepsWindowsAddedAfterTheSnapshot(t *testing.T) {
	t.Parallel()

	c := seededController()
	since := c.mark()
	c.apply(WindowAddNotification{Window: "@3"})

	events := c.reconcile([]Window{
		{ID: "@1", Name: "claude", Active: true, ActivePane: "%1", Width: 120, Height: 40, Layout: singlePaneLayout("%1", 120, 40)},
		{ID: "@2", Name: "shell", ActivePane: "%2", Width: 120, Height: 40, Layout: singlePaneLayout("%2", 120, 40)},
	}, since)

	require.Empty(t, events, "a snapshot that predates @3 reports nothing about it")
	require.Equal(t, []string{"@1", "@2", "@3"}, windowIDs(c.Windows()),
		"the window survives the merge and keeps its place in the order")

	// It is a placeholder until a snapshot that has seen it fills it in, and
	// that snapshot is also what may legitimately close it.
	since = c.mark()
	c.reconcile([]Window{
		{ID: "@1", Name: "claude", Active: true, ActivePane: "%1", Width: 120, Height: 40, Layout: singlePaneLayout("%1", 120, 40)},
	}, since)
	require.Equal(t, []string{"@1"}, windowIDs(c.Windows()),
		"a snapshot taken after the add is authoritative and does close it")
}

func windowIDs(windows []Window) []string {
	ids := make([]string, 0, len(windows))
	for _, w := range windows {
		ids = append(ids, w.ID)
	}
	return ids
}
