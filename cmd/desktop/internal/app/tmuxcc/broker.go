package tmuxcc

import (
	"sync"
	"time"
)

// defaultBufferBytes bounds the per-session backlog of undelivered output.
const defaultBufferBytes = 8 << 20

// defaultBufferEvents bounds the same backlog by count, because only Output is
// charged bytes: a low-output session whose windows rename or change active pane
// in a loop, left with no subscriber, would otherwise grow it without limit and
// trip nothing. An event costs more than its payload — the boxed value, the
// struct, the slice slot — so 64k of them is the same order of memory as the
// byte bound, not the zero they measure.
const defaultBufferEvents = 1 << 16

// backlogBounds is what the backlog refuses to grow past. Both are checked; a
// zero field takes its default.
type backlogBounds struct {
	bytes  int
	events int
}

// subscriberQueue is zero deliberately: an event leaves the accounted backlog
// only once a subscriber has taken it, so replacing a subscriber cannot strand
// events in the channel it is closing. A re-Subscribe — the WebSocket reconnect
// path — must never hand the emulator a stream with a hole in it.
const subscriberQueue = 0

// finalDelivery bounds how long a closing broker offers the last lifecycle
// event. It is a backstop, not the working bound: a transport that gives up
// unsubscribes, which releases the pump at once. What it has to cover is a
// subscriber that is alive but parked inside one congested socket write, which
// is precisely the subscriber overflow produces — 250ms was shorter than such a
// write, so the stream that most needed to say why it ended was the one that
// could not.
const finalDelivery = 2 * time.Second

// resyncCooldown is how long a resync has to hold for the next one to be worth
// attempting. Refilling the whole bound inside it means the subscriber is not
// draining at all, and each resync costs a `capture-pane` per window while the
// paint gate buffers the flood that is still arriving — so retrying on that
// cadence burns CPU to re-lose the same bytes. Past this the stream ends the
// way overflow always ended it.
const resyncCooldown = 10 * time.Second

type subscription struct {
	gen  uint64
	ch   chan Event
	stop chan struct{}
}

// broker fans one client's event stream out to at most one subscriber. The
// reader goroutine calls publish, which never blocks: events queue in a bounded
// backlog that a pump goroutine drains into the subscriber's channel. With no
// subscriber the backlog is what replays first-paint to a WebSocket that
// connects after Attach.
//
// A terminal byte stream cannot drop-oldest without corrupting the emulator, so
// exceeding the bound stops the backlog dead: onOverflow drops everything until
// resync clears it and a repaint re-establishes what the emulator should be
// showing. A resync the broker will not grant ends the stream instead.
type broker struct {
	mu        sync.Mutex
	cond      *sync.Cond
	buf       []Event
	bytes     int
	maxBytes  int
	maxEvents int
	sub       *subscription
	gen       uint64
	// epoch counts backlog clears. A pump reads it alongside the head it is
	// delivering, because a resync drops that head under it and leaves the
	// subscriber in place — see advance.
	epoch  uint64
	closed bool

	overflow bool
	// dropped is what overflow has cost so far: the tripping event plus
	// everything refused behind it. Reported once, to the log, when the resync
	// that clears it runs.
	dropped        int
	lastResync     time.Time
	resyncCooldown time.Duration

	// done releases a pump parked on a subscriber that stopped reading:
	// sync.Cond cannot wake a goroutine blocked on a channel send.
	done chan struct{}

	onOverflow func()
}

func newBroker(bounds backlogBounds, onOverflow func()) *broker {
	if bounds.bytes <= 0 {
		bounds.bytes = defaultBufferBytes
	}
	if bounds.events <= 0 {
		bounds.events = defaultBufferEvents
	}
	b := &broker{
		maxBytes:       bounds.bytes,
		maxEvents:      bounds.events,
		resyncCooldown: resyncCooldown,
		onOverflow:     onOverflow,
		done:           make(chan struct{}),
	}
	b.cond = sync.NewCond(&b.mu)
	return b
}

