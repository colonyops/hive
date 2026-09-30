//go:build !server

package tmuxcc

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func lifecycleIs(kind LifecycleKind) func(Event) bool {
	return func(ev Event) bool {
		lc, ok := ev.(LifecycleChanged)
		return ok && lc.Kind == kind
	}
}

func outputContains(windowID, want string) func(Event) bool {
	return func(ev Event) bool {
		out, ok := ev.(Output)
		return ok && out.WindowID == windowID && strings.Contains(string(out.Data), want)
	}
}

func lastLifecycle(t *testing.T, events []Event) LifecycleChanged {
	t.Helper()
	for _, ev := range slices.Backward(events) {
		if lc, ok := ev.(LifecycleChanged); ok {
			return lc
		}
	}
	t.Fatalf("no lifecycle event in %#v", events)
	return LifecycleChanged{}
}

func TestAttachRunsTheHandshakeSequence(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows("@1 1 %1 120 40 0 b25f,120x40,0,0,1 claude", "@2 0 %2 120 40 0 b25f,120x40,0,0,2 shell")
	f.setCapture("%1", "claude> ready")
	f.setCapture("%2", "$ ")

	client := attachFake(t, f, Options{Cols: 120, Rows: 40})

	require.Equal(t, []Window{
		{ID: "@1", Name: "claude", Active: true, ActivePane: "%1", Width: 120, Height: 40, Layout: singlePaneLayout("%1", 120, 40)},
		{ID: "@2", Name: "shell", ActivePane: "%2", Width: 120, Height: 40, Layout: singlePaneLayout("%2", 120, 40)},
	}, client.Windows())

	commands := f.sentCommands()
	require.Equal(t, "refresh-client -C 120,40", commands[0])
	require.Equal(t, `list-windows -F "`+listWindowsFormat+`"`, commands[1])
	// Only the active window is snapshotted before Attach answers. A first
	// paint is dominated by capture-pane, so painting every window here made
	// attach latency scale with the window count for panes the renderer cannot
	// show yet. The bound is spelled out rather than built from historyLines:
	// it is a decision about startup cost, so changing it should fail a test.
	require.Equal(t, []string{
		`display-message -p -t %1 "` + paneStateFormat + `"`,
		"capture-pane -pe -J -S -2000 -E -1 -t %1",
		"capture-pane -pe -S 0 -t %1",
	}, commands[2:5], "the active window is snapshotted state-first, then history, then screen")

	// The rest follow on the client's own lifetime, in the same shape. The wait
	// is on the last command of the sequence, not the first: anything earlier
	// races the two that follow it.
	f.awaitCommands(t, "capture-pane -pe -S 0 -t %2", 1)
	require.Equal(t, []string{
		`display-message -p -t %2 "` + paneStateFormat + `"`,
		"capture-pane -pe -J -S -2000 -E -1 -t %2",
		"capture-pane -pe -S 0 -t %2",
	}, f.sentCommands()[5:8], "a deferred window is snapshotted the same way")

	for _, cmd := range f.sentCommands() {
		require.NotContains(t, cmd, "pause-after", "v1 never enables pause mode")
	}
}

// The deferred paint must not reorder a pane's stream: whatever the background
// window produced while its snapshot was in flight has to land behind that
// snapshot, exactly as it does for the window painted synchronously.
func TestDeferredFirstPaintPrecedesTheOutputItRaced(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows("@1 1 %1 120 40 0 b25f,120x40,0,0,1 claude", "@2 0 %2 120 40 0 b25f,120x40,0,0,2 shell")
	f.setCapture("%1", "claude> ready")
	f.setHistory("%2", "old scrollback")
	f.setCapture("%2", "$ ")
	// Emitted while the deferred snapshot for %2 is being requested.
	f.setOnCommand(func(cmd string) {
		if strings.HasPrefix(cmd, `display-message -p -t %2`) {
			f.emit(`%output %2 live-after-mark`)
		}
	})

	client := attachFake(t, f, Options{Cols: 120, Rows: 40})
	events, unsubscribe := client.Subscribe()
	t.Cleanup(unsubscribe)

	var window2 []byte
	require.Eventually(t, func() bool {
		for {
			select {
			case ev := <-events:
				if out, ok := ev.(Output); ok && out.WindowID == "@2" {
					window2 = append(window2, out.Data...)
				}
			default:
				return bytes.Contains(window2, []byte("live-after-mark"))
			}
		}
	}, 2*time.Second, time.Millisecond)

	require.Less(t,
		bytes.Index(window2, []byte("old scrollback")),
		bytes.Index(window2, []byte("live-after-mark")),
		"the snapshot must reach the stream before the output that raced it")
}

// A caller with nothing measured must not vote a placeholder: tmux would obey
// it and resize the session, and every other client attached to it, to a size
// nobody asked for. Setting no size keeps this client out of the negotiation
// until Resize opts it in.
func TestUnsizedAttachSetsNoClientSizeUntilResized(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows("@1 1 %1 120 40 0 b25f,120x40,0,0,1 claude")

	ctx := t.Context()
	client, err := Attach(ctx, ctx, Options{Slug: f.slug, newProcess: f.factory()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close(context.WithoutCancel(ctx)) })

	require.Zero(t, f.countCommands("refresh-client"))
	require.Equal(t, 120, client.Windows()[0].Width, "the session keeps the size its other clients gave it")

	require.NoError(t, client.Resize(ctx, 213, 55))
	require.Equal(t, "refresh-client -C 213,55", f.sentCommands()[len(f.sentCommands())-1])
}

// First paint reaches a subscriber that connects after Attach, because the
// broker buffered it.
func TestAttachFirstPaintsEachWindow(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows("@1 1 %1 120 3 0 b25f,120x3,0,0,1 claude", "@2 0 %2 120 3 0 b25f,120x3,0,0,2 shell")
	f.setCapture("%1", "claude> ready", "second row", "")
	f.setCapture("%2", "$ ", "", "")

	client := attachFake(t, f, Options{})
	// The non-active window is painted after Attach answers, from a goroutine
	// that can beat the attach lifecycle event onto the stream — so the stop
	// condition is both of them, in whichever order they land.
	var painted, attached bool
	events, unsubscribe := subscribeAndCollect(t, client, func(ev Event) bool {
		painted = painted || outputContains("@2", "$ ")(ev)
		attached = attached || lifecycleIs(LifecycleAttached)(ev)
		return painted && attached
	})
	defer unsubscribe()

	require.Equal(t, "claude> ready\r\nsecond row\r\n", outputData(events, "@1"))
	require.Equal(t, "$ \r\n\r\n", outputData(events, "@2"))
	require.Equal(t, LifecycleAttached, lastLifecycle(t, events).Kind)
}

