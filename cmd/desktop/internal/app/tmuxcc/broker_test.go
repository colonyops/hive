package tmuxcc

import (
	"runtime"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

func outputEvent(window, data string) Output {
	return Output{At: time.Now(), WindowID: window, PaneID: "%1", Data: []byte(data)}
}

func requireOutput(t *testing.T, ev Event) Output {
	t.Helper()
	out, ok := ev.(Output)
	require.True(t, ok, "expected Output, got %#v", ev)
	return out
}

func windowEvent(window, name string) WindowChanged {
	return WindowChanged{Kind: WindowRenamed, Window: Window{ID: window, Name: name}}
}

func requireLifecycle(t *testing.T, ev Event) LifecycleChanged {
	t.Helper()
	lc, ok := ev.(LifecycleChanged)
	require.True(t, ok, "expected LifecycleChanged, got %#v", ev)
	return lc
}

func requireWindow(t *testing.T, ev Event) WindowChanged {
	t.Helper()
	wc, ok := ev.(WindowChanged)
	require.True(t, ok, "expected WindowChanged, got %#v", ev)
	return wc
}

// hasOverflowed reads the flag publish sets, which the onOverflow callback
// trails: asserting a broker did *not* overflow cannot wait on a channel that is
// never closed.
func hasOverflowed(b *broker) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.overflow
}

// backlogHas reports whether the broker is holding output carrying want. A test
// that waits on depth alone is asking "has anything landed", which a deferred
// first paint also answers — and then the repaint's reset can run before the
// byte it is supposed to supersede has even been published.
func backlogHas(b *broker, want string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ev := range b.buf {
		if out, isOutput := ev.(Output); isOutput && strings.Contains(string(out.Data), want) {
			return true
		}
	}
	return false
}

func receive(t *testing.T, ch <-chan Event) Event {
	t.Helper()
	select {
	case ev, ok := <-ch:
		require.True(t, ok, "channel closed")
		return ev
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for an event")
		return nil
	}
}

// Output produced before anyone subscribes is what carries first paint to a
// WebSocket that connects after Attach.
func TestBrokerReplaysBacklogOnSubscribe(t *testing.T) {
	t.Parallel()

	b := newBroker(backlogBounds{}, nil)
	b.publish(outputEvent("@1", "first"))
	b.publish(outputEvent("@1", "second"))
	require.Equal(t, len("firstsecond"), b.depth())

	ch, unsubscribe := b.subscribe()
	defer unsubscribe()

	require.Equal(t, outputEvent("@1", "first").Data, requireOutput(t, receive(t, ch)).Data)
	require.Equal(t, outputEvent("@1", "second").Data, requireOutput(t, receive(t, ch)).Data)
}

func TestBrokerSubscribeReplacesPrevious(t *testing.T) {
	t.Parallel()

	b := newBroker(backlogBounds{}, nil)
	first, _ := b.subscribe()
	second, unsubscribe := b.subscribe()
	defer unsubscribe()

	select {
	case _, ok := <-first:
		require.False(t, ok, "the replaced channel closes")
	case <-time.After(2 * time.Second):
		t.Fatal("the replaced channel stayed open")
	}

	b.publish(outputEvent("@1", "live"))
	require.Equal(t, []byte("live"), requireOutput(t, receive(t, second)).Data)
}

// A stale unsubscribe belongs to a generation that is already gone; acting on
// it would detach the subscriber that replaced it.
func TestBrokerStaleUnsubscribeIsInert(t *testing.T) {
	t.Parallel()

	b := newBroker(backlogBounds{}, nil)
	_, staleUnsubscribe := b.subscribe()
	current, unsubscribe := b.subscribe()
	defer unsubscribe()

	staleUnsubscribe()

	b.publish(outputEvent("@1", "live"))
	require.Equal(t, []byte("live"), requireOutput(t, receive(t, current)).Data)
}

