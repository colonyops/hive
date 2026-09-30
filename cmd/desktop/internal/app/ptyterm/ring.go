package ptyterm

import "sync"

// ring is a terminal's replay buffer: the last max bytes the PTY produced, kept
// so a re-attach can restore the screen.
//
// It is the raw byte stream with its head cut off, not a rendered screen, so a
// replay can begin inside an escape sequence and re-runs alternate-screen
// transitions the pane already made. The trade is deliberate: it restores
// scrollback a re-render could not, and it costs one resident buffer per
// terminal.
type ring struct {
	mu   sync.Mutex
	buf  []byte
	max  int
	full bool
	head int
}

func newRing(max int) *ring {
	return &ring{buf: make([]byte, 0, min(max, 64<<10)), max: max}
}

func (r *ring) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(p) >= r.max {
		// The write alone overruns the ring: keep its tail and start over.
		r.buf = append(r.buf[:0], p[len(p)-r.max:]...)
		r.full = true
		r.head = 0
		return len(p), nil
	}
	if !r.full {
		r.buf = append(r.buf, p...)
		if len(r.buf) >= r.max {
			r.full = true
			r.head = len(r.buf) - r.max
			r.buf = r.buf[r.head:]
			r.head = 0
		}
		return len(p), nil
	}
	// Steady state: overwrite from the head, wrapping once.
	n := copy(r.buf[r.head:], p)
	if n < len(p) {
		copy(r.buf, p[n:])
	}
	r.head = (r.head + len(p)) % r.max
	return len(p), nil
}

// Snapshot returns the buffered bytes oldest-first.
func (r *ring) Snapshot() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.full {
		return append([]byte(nil), r.buf...)
	}
	out := make([]byte, 0, len(r.buf))
	out = append(out, r.buf[r.head:]...)
	return append(out, r.buf[:r.head]...)
}