// The scrollback tmux holds for a pane is what makes attaching to a session
// that has been running for hours worth anything: without it the tab opens on
// whatever happens to be on screen and everything before it is gone.
func TestFirstPaintReplaysHistoryAheadOfTheScreen(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows("@1 1 %1 120 2 0 b25f,120x2,0,0,1 claude")
	f.setHistory("%1", "$ echo hi", "hi")
	f.setCapture("%1", "$ ", "")
	f.setCursor("%1", 0, 2)

	client := attachFake(t, f, Options{})
	events, unsubscribe := subscribeAndCollect(t, client, lifecycleIs(LifecycleAttached))
	defer unsubscribe()

	require.Equal(t, "$ echo hi\r\nhi\r\n$ \r\n\x1b[1;3H", outputData(events, "@1"),
		"history scrolls out of the viewport, the screen fills it, the cursor lands last")
}

func TestFirstPaintRestoresMouseTrackingAheadOfTheCursor(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows("@1 1 %1 120 2 0 b25f,120x2,0,0,1 nvim")
	f.setHistory("%1", "$ nvim README.md")
	f.setCapture("%1", "README.md", "~")
	f.setPaneState("%1", 0, 4, 1002, true)

	client := attachFake(t, f, Options{})
	events, unsubscribe := subscribeAndCollect(t, client, lifecycleIs(LifecycleAttached))
	defer unsubscribe()

	require.Equal(t, "$ nvim README.md\r\nREADME.md\r\n~\x1b[?1002;1006h\x1b[1;5H", outputData(events, "@1"))
}

// An emulator pins its viewport to the last rows written, so the screen has to
// occupy the whole grid: paint it short and the pane's row 0 sits below the
// viewport's, and the cursor-addressed redraws an alternate-screen app makes
// land that many rows off for the rest of the attach.
func TestFirstPaintFillsTheWindowHeight(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows("@1 1 %1 120 5 0 b25f,120x5,0,0,1 claude")
	f.setCapture("%1", "$ echo hi", "hi", "$ ")
	f.setCursor("%1", 2, 2)

	client := attachFake(t, f, Options{})
	events, unsubscribe := subscribeAndCollect(t, client, lifecycleIs(LifecycleAttached))
	defer unsubscribe()

	painted := outputData(events, "@1")
	require.Equal(t, 4, strings.Count(painted, "\r\n"), "five rows are five rows, blank or not")
	require.True(t, strings.HasSuffix(painted, "\x1b[3;3H"), "cursor restored to its own row: %q", painted)
}

// tmux answers with no cursor only when the pane went away mid-snapshot. Moving
// the cursor on a guess would be worse than leaving it where the screen put it.
func TestFirstPaintWithoutACursorLeavesItAlone(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows("@1 1 %1 120 2 0 b25f,120x2,0,0,1 claude")
	f.setCapture("%1", "$ ", "")

	client := attachFake(t, f, Options{})
	events, unsubscribe := subscribeAndCollect(t, client, lifecycleIs(LifecycleAttached))
	defer unsubscribe()

	require.Equal(t, "$ \r\n", outputData(events, "@1"))
}

// The pane keeps writing throughout the attach: the snapshot must land first
// and the live stream must stay contiguous behind it.
func TestAttachReplaysLiveOutputBehindSnapshot(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows("@1 1 %1 120 1 0 b25f,120x1,0,0,1 claude")
	f.setCapture("%1", "SNAPSHOT")

	stop, writerDone := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(writerDone)
		for {
			select {
			case <-stop:
				return
			default:
			}
			f.emit(`%output %1 A`)
		}
	}()

	client := attachFake(t, f, Options{})
	close(stop)
	<-writerDone
	f.emit(`%output %1 SENTINEL`)

	events, unsubscribe := subscribeAndCollect(t, client, outputContains("@1", "SENTINEL"))
	defer unsubscribe()

	data := outputData(events, "@1")
	require.True(t, strings.HasPrefix(data, "SNAPSHOT"), "snapshot paints first: %q", data)
	require.True(t, strings.HasSuffix(data, "SENTINEL"))

	middle := strings.TrimSuffix(strings.TrimPrefix(data, "SNAPSHOT"), "SENTINEL")
	require.Equal(t, strings.Repeat("A", len(middle)), middle, "the live stream stays contiguous")
}

// A transport-only drop leaves this client attached and its emulator gone. The
// stream that replaces it starts from nothing, so a repaint has to put every
// window back on screen — and drop what the dead transport never delivered,
// which the snapshot already contains.
func TestRepaintSnapshotsEveryWindowForTheNextSubscriber(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows("@1 1 %1 120 1 0 b25f,120x1,0,0,1 claude", "@2 0 %2 120 1 0 b25f,120x1,0,0,2 shell")
	f.setCapture("%1", "FIRST")
	f.setCapture("%2", "$ ")

	client := attachFake(t, f, Options{})
	_, dropped := subscribeAndCollect(t, client, lifecycleIs(LifecycleAttached))
	dropped()

	f.emit("%output %1 STALE")
	require.Eventually(t, func() bool { return backlogHas(client.events, "STALE") }, 2*time.Second, time.Millisecond,
		"the output the dropped transport never took is what the repaint supersedes")
	f.setCapture("%1", "REPAINTED")

	require.NoError(t, client.Repaint(t.Context()))

	events, unsubscribe := subscribeAndCollect(t, client, outputContains("@2", "$ "))
	defer unsubscribe()
	require.Equal(t, "REPAINTED", outputData(events, "@1"), "the pane is captured again, and the undelivered bytes go with the old stream")
	require.Equal(t, "$ ", outputData(events, "@2"), "every window is painted, not just the active one")
}