func TestBrokerUnsubscribeClosesChannel(t *testing.T) {
	t.Parallel()

	b := newBroker(backlogBounds{}, nil)
	ch, unsubscribe := b.subscribe()
	unsubscribe()

	select {
	case _, ok := <-ch:
		require.False(t, ok)
	case <-time.After(2 * time.Second):
		t.Fatal("channel stayed open after unsubscribe")
	}
}

// A terminal stream cannot drop bytes, so crossing the bound stops the backlog
// dead and reports it. Everything droppable is refused from that moment until a
// resync clears it — the recovery is a repaint, not a replay.
func TestBrokerOverflowStopsTheBacklogAndReportsIt(t *testing.T) {
	t.Parallel()

	overflowed := make(chan struct{})
	b := newBroker(backlogBounds{bytes: 64}, func() { close(overflowed) })

	b.publish(outputEvent("@1", "0123456789"))
	b.publish(outputEvent("@1", string(make([]byte, 128))))

	select {
	case <-overflowed:
	case <-time.After(2 * time.Second):
		t.Fatal("overflow was not reported")
	}

	// Lifecycle events still get through: whichever of degraded or exited the
	// overflow resolves into has to reach the subscriber.
	ch, unsubscribe := b.subscribe()
	defer unsubscribe()
	b.publish(LifecycleChanged{Kind: LifecycleExited, Message: "overflow"})

	require.Equal(t, outputEvent("@1", "0123456789").Data, requireOutput(t, receive(t, ch)).Data)
	require.Equal(t,
		LifecycleChanged{Kind: LifecycleExited, Message: "overflow"},
		requireLifecycle(t, receive(t, ch)))
}

// The recovery keeps the subscriber the reset path drops: the emulator that was
// reading is the one the repaint behind the resync is drawn for, so handing it
// a closed channel would end the very stream this exists to save.
func TestBrokerResyncKeepsTheSubscriberAndDropsTheBacklog(t *testing.T) {
	t.Parallel()

	overflowed := make(chan struct{})
	b := newBroker(backlogBounds{bytes: 64}, func() { close(overflowed) })
	ch, unsubscribe := b.subscribe()
	defer unsubscribe()

	// Nobody is reading ch, so the pump parks on the first event and the rest
	// pile up behind it — which is how the bound is crossed at all.
	for range 4 {
		b.publish(outputEvent("@1", strings.Repeat("x", 32)))
	}
	select {
	case <-overflowed:
	case <-time.After(2 * time.Second):
		t.Fatal("overflow was not reported")
	}

	dropped, ok := b.resync(time.Now())
	require.True(t, ok)
	require.Positive(t, dropped, "the bytes the subscriber never took are reported")
	require.Zero(t, b.depth())
	require.False(t, hasOverflowed(b), "the resync lifts the state that was refusing output")

	b.publish(LifecycleChanged{Kind: LifecycleDegraded, Message: "overflow"})
	b.publish(outputEvent("@1", "REPAINTED"))

	// The head the pump was parked on still arrives — it was already in flight
	// to a subscriber that is staying — and the repaint follows it.
	var painted string
	for painted == "" {
		if out, isOutput := receive(t, ch).(Output); isOutput && string(out.Data) == "REPAINTED" {
			painted = string(out.Data)
		}
	}
	require.Equal(t, "REPAINTED", painted)
}

// Overflow re-arms after a resync. The flag is what gates the callback, so a
// stale one would leave a second flood silently refusing output on a stream
// nobody ever told.
func TestBrokerOverflowReportsAgainAfterAResync(t *testing.T) {
	t.Parallel()

	overflows := make(chan struct{}, 4)
	b := newBroker(backlogBounds{bytes: 64}, func() { overflows <- struct{}{} })
	b.resyncCooldown = 0

	flood := func() {
		for range 4 {
			b.publish(outputEvent("@1", strings.Repeat("x", 32)))
		}
		select {
		case <-overflows:
		case <-time.After(2 * time.Second):
			t.Fatal("overflow was not reported")
		}
	}

	flood()
	_, ok := b.resync(time.Now())
	require.True(t, ok)
	flood()
}

