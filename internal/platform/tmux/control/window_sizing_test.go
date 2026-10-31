package tmuxcc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	twoWindows1 = "@1 1 %1 120 40 0 b25f,120x40,0,0,1 claude"
	twoWindows2 = "@2 0 %2 120 40 0 b25f,120x40,0,0,2 shell"
)

func attachSizing(t *testing.T, f *fakeTmux) *Client {
	t.Helper()
	ctx := t.Context()
	client, err := Attach(ctx, ctx, Options{Slug: f.slug, newProcess: f.factory()})
	require.NoError(t, err)
	return client
}

func TestResizeWindowSizesAWindowThatIsNotCurrent(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows(twoWindows1, twoWindows2)
	f.setOption(globalOption, "on")
	client := attachSizing(t, f)

	ctx := t.Context()
	require.NoError(t, client.ResizeWindow(ctx, "@2", 60, 20))
	require.NoError(t, client.ResizeWindow(ctx, "@2", 61, 20))
	require.Equal(t, []string{
		"show-options -w -v -A -t @2 aggressive-resize",
		"show-options -w -v -q -t @2 @hive-aggressive-resize",
		"show-options -w -v -q -t @2 aggressive-resize",
		"set-option -w -t @2 @hive-aggressive-resize inherit",
		"set-option -w -t @2 aggressive-resize off",
		"refresh-client -C @2:60x20",
		"refresh-client -C @2:61x20",
	}, f.commandsMatching("@2"), "the mark goes before the change, and the change before the size")

	require.NoError(t, client.Close(context.WithoutCancel(ctx)))
	require.Contains(t, f.sentCommands(), "set-option -w -t @2 -u aggressive-resize")
	require.Contains(t, f.sentCommands(), "set-option -w -t @2 -u @hive-aggressive-resize")
}

func TestResizeWindowLeavesTheOptionAloneWhenItIsOff(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows(twoWindows1)
	f.setOption(globalOption, "off")
	client := attachSizing(t, f)

	ctx := t.Context()
	require.NoError(t, client.ResizeWindow(ctx, "@1", 60, 20))
	require.NoError(t, client.Close(context.WithoutCancel(ctx)))
	for _, cmd := range f.sentCommands() {
		require.NotContains(t, cmd, "set-option")
	}
}

func TestCloseRestoresAWindowsOwnAggressiveResize(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows(twoWindows1)
	f.setOption("@1", "on")
	client := attachSizing(t, f)

	ctx := t.Context()
	require.NoError(t, client.ResizeWindow(ctx, "@1", 60, 20))
	require.Contains(t, f.sentCommands(), "set-option -w -t @1 @hive-aggressive-resize on")
	require.NoError(t, client.Close(context.WithoutCancel(ctx)))
	require.Contains(t, f.sentCommands(), "set-option -w -t @1 aggressive-resize on")
}

// A client that ended without Close leaves the option off and the mark behind.
// A later client must restore that even when it never sizes the window, and must
// not save the "off" it finds as the window's own value.
func TestCloseRestoresWhatAnEarlierClientLeftBehind(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows(twoWindows1, twoWindows2)
	f.setMark("@2", aggressiveResizeInherited)
	f.setOption("@2", "off")
	client := attachSizing(t, f)

	require.NoError(t, client.Close(context.WithoutCancel(t.Context())))
	require.Contains(t, f.sentCommands(), "set-option -w -t @2 -u aggressive-resize")
	require.Contains(t, f.sentCommands(), "set-option -w -t @2 -u @hive-aggressive-resize")
}

func TestResizeWindowKeepsAnEarlierMark(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows(twoWindows1)
	f.setMark("@1", "on")
	f.setOption(globalOption, "on")
	client := attachSizing(t, f)
	t.Cleanup(func() { _ = client.Close(context.WithoutCancel(t.Context())) })

	require.NoError(t, client.ResizeWindow(t.Context(), "@1", 60, 20))
	require.NotContains(t, f.sentCommands(), "set-option -w -t @1 @hive-aggressive-resize inherit")
	require.Contains(t, f.sentCommands(), "set-option -w -t @1 aggressive-resize off")
}