// %window-add carries no name or pane, so it schedules a list-windows on the
// command worker — never on the reader goroutine.
func TestWindowAddTriggersReconcile(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows("@1 1 %1 120 40 0 b25f,120x40,0,0,1 claude")
	client := attachFake(t, f, Options{})

	f.setWindows("@1 1 %1 120 40 0 b25f,120x40,0,0,1 claude", "@2 0 %2 120 40 0 b25f,120x40,0,0,2 shell")
	f.emit("%window-add @2")

	events, unsubscribe := subscribeAndCollect(t, client, func(ev Event) bool {
		wc, ok := ev.(WindowChanged)
		return ok && wc.Window.ID == "@2" && wc.Window.Name == "shell"
	})
	defer unsubscribe()
	require.NotEmpty(t, events)

	f.awaitCommands(t, "list-windows", 2)
	require.Equal(t, []Window{
		{ID: "@1", Name: "claude", Active: true, ActivePane: "%1", Width: 120, Height: 40, Layout: singlePaneLayout("%1", 120, 40)},
		{ID: "@2", Name: "shell", ActivePane: "%2", Width: 120, Height: 40, Layout: singlePaneLayout("%2", 120, 40)},
	}, client.Windows())
}

// A reconcile's snapshot is a round trip old by the time it is merged, so a
// window that appeared while it was in flight is missing from it. Removing what
// the snapshot does not have drops a window tmux is still holding and publishes
// a close for it, and every rename, close or select on that id fails until a
// later reconcile puts it back (#278).
func TestReconcileKeepsAWindowItsSnapshotIsTooOldToHold(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows("@1 1 %1 120 40 0 b25f,120x40,0,0,1 claude")
	client := attachFake(t, f, Options{})

	// The hook runs before the reply is composed, so @2 is announced while the
	// snapshot that predates it is still in flight, and the reader applies the
	// add before it can deliver that reply. tmux catches up from the next
	// snapshot on, which is what a real server would answer. The renames ride
	// along as markers for which merge has run.
	calls := 0
	f.setOnCommand(func(cmd string) {
		if !strings.HasPrefix(cmd, "list-windows") {
			return
		}
		calls++
		if calls == 1 {
			f.setWindows("@1 1 %1 120 40 0 b25f,120x40,0,0,1 renamed")
			f.emit("%window-add @2")
			return
		}
		f.setWindows("@1 1 %1 120 40 0 b25f,120x40,0,0,1 settled", "@2 0 %2 120 40 0 b25f,120x40,0,0,2 shell")
	})
	f.emit("%layout-change @1 b25d,120x40,0,0,1")

	events, unsubscribe := subscribeAndCollect(t, client, func(ev Event) bool {
		wc, ok := ev.(WindowChanged)
		return ok && wc.Kind == WindowRenamed && wc.Window.ID == "@1" && wc.Window.Name == "settled"
	})
	defer unsubscribe()

	for _, ev := range events {
		wc, ok := ev.(WindowChanged)
		require.False(t, ok && wc.Kind == WindowClosed && wc.Window.ID == "@2",
			"a window the snapshot is too old to hold is not reported closed")
	}
	require.Equal(t, []string{"@1", "@2"}, windowIDs(client.Windows()))
}

// %window-add carries no pane, so %output for the new window is unroutable
// until the reconcile lands and would otherwise be lost — along with the new
// tab's prompt. The snapshot the reconcile takes is what puts both on screen.
func TestReconcileFirstPaintsANewWindow(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows("@1 1 %1 120 40 0 b25f,120x40,0,0,1 claude")
	client := attachFake(t, f, Options{})

	f.setCapture("%2", "$ echo hi", "hi")
	f.setWindows("@1 1 %1 120 40 0 b25f,120x40,0,0,1 claude", "@2 0 %2 120 2 0 b25f,120x2,0,0,2 shell")
	f.emit(`%output %2 unroutable\015\012`)
	f.emit("%window-add @2")

	f.awaitCommands(t, "capture-pane -pe -S 0 -t %2", 1)
	f.emit(`%output %2 SENTINEL`)

	events, unsubscribe := subscribeAndCollect(t, client, outputContains("@2", "SENTINEL"))
	defer unsubscribe()

	require.Equal(t, "$ echo hi\r\nhiSENTINEL", outputData(events, "@2"),
		"the snapshot paints first and live output replays behind it")

	var added bool
	for _, ev := range events {
		if wc, ok := ev.(WindowChanged); ok && wc.Window.ID == "@2" {
			added = true
			continue
		}
		if out, ok := ev.(Output); ok && out.WindowID == "@2" {
			require.True(t, added, "the tab exists before its first byte")
		}
	}
}

// A window can die while the snapshot that would give it its first paint is
// still in flight. The tab's closure has to reach the subscriber regardless, and
// the bytes held for a pane with no tab left have nowhere to go.
func TestWindowClosedDuringItsFirstPaint(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows("@1 1 %1 120 40 0 b25f,120x40,0,0,1 claude")
	client := attachFake(t, f, Options{})

	f.setCapture("%2", "$ prompt")
	f.setWindows("@1 1 %1 120 40 0 b25f,120x40,0,0,1 claude", "@2 0 %2 120 40 0 b25f,120x40,0,0,2 shell")
	f.setOnCommand(func(cmd string) {
		if strings.HasPrefix(cmd, "capture-pane -pe -S 0 -t %2") {
			f.emit(`%output %2 HELD`)
			f.emit("%window-close @2")
		}
	})
	f.emit("%window-add @2")

	events, unsubscribe := subscribeAndCollect(t, client, func(ev Event) bool {
		wc, ok := ev.(WindowChanged)
		return ok && wc.Kind == WindowClosed && wc.Window.ID == "@2"
	})
	defer unsubscribe()

	for _, ev := range events {
		out, ok := ev.(Output)
		require.False(t, ok && out.WindowID == "@2", "a closed window renders nothing: %#v", ev)
	}
	require.NotContains(t, client.Windows(), Window{ID: "@2", Name: "shell", ActivePane: "%2", Width: 120, Height: 40, Layout: singlePaneLayout("%2", 120, 40)})

	client.paint.mu.Lock()
	defer client.paint.mu.Unlock()
	require.NotContains(t, client.paint.buf, "%2", "the gate still holds bytes for a dead pane")
	require.NotContains(t, client.paint.held, "%2", "the gate is still holding a dead pane")
	require.NotContains(t, client.paint.pending, "%2")
	require.NotContains(t, client.paint.live, "%2")
}

