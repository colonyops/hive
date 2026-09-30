package ptyterm

import "sync"

// broker fans one terminal's events out to at most one subscriber.
//
// A slow subscriber is answered by coalescing rather than by dropping or
// pausing: consecutive Output is concatenated in place, so the queue stops
// growing while the bytes stay whole and in order. That is only sound because
// this process owns the byte stream end to end — there is no frame boundary in
// a PTY read that means anything to the far end.
type broker struct {
	mu       sync.Mutex
	sub      *subscriber
	maxBytes int
}

type subscriber struct {
	ch     chan Event
	queue  []Event
	bytes  int
	wake   chan struct{}
	closed bool
	done   chan struct{}
}

func newBroker(maxBytes int) *broker {
	return &broker{maxBytes: maxBytes}
}

// Subscribe replaces any existing subscriber, closing its channel — that is how
// a transport learns it was superseded.
func (b *broker) Subscribe() (<-chan Event, func()) {
	b.mu.Lock()
	if b.sub != nil {
		b.closeLocked(b.sub)
	}
	sub := &subscriber{
		ch:   make(chan Event, 64),
		wake: make(chan struct{}, 1),
		done: make(chan struct{}),
	}
	b.sub = sub
	b.mu.Unlock()

	go b.pump(sub)
	return sub.ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if b.sub == sub {
			b.sub = nil
		}
		b.closeLocked(sub)
	}
}

// Publish queues an event for the subscriber. It never blocks: the PTY reader
// calls it, and a reader that stalls stalls the child process behind it.
func (b *broker) Publish(ev Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	sub := b.sub
	if sub == nil || sub.closed {
		return
	}

	if out, ok := ev.(Output); ok {
		if n := len(sub.queue); n > 0 {
			if last, isOutput := sub.queue[n-1].(Output); isOutput {
				last.Data = append(last.Data, out.Data...)
				sub.queue[n-1] = last
				sub.bytes += len(out.Data)
				b.signal(sub)
				return
			}
		}
		// Copy: the reader owns its scratch buffer and reuses it on the next read.
		out.Data = append([]byte(nil), out.Data...)
		ev = out
		sub.bytes += len(out.Data)
	}

	sub.queue = append(sub.queue, ev)
	if sub.bytes > b.maxBytes {
		// Coalescing bounds the queue's length, not its bytes: a child that
		// outruns the transport for long enough still has to be cut off, and a
		// re-attach replays the ring, which is a complete resync rather than a
		// partial one.
		b.closeLocked(sub)
		if b.sub == sub {
			b.sub = nil
		}
		return
	}
	b.signal(sub)
}

func (b *broker) signal(sub *subscriber) {
	select {
	case sub.wake <- struct{}{}:
	default:
	}
}

func (b *broker) closeLocked(sub *subscriber) {
	if sub.closed {
		return
	}
	sub.closed = true
	close(sub.done)
}

// pump moves queued events onto the subscriber's channel. It is the only
// goroutine that writes to ch, so closing it here needs no coordination.
func (b *broker) pump(sub *subscriber) {
	defer close(sub.ch)
	for {
		b.mu.Lock()
		queue := sub.queue
		sub.queue = nil
		sub.bytes = 0
		closed := sub.closed
		b.mu.Unlock()

		for _, ev := range queue {
			select {
			case sub.ch <- ev:
			case <-sub.done:
				return
			}
		}
		if closed {
			return
		}
		select {
		case <-sub.wake:
		case <-sub.done:
			// Drain what was queued before the close so an exit notice is not
			// lost to the race between publishing it and tearing down.
			b.mu.Lock()
			queue := sub.queue
			sub.queue = nil
			b.mu.Unlock()
			for _, ev := range queue {
				select {
				case sub.ch <- ev:
				default:
				}
			}
			return
		}
	}
}