// A resync that does not survive its cooldown was not a recovery: the flood is
// outrunning the subscriber, and capturing every window again would cost more
// than it saves. Refusing is what keeps the fatal path reachable.
func TestBrokerRefusesAResyncInsideTheCooldown(t *testing.T) {
	t.Parallel()

	b := newBroker(backlogBounds{bytes: 64}, nil)
	start := time.Now()

	_, ok := b.resync(start)
	require.True(t, ok)

	_, ok = b.resync(start.Add(resyncCooldown - time.Millisecond))
	require.False(t, ok, "a second resync inside the cooldown is refused")

	_, ok = b.resync(start.Add(resyncCooldown))
	require.True(t, ok, "past the cooldown the recovery is worth trying again")
}

// A closing broker has nothing to recover into: its backlog is the last thing a
// live subscriber will read, and the exit reason is behind it.
func TestBrokerRefusesAResyncOnceClosed(t *testing.T) {
	t.Parallel()

	b := newBroker(backlogBounds{}, nil)
	b.close()

	_, ok := b.resync(time.Now())
	require.False(t, ok)
}

// Only Output is charged bytes, so a session that renames windows or switches
// panes in a loop with nobody subscribed is invisible to the byte bound. The
// count bound is what keeps that backlog finite.
func TestBrokerOverflowsOnEventCount(t *testing.T) {
	t.Parallel()

	overflowed := make(chan struct{})
	b := newBroker(backlogBounds{events: 4}, func() { close(overflowed) })

	for range 5 {
		b.publish(windowEvent("@1", "renamed"))
	}

	select {
	case <-overflowed:
	case <-time.After(2 * time.Second):
		t.Fatal("the count bound never reported overflow")
	}
	require.Zero(t, b.depth(), "a window event is still worth no bytes")

	// The fifth was dropped rather than buffered, and the teardown reason still
	// gets in behind the four that fit.
	ch, unsubscribe := b.subscribe()
	defer unsubscribe()
	b.publish(LifecycleChanged{Kind: LifecycleExited, Message: "overflow"})

	for range 4 {
		require.Equal(t, WindowRenamed, requireWindow(t, receive(t, ch)).Kind)
	}
	require.Equal(t, LifecycleExited, requireLifecycle(t, receive(t, ch)).Kind)
}

// A backlog sitting exactly on the count bound must still admit a lifecycle
// event. Bounding one would make it the event that trips overflow, and the same
// branch would then discard it — the stream would end with nothing saying why.
func TestBrokerLifecycleNeverTripsTheCountBound(t *testing.T) {
	t.Parallel()

	b := newBroker(backlogBounds{events: 4}, nil)

	for range 4 {
		b.publish(windowEvent("@1", "renamed"))
	}
	b.publish(LifecycleChanged{Kind: LifecycleExited, Message: "detached"})
	require.False(t, hasOverflowed(b), "the lifecycle event became the overflow trigger")

	ch, unsubscribe := b.subscribe()
	defer unsubscribe()
	for range 4 {
		requireWindow(t, receive(t, ch))
	}
	require.Equal(t, "detached", requireLifecycle(t, receive(t, ch)).Message)
}

// Overflow closes the broker with the subscriber behind — by definition, since
// a subscriber that could keep up is what would have kept the backlog small.
// The exit reason still has to arrive, or the frontend sees a bare socket close
// with nothing to report.
func TestBrokerCloseDeliversLifecycleToADrainingSubscriber(t *testing.T) {
	t.Parallel()

	b := newBroker(backlogBounds{}, nil)
	ch, unsubscribe := b.subscribe()
	defer unsubscribe()

	for range 128 {
		b.publish(outputEvent("@1", "x"))
	}
	b.publish(LifecycleChanged{Kind: LifecycleExited, Message: "overflow"})
	b.close()

	for {
		ev, ok := <-ch
		if !ok {
			t.Fatal("the channel closed before the exit reason arrived")
		}
		if lc, isLifecycle := ev.(LifecycleChanged); isLifecycle {
			require.Equal(t, LifecycleChanged{Kind: LifecycleExited, Message: "overflow"}, lc)
			return
		}
	}
}