// The server blocks mid-reply once the client's reader is gone. Teardown has to
// be able to kill it anyway — the harness used to hold its lock across that
// write, so Kill deadlocked and this test could not be written.
func TestTeardownWhileTheServerIsMidReply(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows("@1 1 %1 120 40 0 b25f,120x40,0,0,1 claude")
	client := attachFake(t, f, Options{})
	ch, unsubscribe := client.Subscribe()
	defer unsubscribe()

	rows := make([]string, 0, 64)
	for i := range 64 {
		rows = append(rows, fmt.Sprintf("@%d 0 %%%d 120 40 shell", 100+i, 100+i))
	}
	f.setWindows(rows...)
	f.setOnCommand(func(cmd string) {
		if strings.HasPrefix(cmd, "list-windows") {
			f.emit("%end 1 1 1")
		}
	})
	f.emit("%window-add @2")

	events := collect(t, ch, lifecycleIs(LifecycleExited))
	require.Equal(t, "protocol error", lastLifecycle(t, events).Message)
}

func TestReconcileIsCoalesced(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows("@1 1 %1 120 40 0 b25f,120x40,0,0,1 claude")
	client := attachFake(t, f, Options{})

	f.setWindows("@1 1 %1 120 40 0 b25f,120x40,0,0,1 claude", "@2 0 %2 120 40 0 b25f,120x40,0,0,2 shell")
	for range 5 {
		f.emit("%window-add @2")
		f.emit("%layout-change @2 b25d,80x24,0,0,1")
	}

	_, unsubscribe := subscribeAndCollect(t, client, func(ev Event) bool {
		wc, ok := ev.(WindowChanged)
		return ok && wc.Window.ID == "@2" && wc.Window.Name == "shell"
	})
	defer unsubscribe()

	require.Less(t, f.countCommands("list-windows"), 11, "pending reconciles absorb new triggers")
}

func TestSessionWindowChangedEmitsActiveChanged(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows("@1 1 %1 120 40 0 b25f,120x40,0,0,1 claude", "@2 0 %2 120 40 0 b25f,120x40,0,0,2 shell")
	client := attachFake(t, f, Options{})

	f.emit("%session-window-changed $1 @2")

	events, unsubscribe := subscribeAndCollect(t, client, func(ev Event) bool {
		wc, ok := ev.(WindowChanged)
		return ok && wc.Kind == WindowActiveChanged
	})
	defer unsubscribe()

	changed, ok := events[len(events)-1].(WindowChanged)
	require.True(t, ok)
	require.Equal(t, "@2", changed.Window.ID)
	require.True(t, changed.Window.Active)
}

// tmux sizes a window to the smallest attached client, so a second client
// attaching elsewhere resizes it under us. %layout-change is how that reaches
// the renderer in time to draw the repaint that follows at the right width.
func TestLayoutChangeCarriesTheWindowSize(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows("@1 1 %1 120 40 0 b25f,120x40,0,0,1 claude")
	client := attachFake(t, f, Options{})

	f.emit("%layout-change @1 b25d,80x24,0,0,1 b25d,80x24,0,0,1 *")

	events, unsubscribe := subscribeAndCollect(t, client, func(ev Event) bool {
		wc, ok := ev.(WindowChanged)
		return ok && wc.Kind == WindowLayoutChanged
	})
	defer unsubscribe()

	changed, ok := events[len(events)-1].(WindowChanged)
	require.True(t, ok)
	require.Equal(t, "@1", changed.Window.ID)
	require.Equal(t, 80, changed.Window.Width)
	require.Equal(t, 24, changed.Window.Height)
}

func TestExtendedOutputIsRoutedLikeOutput(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows("@1 1 %1 120 40 0 b25f,120x40,0,0,1 claude")
	client := attachFake(t, f, Options{})

	f.emit(`%extended-output %1 4212 : caught\040up\015\012`)

	events, unsubscribe := subscribeAndCollect(t, client, outputContains("@1", "caught"))
	defer unsubscribe()
	require.Contains(t, outputData(events, "@1"), "caught up\r\n")
}

func TestOversizedOutputReachesTheSubscriber(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows("@1 1 %1 120 40 0 b25f,120x40,0,0,1 claude")
	client := attachFake(t, f, Options{})

	payload := strings.Repeat("z", 70_000)
	f.emit("%output %1 " + payload)

	events, unsubscribe := subscribeAndCollect(t, client, outputContains("@1", "zzz"))
	defer unsubscribe()
	require.Contains(t, outputData(events, "@1"), payload)
}

func TestOutputFromEveryPaneInTheLayoutIsForwarded(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows("@1 1 %2 120 40 0 f91d,120x40,0,0{60x40,0,0,1,59x40,61,0,2} claude")
	client := attachFake(t, f, Options{})

	f.emit(`%output %1 left\015\012`)
	f.emit(`%output %2 right\015\012`)
	f.emit(`%output %9 stray\015\012`)
	f.emit(`%output %1 done\015\012`)

	events, unsubscribe := subscribeAndCollect(t, client, outputContains("@1", "done"))
	defer unsubscribe()

	// The first paints of both panes precede these on the stream; a blank
	// screen trims to nothing and is skipped.
	var panes []string
	for _, ev := range events {
		if out, ok := ev.(Output); ok && strings.TrimSpace(string(out.Data)) != "" {
			panes = append(panes, out.PaneID+":"+strings.TrimSpace(string(out.Data)))
		}
	}
	require.Equal(t, []string{"%1:left", "%2:right", "%1:done"}, panes)
}

func TestSplitPaintsTheNewPaneAtItsOwnHeight(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows("@1 1 %1 120 40 0 b25f,120x40,0,0,1 claude")
	f.setCapture("%1", "claude> ready")
	client := attachFake(t, f, Options{Cols: 120, Rows: 40})
	events, unsubscribe := client.Subscribe()
	t.Cleanup(unsubscribe)

	f.setWindows("@1 1 %2 120 40 0 95e4,120x40,0,0[120x20,0,0,1,120x19,0,21,2] claude")
	f.setCapture("%2", "$ ")
	f.emit("%window-pane-changed @1 %2")
	f.emit("%layout-change @1 95e4,120x40,0,0[120x20,0,0,1,120x19,0,21,2] 95e4,120x40,0,0[120x20,0,0,1,120x19,0,21,2] *")

	f.awaitCommands(t, "capture-pane -pe -S 0 -t %2", 1)
	require.Equal(t, []string{
		`display-message -p -t %2 "` + paneStateFormat + `"`,
		"capture-pane -pe -J -S -2000 -E -1 -t %2",
		"capture-pane -pe -S 0 -t %2",
	}, f.commandsMatching("-t %2"), "the new pane is snapshotted once, state-first")

	var painted []byte
	require.Eventually(t, func() bool {
		for {
			select {
			case ev := <-events:
				if out, ok := ev.(Output); ok && out.PaneID == "%2" {
					painted = append(painted, out.Data...)
				}
			default:
				return bytes.Contains(painted, []byte("$ "))
			}
		}
	}, 2*time.Second, time.Millisecond)
	require.Equal(t, 19, strings.Count(string(painted), "\r\n")+1, "the screen is written at the pane's 19 rows, not the window's 40")
}

