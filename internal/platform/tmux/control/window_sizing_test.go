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
	return attachSizingBounded(t, f, 0)
}

// attachSizingBounded shortens the bound on state-changing round trips, for a
// test that freezes the fake server and waits for that bound to pass.
func attachSizingBounded(t *testing.T, f *fakeTmux, bound time.Duration) *Client {
	t.Helper()
	ctx := t.Context()
	client, err := Attach(ctx, ctx, Options{Slug: f.slug, newProcess: f.factory(), stateChangeTimeout: bound})
	require.NoError(t, err)
	return client
}

const frozenBound = 50 * time.Millisecond

// waitClosed fails when Close has not returned once the bound and the detach
// have had three times their room.
func waitClosed(t *testing.T, closed <-chan struct{}) {
	t.Helper()
	select {
	case <-closed:
	case <-time.After(3*frozenBound + detachTimeout):
		t.Fatal("Close stayed behind a frozen server")
	}
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

// RenameSessionIfPresent renames the session and closes the client after, when
// every client reports the new name; the count must still find them.
func TestCloseCountsControlClientsBySessionIDAfterARename(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows(twoWindows1)
	f.setMark("@1", aggressiveResizeInherited)
	client := attachSizing(t, f)
	f.renameSession("hive-renamed")
	f.setClients("1 hive-renamed", "1 hive-renamed")

	require.NoError(t, client.Close(context.WithoutCancel(t.Context())))
	require.Empty(t, f.commandsMatching("set-option -w -t @1"))
}

// A client count that cannot be read restores anyway: leaving the option off
// for good is the worse of the two outcomes.
func TestCloseRestoresWhenTheClientCountCannotBeRead(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows(twoWindows1)
	f.setMark("@1", aggressiveResizeInherited)
	f.setClients("1 hive-demo", "1 hive-demo")
	f.mu.Lock()
	f.failures["list-clients"] = "no"
	f.mu.Unlock()
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
// the state-change bound, and then the kill in teardown can run.
func TestAFrozenServerCannotHoldClosePastThePreparationBound(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows(twoWindows1)
	f.setOption(globalOption, "on")
	client := attachSizingBounded(t, f, frozenBound)
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
	waitClosed(t, closed)
	<-resized
}

// A re-attach clears the per-window sizes on the attach request's context,
// which has no deadline; a frozen server must not hold Close behind that
// either.
func TestAFrozenServerCannotHoldClosePastTheClearBound(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows(twoWindows1)
	client := attachSizingBounded(t, f, frozenBound)
	require.NoError(t, client.ResizeWindow(t.Context(), "@1", 60, 20))
	f.setSilent("refresh-client -C @1:")

	renegotiated := make(chan struct{})
	go func() {
		defer close(renegotiated)
		_ = client.Renegotiate(context.WithoutCancel(t.Context()), 120, 40)
	}()
	f.awaitCommands(t, "refresh-client -C @1:", 2)

	closed := make(chan struct{})
	go func() {
		defer close(closed)
		_ = client.Close(context.WithoutCancel(t.Context()))
	}()
	waitClosed(t, closed)
	<-renegotiated
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

// The size goes out even when the option could not be changed, so the clear
// must cover that window too.
func TestRenegotiateClearsASizeWhosePreparationFailed(t *testing.T) {
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
	require.NoError(t, client.ResizeWindow(ctx, "@1", 60, 20))
	require.NoError(t, client.Renegotiate(ctx, 120, 40))
	require.Contains(t, f.sentCommands(), "refresh-client -C @1:")
}
