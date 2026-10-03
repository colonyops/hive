package tmuxcc

import (
	"sync"
	"time"
)

// paintGate holds live output per pane until that pane's snapshot resolves,
// then replays it behind the snapshot. Emitting under the same lock the reader
// appends under is what keeps the two in order.
//
// Before the attach sequence completes every pane is held; afterwards only a
// pane being snapshotted is, which is how a window created later gets a first
// paint without stalling the rest of the session.
type paintGate struct {
	mu   sync.Mutex
	emit func(pane string, data []byte, at time.Time)
	open bool
	live map[string]bool
	held map[string]bool
	// pending distinguishes panes still owed a first paint from panes temporarily
	// reheld for repaint.
	pending map[string]bool
	buf     map[string][]byte
	marks   map[string]int
}

func newPaintGate(emit func(pane string, data []byte, at time.Time)) *paintGate {
	return &paintGate{
		emit:    emit,
		live:    map[string]bool{},
		held:    map[string]bool{},
		pending: map[string]bool{},
		buf:     map[string][]byte{},
		marks:   map[string]int{},
	}
}

func (g *paintGate) route(pane string, data []byte, at time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.held[pane] || (!g.open && !g.live[pane]) {
		g.buf[pane] = append(g.buf[pane], data...)
		return
	}
	g.emit(pane, data, at)
}

// hold reports whether a pane is still owed its first paint, including when
// the reader already took the gate. Bytes before mark belong to the snapshot.
func (g *paintGate) hold(pane string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.live[pane] {
		return false
	}
	if g.held[pane] {
		return g.pending[pane]
	}
	g.held[pane] = true
	g.pending[pane] = true
	return true
}

// rehold buffers a pane whether or not it has been painted before. It is what a
// deferred first paint needs and hold cannot give: a repaint's windows are all
// live from the attach that preceded it, so hold would decline them and their
// output would stream ahead of the snapshot that has to precede it.
func (g *paintGate) rehold(pane string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.held[pane] = true
}

// mark holds the pane and records the point the snapshot command was sent at:
// output buffered before it is inside the snapshot, output after it is replayed
// behind the snapshot.
func (g *paintGate) mark(pane string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.held[pane] = true
	delete(g.pending, pane)
	g.marks[pane] = len(g.buf[pane])
}

func (g *paintGate) release(pane string, painted []byte) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.held[pane] {
		// discard ran while the snapshot was in flight: the window closed and
		// there is nothing left to paint onto.
		return
	}
	now := time.Now()
	if len(painted) > 0 {
		g.emit(pane, painted, now)
	}
	if tail := g.buf[pane][g.marks[pane]:]; len(tail) > 0 {
		g.emit(pane, tail, now)
	}
	delete(g.buf, pane)
	delete(g.marks, pane)
	delete(g.held, pane)
	delete(g.pending, pane)
	g.live[pane] = true
}

// discard forgets a pane entirely, dropping whatever it was holding. A closed
// window has no tab to render onto, and tmux never reuses a pane id, so
// remembering it would only grow the gate for the life of the client.
func (g *paintGate) discard(pane string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.buf, pane)
	delete(g.marks, pane)
	delete(g.held, pane)
	delete(g.pending, pane)
	delete(g.live, pane)
}

// openAll opens the gate for every pane that is not mid-snapshot. A held pane
// keeps its buffer and its mark: its snapshot has not landed yet, and flushing
// what it is holding would put the pane's own scrollback on the stream *behind*
// the live output that followed it. That is the case a deferred first paint
// makes ordinary — the gate opens while the background windows are still being
// captured.
func (g *paintGate) openAll() {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	for pane, data := range g.buf {
		if g.held[pane] {
			continue
		}
		if len(data) > 0 {
			g.emit(pane, data, now)
		}
		delete(g.buf, pane)
		delete(g.marks, pane)
	}
	g.open = true
}