// tmux can send a split notification and the new pane's first output in either
// order. Output waits for the snapshot so the prompt is not drawn twice.
func TestSplitOutputWaitsForTheNewPanesSnapshot(t *testing.T) {
	t.Parallel()

	const layoutChange = "%layout-change @1 95e4,120x40,0,0[120x20,0,0,1,120x19,0,21,2] 95e4,120x40,0,0[120x20,0,0,1,120x19,0,21,2] *"
	cases := map[string][]string{
		"layout first":      {layoutChange, `%output %2 EARLY`},
		"pane change first": {"%window-pane-changed @1 %2", `%output %2 EARLY`, layoutChange},
	}
	for name, lines := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := newFakeTmux(t, "hive-demo")
			f.setWindows("@1 1 %1 120 40 0 b25f,120x40,0,0,1 claude")
			f.setCapture("%1", "claude> ready")
			client := attachFake(t, f, Options{Cols: 120, Rows: 40})
			events, unsubscribe := client.Subscribe()
			t.Cleanup(unsubscribe)

			f.setWindows("@1 1 %2 120 40 0 95e4,120x40,0,0[120x20,0,0,1,120x19,0,21,2] claude")
			f.setCapture("%2", "SNAPSHOT")
			f.write(lines...)
			f.awaitCommands(t, "capture-pane -pe -S 0 -t %2", 1)
			f.emit(`%output %2 LATE`)

			var chunks []string
			collect(t, events, func(ev Event) bool {
				out, ok := ev.(Output)
				if !ok || out.PaneID != "%2" {
					return false
				}
				chunks = append(chunks, string(out.Data))
				return strings.Contains(string(out.Data), "LATE")
			})

			require.True(t, strings.HasPrefix(chunks[0], "SNAPSHOT"), "the snapshot is the pane's first chunk: %q", chunks)
			streamed := strings.Join(chunks, "")
			require.NotContains(t, streamed, "EARLY", "bytes the snapshot already holds are not replayed")
			require.True(t, strings.HasSuffix(streamed, "LATE"), "bytes after the snapshot follow it: %q", chunks)
		})
	}
}

// Paint the visible zoomed pane before deferred captures of hidden panes.
func TestZoomedPanePaintsAtTheWindowHeight(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows("@1 1 %2 120 40 1 95e4,120x40,0,0[120x20,0,0,1,120x19,0,21,2] claude")
	f.setCapture("%1", "hidden")
	f.setCapture("%2", "zoomed")
	client := attachFake(t, f, Options{Cols: 120, Rows: 40})

	require.True(t, client.Windows()[0].Zoomed)
	commands := f.sentCommands()
	require.Equal(t, `display-message -p -t %2 "`+paneStateFormat+`"`, commands[2], "the zoomed pane paints before the attach answers")
	f.awaitCommands(t, "capture-pane -pe -S 0 -t %1", 1)

	events, unsubscribe := subscribeAndCollect(t, client, func(ev Event) bool {
		out, ok := ev.(Output)
		return ok && out.PaneID == "%1"
	})
	defer unsubscribe()
	rows := func(pane string) int {
		var data []byte
		for _, ev := range events {
			if out, ok := ev.(Output); ok && out.PaneID == pane {
				data = append(data, out.Data...)
			}
		}
		return strings.Count(string(data), "\r\n") + 1
	}
	require.Equal(t, 40, rows("%2"), "zoomed: the window's height")
	require.Equal(t, 20, rows("%1"), "hidden: its own height")
}

func TestPaneCommands(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows("@1 1 %1 120 40 0 f91d,120x40,0,0{60x40,0,0,1,59x40,61,0,2} claude")
	client := attachFake(t, f, Options{})
	ctx := t.Context()

	f.setSplitPane("%3")
	pane, err := client.SplitPane(ctx, "%1", SplitHorizontal)
	require.NoError(t, err)
	require.Equal(t, "%3", pane)
	require.Contains(t, f.sentCommands(), `split-window -h -t %1 -c "`+currentPathFormat+`" -P -F "#{pane_id}"`)
	_, err = client.SplitPane(ctx, "%2", SplitVertical)
	require.NoError(t, err)
	require.Contains(t, f.sentCommands(), `split-window -v -t %2 -c "`+currentPathFormat+`" -P -F "#{pane_id}"`)
	_, err = client.SplitPane(ctx, "%1", SplitDirection("sideways"))
	require.ErrorIs(t, err, ErrInvalidDirection)

	require.NoError(t, client.SelectPane(ctx, "%2", PaneSelf))
	require.Contains(t, f.sentCommands(), "select-pane -t %2")
	require.NoError(t, client.SelectPane(ctx, "%2", PaneLeft))
	require.Contains(t, f.sentCommands(), "select-pane -L -t %2")
	require.NoError(t, client.SelectPane(ctx, "%2", PaneDown))
	require.Contains(t, f.sentCommands(), "select-pane -D -t %2")
	require.ErrorIs(t, client.SelectPane(ctx, "%2", PaneDirection("back")), ErrInvalidDirection)

	require.NoError(t, client.KillPane(ctx, "%2"))
	require.Contains(t, f.sentCommands(), "kill-pane -t %2")

	require.NoError(t, client.ResizePane(ctx, "%1", 30, 0))
	require.Contains(t, f.sentCommands(), "resize-pane -t %1 -x 30")
	require.NoError(t, client.ResizePane(ctx, "%1", 0, 12))
	require.Contains(t, f.sentCommands(), "resize-pane -t %1 -y 12")
	require.NoError(t, client.ResizePane(ctx, "%1", 30, 12))
	require.Contains(t, f.sentCommands(), "resize-pane -t %1 -x 30 -y 12")
	require.ErrorIs(t, client.ResizePane(ctx, "%1", 0, 0), ErrInvalidSize)
	require.ErrorIs(t, client.ResizePane(ctx, "%1", maxDimension+1, 0), ErrInvalidSize)

	require.NoError(t, client.ZoomPane(ctx, "%2"))
	require.Contains(t, f.sentCommands(), "resize-pane -Z -t %2")

	// A pane id is a command argument, so anything but %<digits> a tracked
	// window owns is refused before it reaches tmux.
	for _, bad := range []string{"@1", "%99", "%1; kill-server", ""} {
		require.ErrorIs(t, client.SelectPane(ctx, bad, PaneSelf), ErrUnknownPane, bad)
		_, err := client.SplitPane(ctx, bad, SplitHorizontal)
		require.ErrorIs(t, err, ErrUnknownPane, bad)
		require.ErrorIs(t, client.KillPane(ctx, bad), ErrUnknownPane, bad)
		require.ErrorIs(t, client.ResizePane(ctx, bad, 10, 0), ErrUnknownPane, bad)
		require.ErrorIs(t, client.ZoomPane(ctx, bad), ErrUnknownPane, bad)
	}
	for _, cmd := range f.sentCommands() {
		require.NotContains(t, cmd, "kill-server")
	}
}