func TestBrokerCloseDrainsThenClosesChannel(t *testing.T) {
	t.Parallel()

	b := newBroker(backlogBounds{}, nil)
	ch, unsubscribe := b.subscribe()
	defer unsubscribe()

	b.publish(LifecycleChanged{Kind: LifecycleExited, Message: "detached"})
	b.close()

	require.Equal(t, LifecycleExited, requireLifecycle(t, receive(t, ch)).Kind)
	select {
	case _, ok := <-ch:
		require.False(t, ok)
	case <-time.After(2 * time.Second):
		t.Fatal("channel stayed open after close")
	}
}

// A WebSocket reconnect re-subscribes, and the emulator it feeds cannot be
// handed a stream with a hole in it: whatever the replaced subscriber never
// took must still be in the backlog.
func TestBrokerReplacementSubscriberSeesEveryUndeliveredEvent(t *testing.T) {
	t.Parallel()

	b := newBroker(backlogBounds{}, nil)
	stalled, _ := b.subscribe()

	const count = 8
	for i := range count {
		b.publish(outputEvent("@1", strconv.Itoa(i)))
	}
	require.Equal(t, count, b.depth(), "nothing leaves the backlog undelivered")

	replacement, unsubscribe := b.subscribe()
	defer unsubscribe()

	// Read until the bytes are all accounted for rather than for a fixed number
	// of events: queued output for one pane is coalesced, so what the
	// replacement is owed is a byte stream with no hole in it, not a particular
	// number of frames.
	var got strings.Builder
	for got.Len() < count {
		got.Write(requireOutput(t, receive(t, replacement)).Data)
	}
	require.Equal(t, "01234567", got.String())

	select {
	case _, open := <-stalled:
		require.False(t, open, "the replaced channel closes without consuming the backlog")
	case <-time.After(2 * time.Second):
		t.Fatal("the replaced channel stayed open")
	}
}

// The repaint behind a re-attach publishes a snapshot of everything the
// backlog holds, so both have to go: a subscriber left draining would take the
// snapshot with it, and the backlog would replay pre-snapshot bytes into the
// emulator the snapshot is drawn for.
func TestBrokerResetDropsTheBacklogAndItsSubscriber(t *testing.T) {
	t.Parallel()

	b := newBroker(backlogBounds{}, nil)
	dropped, _ := b.subscribe()
	b.publish(outputEvent("@1", "stale"))

	b.reset()
	require.Zero(t, b.depth())

	b.publish(outputEvent("@1", "painted"))
	ch, unsubscribe := b.subscribe()
	defer unsubscribe()
	require.Equal(t, []byte("painted"), requireOutput(t, receive(t, ch)).Data,
		"the next subscriber opens on the snapshot, not on what preceded it")

	select {
	case _, open := <-dropped:
		require.False(t, open, "the subscriber the repaint is not for is released")
	case <-time.After(2 * time.Second):
		t.Fatal("the dropped channel stayed open")
	}
}