// publish appends ev to the backlog. Only a droppable event is bounded, and the
// exemption governs both sides of the bound: an event that can trip overflow is
// also an event the overflowed broker discards, so a bounded lifecycle event
// would become the trigger and then be thrown away — ending the stream with
// nothing saying why.
func (b *broker) publish(ev Event) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	size := eventBytes(ev)
	if droppable(ev) {
		if b.overflow {
			b.dropped += size
			b.mu.Unlock()
			return
		}
		if b.bytes+size > b.maxBytes || len(b.buf) >= b.maxEvents {
			b.overflow = true
			b.dropped = size
			// The flag is the guard the callback needs: only resync clears it,
			// so a second report cannot fire until a recovery has run.
			overflowed := b.onOverflow
			b.mu.Unlock()
			if overflowed != nil {
				go overflowed()
			}
			return
		}
	}
	if b.coalesceLocked(ev, size) {
		b.mu.Unlock()
		b.cond.Broadcast()
		return
	}
	b.buf = append(b.buf, ev)
	b.bytes += size
	b.mu.Unlock()
	b.cond.Broadcast()
}

// maxCoalescedBytes bounds one merged frame. tmux caps a %output notification
// at 2KB, so a busy pane produces thousands of them a second and every
// per-frame cost downstream — a WebSocket frame, a webview message task, a
// decode — is paid that many times. Merging them while they are queued anyway
// divides all of it at once.
const maxCoalescedBytes = 64 << 10

// coalesceLocked folds ev into the last queued event when both are output for
// the same pane, reporting whether it did. It merges only into an event behind
// the head, because the head is the one pump may already be handing to the
// subscriber — it holds its own copy and charges the backlog for the size it
// read, so growing it there would both lose the appended bytes and corrupt the
// byte accounting.
//
// Nothing here adds latency: a merge is only possible when a second event is
// already waiting, which means the subscriber is behind. A consumer keeping up
// sees a backlog of at most one and every event whole.
func (b *broker) coalesceLocked(ev Event, size int) bool {
	out, isOutput := ev.(Output)
	if !isOutput || len(b.buf) < 2 {
		return false
	}
	last := len(b.buf) - 1
	tail, isOutput := b.buf[last].(Output)
	if !isOutput || tail.WindowID != out.WindowID || tail.PaneID != out.PaneID {
		return false
	}
	if len(tail.Data)+size > maxCoalescedBytes {
		return false
	}
	// A fresh buffer rather than append onto tail.Data: that slice can alias
	// one the paint gate or the decoder still owns, and appending into its
	// spare capacity would write through to the other holder.
	merged := make([]byte, 0, len(tail.Data)+size)
	merged = append(merged, tail.Data...)
	merged = append(merged, out.Data...)
	// tail.At is kept, not out.At: it stamps the oldest byte in the frame, so
	// the latency the adapter reports stays the worst case rather than being
	// reset by every merge.
	tail.Data = merged
	b.buf[last] = tail
	b.bytes += size
	return true
}

// droppable reports whether losing ev is recoverable. Overflow is answered by a
// repaint, which puts every screen back and re-lists the windows; nothing
// replays the lifecycle event that says the stream degraded — or, when the
// resync is refused, why it ended.
func droppable(ev Event) bool {
	_, lifecycle := ev.(LifecycleChanged)
	return !lifecycle
}

// depth reports the buffered output bytes awaiting delivery.
func (b *broker) depth() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.bytes
}

// subscribe replaces any existing subscriber, closing its channel, and returns
// a channel that starts with the buffered backlog. The unsubscribe func is
// generation-scoped: a stale one cannot detach its replacement.
func (b *broker) subscribe() (<-chan Event, func()) {
	b.mu.Lock()
	b.gen++
	gen := b.gen
	if b.sub != nil {
		close(b.sub.stop)
	}
	sub := &subscription{gen: gen, ch: make(chan Event, subscriberQueue), stop: make(chan struct{})}
	b.sub = sub
	b.mu.Unlock()
	b.cond.Broadcast()

	go b.pump(sub)
	return sub.ch, func() { b.unsubscribe(gen) }
}

func (b *broker) unsubscribe(gen uint64) {
	b.mu.Lock()
	if b.sub != nil && b.sub.gen == gen {
		close(b.sub.stop)
		b.sub = nil
		b.gen++
	}
	b.mu.Unlock()
	b.cond.Broadcast()
}