// A pane that left the layout still resolves to its window, so its last bytes
// land in a tab. A verb on it must not: tmux would answer "can't find pane",
// which reaches the caller as an internal error rather than the not-found the
// routes promise.
func TestVerbsOnADepartedPaneAreRefused(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows("@1 1 %2 120 40 0 f91d,120x40,0,0{60x40,0,0,1,59x40,61,0,2} claude")
	client := attachFake(t, f, Options{})
	ctx := t.Context()

	f.setWindows("@1 1 %1 120 40 0 b25f,120x40,0,0,1 claude")
	f.emit("%layout-change @1 b25f,120x40,0,0,1 b25f,120x40,0,0,1 *")
	f.emit("%window-pane-changed @1 %1")
	f.emit(`%output %2 last words`)

	events, unsubscribe := subscribeAndCollect(t, client, outputContains("@1", "last words"))
	defer unsubscribe()
	var departed bool
	for _, ev := range events {
		if wc, ok := ev.(WindowChanged); ok && wc.Window.ID == "@1" && len(wc.Window.Layout.Panes()) == 1 {
			departed = true
		}
	}
	require.True(t, departed, "the one-pane layout reached the subscriber")

	before := len(f.sentCommands())
	require.ErrorIs(t, client.KillPane(ctx, "%2"), ErrUnknownPane)
	require.ErrorIs(t, client.SelectPane(ctx, "%2", PaneSelf), ErrUnknownPane)
	_, err := client.SplitPane(ctx, "%2", SplitHorizontal)
	require.ErrorIs(t, err, ErrUnknownPane)
	require.ErrorIs(t, client.ResizePane(ctx, "%2", 10, 0), ErrUnknownPane)
	require.ErrorIs(t, client.ZoomPane(ctx, "%2"), ErrUnknownPane)
	require.ErrorIs(t, client.Write(ctx, "%2", []byte("x")), ErrUnknownPane)
	_, err = client.PaneWindow("%2")
	require.ErrorIs(t, err, ErrUnknownPane)
	require.Len(t, f.sentCommands(), before, "nothing reached tmux")

	w, err := client.PaneWindow("%1")
	require.NoError(t, err)
	require.Equal(t, "@1", w.ID)
}

func TestWriteSendsHexChunks(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	client := attachFake(t, f, Options{})

	require.NoError(t, client.Write(t.Context(), "%1", []byte("hi")))
	require.Contains(t, f.sentCommands(), "send-keys -H -t %1 68 69")

	require.NoError(t, client.Write(t.Context(), "%1", []byte(strings.Repeat("a", sendKeysChunk+4))))
	sends := 0
	for _, cmd := range f.sentCommands() {
		if strings.HasPrefix(cmd, "send-keys") {
			sends++
		}
	}
	require.Equal(t, 3, sends, "payloads are chunked")

	require.ErrorIs(t, client.Write(t.Context(), "%99", []byte("x")), ErrUnknownPane)
	require.ErrorIs(t, client.Write(t.Context(), "@1", []byte("x")), ErrUnknownPane, "input names a pane, never a window")
}

// A paste never goes over send-keys: tmux has to see it as a paste to decide
// whether the pane's program wants it bracketed.
func TestPasteLoadsABufferAndPastesIt(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	var loaded struct {
		name    string
		content string
		calls   int
	}
	client := attachFake(t, f, Options{loadBuffer: func(_ context.Context, name string, content io.Reader) error {
		body, err := io.ReadAll(content)
		require.NoError(t, err)
		loaded.name, loaded.content, loaded.calls = name, string(body), loaded.calls+1
		return nil
	}})

	require.NoError(t, client.Paste(t.Context(), "%1", []byte("first\nsecond")))
	require.Equal(t, 1, loaded.calls)
	require.Equal(t, "hive-paste-1", loaded.name, "the buffer is named after the pane, off the numbered stack")
	require.Equal(t, "first\nsecond", loaded.content)
	require.Contains(t, f.sentCommands(), "paste-buffer -d -p -b hive-paste-1 -t %1")

	for _, cmd := range f.sentCommands() {
		require.NotContains(t, cmd, "send-keys", "a paste is never keystrokes")
	}

	require.NoError(t, client.Paste(t.Context(), "%1", nil), "an empty paste is a no-op")
	require.Equal(t, 1, loaded.calls)

	require.ErrorIs(t, client.Paste(t.Context(), "%99", []byte("x")), ErrUnknownPane)
}

func TestWindowCommands(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	client := attachFake(t, f, Options{})
	ctx := t.Context()

	require.NoError(t, client.Resize(ctx, 100, 40))
	require.Contains(t, f.sentCommands(), "refresh-client -C 100,40")
	require.ErrorIs(t, client.Resize(ctx, 0, 40), ErrInvalidSize)
	require.ErrorIs(t, client.Resize(ctx, 80, maxDimension+1), ErrInvalidSize)

	require.NoError(t, client.SelectWindow(ctx, "@1"))
	require.Contains(t, f.sentCommands(), "select-window -t @1")

	id, err := client.NewWindow(ctx)
	require.NoError(t, err)
	require.Equal(t, "@9", id)
	// A new tab opens where the pane in front of the user is, not where the
	// session was started — tmux expands the format against this client.
	require.Contains(t, f.sentCommands(), `new-window -c "#{pane_current_path}" -P -F "#{window_id}"`)

	require.NoError(t, client.RenameWindow(ctx, "@1", "my logs"))
	require.Contains(t, f.sentCommands(), `rename-window -t @1 'my logs'`)
	require.ErrorIs(t, client.RenameWindow(ctx, "@1", "bad\nname"), ErrInvalidName)

	require.NoError(t, client.CloseWindow(ctx, "@1"))
	require.Contains(t, f.sentCommands(), "kill-window -t @1")

	// A window id is attacker-reachable input: it never reaches a command line
	// unless it is @<digits> and one of ours.
	require.ErrorIs(t, client.SelectWindow(ctx, "@1; kill-server"), ErrUnknownWindow)
	require.ErrorIs(t, client.CloseWindow(ctx, "@404"), ErrUnknownWindow)
}