// A pump parked on a subscriber that stopped reading cannot be woken by
// Cond.Broadcast, so close has to reach it another way — otherwise a stalled
// WebSocket peer leaks the pump past client teardown.
func TestBrokerCloseFreesAPumpParkedOnAStalledSubscriber(t *testing.T) {
	// Deliberately not parallel: it counts this package's live goroutines.
	before := clientGoroutines()

	// synctest.Wait returns once the pump is durably blocked, which is the
	// state this is about; polling for it only ever approximated that.
	synctest.Test(t, func(t *testing.T) {
		b := newBroker(backlogBounds{}, nil)
		ch, _ := b.subscribe()
		for range 8 {
			b.publish(outputEvent("@1", "x"))
		}
		b.publish(LifecycleChanged{Kind: LifecycleExited, Message: "overflow"})

		synctest.Wait()
		require.Greater(t, clientGoroutines(), before, "the pump should be parked on the unread channel")

		b.close()

		// The pump gives the stalled subscriber finalDelivery to catch up before
		// it abandons the send; on the fake clock that window costs nothing, and
		// asserting past exactly it is tighter than polling a 5s ceiling was.
		time.Sleep(finalDelivery)
		synctest.Wait()
		require.LessOrEqual(t, clientGoroutines(), before, "the pump outlived the closed broker")
		_ = ch
	})
}

// clientGoroutines counts the long-lived goroutines this package starts. Tests
// that use it must not run in parallel.
func clientGoroutines() int {
	buf := make([]byte, 1<<16)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	count := 0
	for stack := range strings.SplitSeq(string(buf), "\n\n") {
		switch {
		case strings.Contains(stack, "tmuxcc.(*broker).pump"),
			strings.Contains(stack, "tmuxcc.(*Client).worker"),
			strings.Contains(stack, "tmuxcc.(*Client).read"):
			count++
		}
	}
	return count
}

// The reader goroutine is the one calling publish; if it ever blocked, a
// pending command's %end reply would deadlock behind it.
func TestBrokerPublishNeverBlocks(t *testing.T) {
	t.Parallel()

	b := newBroker(backlogBounds{bytes: 1 << 20}, nil)
	_, unsubscribe := b.subscribe()
	defer unsubscribe()

	done := make(chan struct{})
	go func() {
		for range 256 {
			b.publish(outputEvent("@1", "x"))
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("publish blocked on an unread subscriber")
	}
}

// Coalescing exists to cut the number of frames a busy pane produces, and it is
// only allowed to do that if the bytes and their order survive it exactly.
func TestBrokerCoalescesQueuedOutputWithoutLosingBytes(t *testing.T) {
	t.Parallel()

	b := newBroker(backlogBounds{}, nil)
	stalled, _ := b.subscribe()

	const count = 64
	var want strings.Builder
	for i := range count {
		chunk := strconv.Itoa(i) + ","
		want.WriteString(chunk)
		b.publish(outputEvent("@1", chunk))
	}
	require.Equal(t, want.Len(), b.depth(), "coalescing must not change what the backlog is charged")

	replacement, unsubscribe := b.subscribe()
	defer unsubscribe()

	var got strings.Builder
	frames := 0
	for got.Len() < want.Len() {
		got.Write(requireOutput(t, receive(t, replacement)).Data)
		frames++
	}
	require.Equal(t, want.String(), got.String(), "every byte, in order")
	require.Less(t, frames, count, "a stalled subscriber's queued output is delivered in fewer frames than it was published in")

	<-stalled
}

// Output for different windows must never be folded together: each frame
// carries one window id, and merging across them would deliver one pane's bytes
// to another's emulator.
func TestBrokerNeverCoalescesAcrossWindows(t *testing.T) {
	t.Parallel()

	b := newBroker(backlogBounds{}, nil)
	_, _ = b.subscribe()

	b.publish(outputEvent("@1", "a"))
	b.publish(outputEvent("@2", "b"))
	b.publish(outputEvent("@1", "c"))
	b.publish(outputEvent("@2", "d"))

	replacement, unsubscribe := b.subscribe()
	defer unsubscribe()

	perWindow := map[string]string{}
	for range 4 {
		out := requireOutput(t, receive(t, replacement))
		perWindow[out.WindowID] += string(out.Data)
	}
	require.Equal(t, map[string]string{"@1": "ac", "@2": "bd"}, perWindow)
}