func TestCloseLeavesTheOptionWhileAnotherControlClientIsAttached(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows(twoWindows1)
	f.setMark("@1", aggressiveResizeInherited)
	f.setClients("1 hive-demo", "1 hive-demo", "0 hive-demo", "1 other")
	client := attachSizing(t, f)

	require.NoError(t, client.Close(context.WithoutCancel(t.Context())))
	require.Empty(t, f.commandsMatching("set-option -w -t @1"))
}

func TestCloseCountsOnlyControlClientsOnThisSession(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows(twoWindows1)
	f.setMark("@1", aggressiveResizeInherited)
	f.setClients("1 hive-demo", "0 hive-demo", "1 other")
	client := attachSizing(t, f)

	require.NoError(t, client.Close(context.WithoutCancel(t.Context())))
	require.Contains(t, f.sentCommands(), "set-option -w -t @1 -u aggressive-resize")
}

// The request context ending, as a reloading webview does, must not stop the
// change half way: tmux may already have applied the part that was sent.
func TestResizeWindowFinishesThePreparationWhenTheRequestIsCancelled(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows(twoWindows1)
	f.setOption(globalOption, "on")
	client := attachSizing(t, f)
	t.Cleanup(func() { _ = client.Close(context.WithoutCancel(t.Context())) })

	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	_ = client.ResizeWindow(cancelled, "@1", 60, 20)
	require.Contains(t, f.sentCommands(), "set-option -w -t @1 aggressive-resize off")
}

func TestAFailedPreparationIsTriedAgain(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows(twoWindows1)
	f.setOption(globalOption, "on")
	f.mu.Lock()
	f.failures["set-option -w -t @1 aggressive-resize"] = "no"
	f.mu.Unlock()
	client := attachSizing(t, f)
	t.Cleanup(func() { _ = client.Close(context.WithoutCancel(t.Context())) })

	ctx := t.Context()
	require.NoError(t, client.ResizeWindow(ctx, "@1", 60, 20), "the size is still sent")
	f.mu.Lock()
	delete(f.failures, "set-option -w -t @1 aggressive-resize")
	f.mu.Unlock()
	require.NoError(t, client.ResizeWindow(ctx, "@1", 61, 20))
	require.Equal(t, 2, f.countCommands("set-option -w -t @1 aggressive-resize off"))
}

func TestResizeAfterCloseChangesNothing(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows(twoWindows1)
	f.setOption(globalOption, "on")
	client := attachSizing(t, f)

	ctx := context.WithoutCancel(t.Context())
	client.restoreAggressiveResize(ctx)
	client.sizeWhileNotCurrent(ctx, "@1")
	require.Empty(t, f.commandsMatching("set-option"))
	require.NoError(t, client.Close(ctx))
}

// A frozen server must not hold Close behind a preparation: it gives up after
// imageRestoreTimeout, and then the kill in teardown can run.
func TestAFrozenServerCannotHoldClosePastThePreparationBound(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows(twoWindows1)
	f.setOption(globalOption, "on")
	client := attachSizing(t, f)
	f.setSilent("show-options")

	resized := make(chan struct{})
	go func() {
		defer close(resized)
		_ = client.ResizeWindow(context.WithoutCancel(t.Context()), "@1", 60, 20)
	}()
	f.awaitCommands(t, "show-options -w -v -A -t @1 aggressive-resize", 1)

	closed := make(chan struct{})
	go func() {
		defer close(closed)
		_ = client.Close(context.WithoutCancel(t.Context()))
	}()
	select {
	case <-closed:
	case <-time.After(3*imageRestoreTimeout + detachTimeout):
		t.Fatal("Close stayed behind a preparation on a frozen server")
	}
	<-resized
}

// A surface that re-attaches to a live client knows no per-window sizes any more,
// so none may survive in tmux to outrank the size it votes.
func TestRenegotiateClearsPerWindowSizes(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows(twoWindows1, twoWindows2)
	client := attachSizing(t, f)
	t.Cleanup(func() { _ = client.Close(context.WithoutCancel(t.Context())) })

	ctx := t.Context()
	require.NoError(t, client.ResizeWindow(ctx, "@2", 60, 20))
	require.NoError(t, client.Renegotiate(ctx, 120, 40))
	require.Contains(t, f.sentCommands(), "refresh-client -C @2:")
}