// A window id is the only thing a caller sends, so the panes behind it are read
// live: nothing announces what a pane starts running.
func TestListPanesReadsAWindowsPanes(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows("@1 1 %1 120 40 0 b25f,120x40,0,0,1 claude")
	f.setPanes("@1",
		"%1 4812 1 0 claude",
		"%2 4820 0 0 zsh",
		"%3 0 0 1 ",
		"nonsense",
	)
	client := attachFake(t, f, Options{})

	panes, err := client.ListPanes(t.Context(), "@1")
	require.NoError(t, err)
	require.Equal(t, []Pane{
		{ID: "%1", PID: 4812, Active: true, Command: "claude"},
		{ID: "%2", PID: 4820, Command: "zsh"},
		{ID: "%3", Dead: true},
	}, panes, "an unparseable row is dropped rather than failing the read")

	// The same gate every other window command passes: an id reaches a tmux
	// command line only if it is one of ours.
	_, err = client.ListPanes(t.Context(), "@404")
	require.ErrorIs(t, err, ErrUnknownWindow)
}

// A pid past six digits is ordinary on Linux, and reading one as 0 would report
// a pane whose process cannot be found — which the caller above treats as work
// it must not kill silently.
func TestListPanesReadsALongPid(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setPanes("@1", "%1 4194302 1 0 zsh")
	client := attachFake(t, f, Options{})

	panes, err := client.ListPanes(t.Context(), "@1")
	require.NoError(t, err)
	require.Equal(t, []Pane{{ID: "%1", PID: 4194302, Active: true, Command: "zsh"}}, panes)
}

// The destination is an index into the resulting order; which tmux insertion
// expresses it, and whether the moved window keeps the selection, is what this
// pins down. -d is the flag that decides selection, and it has to follow the
// window being moved rather than being fixed.
func TestMoveWindowInsertsAtAPosition(t *testing.T) {
	t.Parallel()

	const (
		alpha   = "@1 0 %1 120 40 0 b25f,120x40,0,0,1 alpha"
		bravo   = "@2 0 %2 120 40 0 b25f,120x40,0,0,2 bravo"
		charlie = "@3 1 %3 120 40 0 b25f,120x40,0,0,3 charlie"
	)

	cases := map[string]struct {
		windowID string
		position int
		want     string
		reordered
	}{
		"a background window to the front": {
			windowID: "@2", position: 0,
			want:      "move-window -d -b -s @2 -t @1",
			reordered: reordered{bravo, alpha, charlie},
		},
		"a background window to the end": {
			windowID: "@1", position: 2,
			want:      "move-window -d -a -s @1 -t @3",
			reordered: reordered{bravo, charlie, alpha},
		},
		"one place along": {
			windowID: "@1", position: 1,
			want:      "move-window -d -a -s @1 -t @2",
			reordered: reordered{bravo, alpha, charlie},
		},
		"the active window keeps the selection": {
			windowID: "@3", position: 0,
			want:      "move-window -b -s @3 -t @1",
			reordered: reordered{charlie, alpha, bravo},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := newFakeTmux(t, "hive-demo")
			f.setWindows(alpha, bravo, charlie)
			client := attachFake(t, f, Options{})
			f.setWindows(tc.reordered...)

			windows, err := client.MoveWindow(t.Context(), tc.windowID, tc.position)
			require.NoError(t, err)

			require.Contains(t, f.sentCommands(), tc.want)
			require.Contains(t, f.sentCommands(), "move-window -r",
				"an insert leaves the session's indices with gaps otherwise")
			require.Equal(t, windowIDsOf(tc.reordered), windowIDs(windows),
				"the answer is the order tmux settled on, read back rather than assumed")
			require.Equal(t, windowIDs(windows), windowIDs(client.Windows()))
		})
	}
}

// A move is an unlink and a relink, so tmux reports the window it moved as
// closed. Acting on that would tear the tab and its terminal down and rebuild
// them blank — and, for the window that was selected, move the selection too.
func TestMoveWindowSurvivesTheRelinkClose(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows("@1 1 %1 120 40 0 b25f,120x40,0,0,1 alpha", "@2 0 %2 120 40 0 b25f,120x40,0,0,2 bravo")
	client := attachFake(t, f, Options{})

	f.setOnCommand(func(cmd string) {
		if strings.HasPrefix(cmd, "move-window -") && !strings.HasPrefix(cmd, "move-window -r") {
			f.setWindows("@2 0 %2 120 40 0 b25f,120x40,0,0,2 bravo", "@1 1 %1 120 40 0 b25f,120x40,0,0,1 alpha")
			f.emit("%window-add @1")
			f.emit("%window-close @1")
		}
	})

	windows, err := client.MoveWindow(t.Context(), "@1", 1)
	require.NoError(t, err)
	require.Equal(t, []string{"@2", "@1"}, windowIDs(windows))
	require.Equal(t, []string{"@2", "@1"}, windowIDs(client.Windows()),
		"the moved window is still in the set the close claimed to remove")

	// The guard lifts with the move: a real close still closes.
	f.setWindows("@2 0 %2 120 40 0 b25f,120x40,0,0,2 bravo")
	f.emit("%window-close @1")
	events, unsubscribe := subscribeAndCollect(t, client, func(ev Event) bool {
		wc, ok := ev.(WindowChanged)
		return ok && wc.Kind == WindowClosed && wc.Window.ID == "@1"
	})
	defer unsubscribe()
	require.NotEmpty(t, events)
}