// reset releases the current subscriber and drops the undelivered backlog. It
// is what a repaint runs first: the snapshot it is about to publish supersedes
// every byte the backlog holds, and a subscriber still draining that backlog
// would consume the snapshot instead of the one that asked for it. A closing
// broker is left alone — its backlog is the last thing a live subscriber will
// ever read.
func (b *broker) reset() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	if b.sub != nil {
		close(b.sub.stop)
		b.sub = nil
		b.gen++
	}
	b.clearLocked()
	b.mu.Unlock()
	b.cond.Broadcast()
}

// resync drops the backlog and lifts the overflow state without touching the
// subscriber, and reports the bytes that never reached it. It is what makes
// overflow recoverable: the repaint behind it publishes onto the same stream,
// so the emulator that was reading is the one the snapshot is drawn for —
// unlike reset, whose whole job is handing the snapshot to a replacement.
//
// A false ok is the broker declining to try again. Either it is closing, or the
// last resync did not survive resyncCooldown — the flood is outrunning the
// subscriber and another repaint would buy seconds, so the caller ends the
// stream instead.
func (b *broker) resync(now time.Time) (int, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return 0, false
	}
	if !b.lastResync.IsZero() && now.Sub(b.lastResync) < b.resyncCooldown {
		return 0, false
	}
	b.lastResync = now
	dropped := b.dropped + b.clearLocked()
	b.dropped = 0
	b.overflow = false
	b.cond.Broadcast()
	return dropped, true
}

// clearLocked drops the undelivered backlog and reports the bytes that went
// with it. The epoch moves because a pump may be parked on the head this
// removes, and it has to tell "resync dropped my event" from "the backlog is
// empty because I am the one who emptied it".
func (b *broker) clearLocked() int {
	dropped := b.bytes
	b.buf = nil
	b.bytes = 0
	b.epoch++
	return dropped
}

// close drains what a live subscriber can still take and closes its channel.
// This is how a WebSocket write pump learns the client is gone.
func (b *broker) close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	close(b.done)
	b.mu.Unlock()
	b.cond.Broadcast()
}

// pump delivers the head of the backlog and only then removes it, so an event
// the subscriber never took stays accounted and replays to its replacement.
func (b *broker) pump(sub *subscription) {
	defer close(sub.ch)
	for {
		b.mu.Lock()
		for len(b.buf) == 0 && b.gen == sub.gen && !b.closed {
			b.cond.Wait()
		}
		if b.gen != sub.gen || len(b.buf) == 0 {
			b.mu.Unlock()
			return
		}
		ev := b.buf[0]
		epoch := b.epoch
		closing := b.closed
		b.mu.Unlock()

		switch {
		case closing && droppable(ev):
			// The re-attach's first paint and window listing produce all of this
			// again, so a closing broker never parks on it — and never spends the
			// final-delivery budget the lifecycle event behind it needs.
			select {
			case sub.ch <- ev:
			default:
			}
		case closing:
			// The lifecycle event that says *why* the stream ended has no such
			// replacement — overflow is exactly the case where the subscriber
			// is behind — so it waits, bounded.
			if !b.sendFinal(sub, ev) {
				return
			}
		default:
			select {
			case sub.ch <- ev:
			case <-sub.stop:
				return
			case <-b.done:
				// Left in the backlog on purpose: the next pass re-reads it
				// under the closing rules above.
				continue
			}
		}
		if !b.advance(sub.gen, epoch, ev) {
			return
		}
	}
}

// advance drops the delivered head. A generation change means a replacement
// pump owns the backlog now, and this one must not consume from it. An epoch
// change means a resync already dropped the head — the accounting went with it,
// and the subscriber is staying, so this pump carries on.
func (b *broker) advance(gen, epoch uint64, ev Event) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.gen != gen {
		return false
	}
	if b.epoch != epoch {
		return true
	}
	if len(b.buf) == 0 {
		return false
	}
	b.buf[0] = nil
	b.buf = b.buf[1:]
	b.bytes -= eventBytes(ev)
	return true
}

func (b *broker) sendFinal(sub *subscription, ev Event) bool {
	timer := time.NewTimer(finalDelivery)
	defer timer.Stop()
	select {
	case sub.ch <- ev:
		return true
	case <-sub.stop:
		return false
	case <-timer.C:
		return false
	}
}

func eventBytes(ev Event) int {
	if o, ok := ev.(Output); ok {
		return len(o.Data)
	}
	return 0
}