func TestMoveWindowRejectsWhatItCannotPlace(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows("@1 1 %1 120 40 0 b25f,120x40,0,0,1 alpha", "@2 0 %2 120 40 0 b25f,120x40,0,0,2 bravo")
	client := attachFake(t, f, Options{})
	ctx := t.Context()

	_, err := client.MoveWindow(ctx, "@404", 0)
	require.ErrorIs(t, err, ErrUnknownWindow)
	_, err = client.MoveWindow(ctx, "@1", 2)
	require.ErrorIs(t, err, ErrInvalidPosition)
	_, err = client.MoveWindow(ctx, "@1", -1)
	require.ErrorIs(t, err, ErrInvalidPosition)
	require.Zero(t, f.countCommands("move-window"), "nothing rejected reaches tmux")

	// Landing where it already is costs tmux a whole index shift for no reorder.
	windows, err := client.MoveWindow(ctx, "@1", 0)
	require.NoError(t, err)
	require.Equal(t, []string{"@1", "@2"}, windowIDs(windows))
	require.Zero(t, f.countCommands("move-window"))
}

// reordered is the list-windows the fake answers with once the move has run.
type reordered []string

func windowIDsOf(lines []string) []string {
	ids := make([]string, 0, len(lines))
	for _, line := range lines {
		w, _ := parseWindowLine(line)
		ids = append(ids, w.ID)
	}
	return ids
}

func TestCommandErrorSurfacesToCaller(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.failures["kill-window"] = "can't find window: @1"
	client := attachFake(t, f, Options{})

	var cmdErr *CommandError
	require.ErrorAs(t, client.CloseWindow(t.Context(), "@1"), &cmdErr)
	require.Equal(t, "can't find window: @1", cmdErr.Message)
}

func TestExitEndsTheStream(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		trigger func(f *fakeTmux)
		reason  string
	}{
		"%exit notification": {
			trigger: func(f *fakeTmux) { f.emit("%exit server exited") },
			reason:  "server exited",
		},
		"stdout close": {
			trigger: func(f *fakeTmux) { f.closeStreams() },
			reason:  "exited",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := newFakeTmux(t, "hive-demo")
			client := attachFake(t, f, Options{})
			ch, unsubscribe := client.Subscribe()
			defer unsubscribe()

			tc.trigger(f)

			events := collect(t, ch, lifecycleIs(LifecycleExited))
			require.Equal(t, tc.reason, lastLifecycle(t, events).Message)

			// The reader is joined and the process reaped by the time the
			// exit event is published, so Close is a no-op that returns.
			require.NoError(t, client.Close(t.Context()))
		})
	}
}

func TestCloseDetachesAndIsIdempotent(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	client := attachFake(t, f, Options{})
	ch, unsubscribe := client.Subscribe()
	defer unsubscribe()

	require.NoError(t, client.Close(t.Context()))
	require.NoError(t, client.Close(t.Context()))

	require.Contains(t, f.sentCommands(), "detach")

	events := collect(t, ch, lifecycleIs(LifecycleExited))
	require.Equal(t, "detached", lastLifecycle(t, events).Message)

	select {
	case _, open := <-ch:
		require.False(t, open, "the subscription closes with the client")
	case <-time.After(2 * time.Second):
		t.Fatal("subscription stayed open after close")
	}
}

func TestProtocolDesyncTearsDownTheClient(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	client := attachFake(t, f, Options{})
	ch, unsubscribe := client.Subscribe()
	defer unsubscribe()

	f.emit("%end 1 1 1")

	events := collect(t, ch, lifecycleIs(LifecycleExited))
	require.Equal(t, "protocol error", lastLifecycle(t, events).Message)

	var sawError bool
	for _, ev := range events {
		if lc, ok := ev.(LifecycleChanged); ok && lc.Kind == LifecycleError {
			sawError = true
			require.Contains(t, lc.Message, "protocol desync")
		}
	}
	require.True(t, sawError, "the desync is reported before the exit")
}

// Not parallel, and neither is TestOutputFromAnInactivePaneIsCounted: the
// instruments carry no session attribute, so every client in the process writes
// to the same series and a concurrent test would land in this delta. Go defers
// parallel tests to the end of the package, which leaves these two alone.
func TestStreamInstrumentsRecord(t *testing.T) {
	f := newFakeTmux(t, "hive-demo")
	f.setWindows("@1 1 %1 120 1 0 b25f,120x1,0,0,1 claude")
	f.setCapture("%1", "ready")

	before := readStreamCounts(t)
	client := attachFake(t, f, Options{})

	f.emit(`%output %1 live\015\012`)
	f.emit("%pause %1")
	f.emit("%continue %1")

	_, unsubscribe := subscribeAndCollect(t, client, lifecycleIs(LifecycleResumed))
	defer unsubscribe()

	after := readStreamCounts(t)
	require.Equal(t, int64(len("ready")+len("live\r\n")), after.bytes-before.bytes)
	require.Equal(t, int64(1), after.paused-before.paused)
	require.Equal(t, int64(1), after.resumed-before.resumed)
	require.Greater(t, after.depth, before.depth, "a publish records the backlog depth")
}

func TestOutputFromAnInactivePaneIsCounted(t *testing.T) {
	f := newFakeTmux(t, "hive-demo")
	f.setWindows("@1 1 %2 120 1 0 f91d,120x1,0,0{60x1,0,0,1,59x1,61,0,2} claude")

	before := readStreamCounts(t)
	client := attachFake(t, f, Options{})

	f.emit(`%output %1 background\015\012`)
	f.emit(`%output %2 foreground\015\012`)

	_, unsubscribe := subscribeAndCollect(t, client, outputContains("@1", "foreground"))
	defer unsubscribe()

	after := readStreamCounts(t)
	require.Equal(t, int64(len("background\r\nforeground\r\n")), after.bytes-before.bytes)
}

func TestAttachRejectsBadOptions(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	ctx := t.Context()

	_, err := Attach(ctx, ctx, Options{Slug: "hive", Cols: 0, Rows: 24, newProcess: f.factory()})
	require.ErrorIs(t, err, ErrInvalidSize)

	_, err = Attach(ctx, ctx, Options{Slug: "", Cols: 80, Rows: 24, newProcess: f.factory()})
	require.ErrorIs(t, err, ErrNotAttached)
}

func TestAttachFailsWhenTheStreamEndsBeforeHandshake(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.closeStreams()

	ctx := t.Context()
	_, err := Attach(ctx, ctx, Options{Slug: "hive-demo", Cols: 80, Rows: 24, newProcess: f.factory()})
	require.Error(t, err)
}

func subscribeAndCollect(t *testing.T, client *Client, stop func(Event) bool) ([]Event, func()) {
	t.Helper()
	ch, unsubscribe := client.Subscribe()
	return collect(t, ch, stop), unsubscribe
}
