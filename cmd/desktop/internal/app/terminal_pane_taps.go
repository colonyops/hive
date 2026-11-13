package app

import (
	"context"
	"sync"
	"sync/atomic"

	tmuxcc "github.com/colonyops/hive/internal/platform/tmux/control"
)

// paneTapQueue bounds the output chunks waiting for a native renderer. Past
// it the tap drops output and asks for a repaint: the snapshot carries the
// pane's screen and scrollback, so nothing the user would read is lost.
const paneTapQueue = 1024

// PaneTap carries one pane's output to a renderer outside the webview. While
// it is open, that pane's output leaves the session stream: the webview has
// no emulator drawing it.
type PaneTap struct {
	C <-chan []byte

	ch       chan []byte
	overflow atomic.Bool
	closed   chan struct{}
	once     sync.Once
	release  func()
}

// Overflowed reports, once per overflow, that output was dropped and the
// renderer must start from a fresh repaint.
func (t *PaneTap) Overflowed() bool { return t.overflow.Swap(false) }

// Close stops the tap; the pane's output returns to the session stream.
func (t *PaneTap) Close() {
	t.once.Do(func() {
		t.release()
		close(t.closed)
	})
}

// Done is closed once the tap is closed.
func (t *PaneTap) Done() <-chan struct{} { return t.closed }

func (t *PaneTap) deliver(p []byte) {
	select {
	case t.ch <- p:
	default:
		t.overflow.Store(true)
	}
}

// paneTaps is the routing table: target, then pane id.
type paneTaps struct {
	mu   sync.Mutex
	taps map[string]map[string]*PaneTap
}

func (r *paneTaps) open(target, pane string) *PaneTap {
	ch := make(chan []byte, paneTapQueue)
	t := &PaneTap{C: ch, ch: ch, closed: make(chan struct{})}
	t.release = func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.taps[target][pane] == t {
			delete(r.taps[target], pane)
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.taps == nil {
		r.taps = map[string]map[string]*PaneTap{}
	}
	if r.taps[target] == nil {
		r.taps[target] = map[string]*PaneTap{}
	}
	if old := r.taps[target][pane]; old != nil {
		defer old.Close()
	}
	r.taps[target][pane] = t
	return t
}

func (r *paneTaps) lookup(target, pane string) *PaneTap {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.taps[target][pane]
}

// divert forwards events to the returned channel, except output from a
// tapped pane, which goes to its tap. It ends when events closes or stop is
// closed.
func (r *paneTaps) divert(target string, events <-chan tmuxcc.Event, stop <-chan struct{}) <-chan tmuxcc.Event {
	out := make(chan tmuxcc.Event)
	go func() {
		defer close(out)
		for ev := range events {
			if o, ok := ev.(tmuxcc.Output); ok {
				if tap := r.lookup(target, o.PaneID); tap != nil {
					tap.deliver(o.Data)
					continue
				}
			}
			select {
			case out <- ev:
			case <-stop:
				return
			}
		}
	}()
	return out
}

// TapPane routes target's pane to a renderer outside the webview and
// repaints the pane onto it. A second tap on the same pane replaces the first.
func (s *TerminalsService) TapPane(ctx context.Context, target, pane string) (*PaneTap, error) {
	tap := s.taps.open(target, pane)
	if err := s.RepaintPane(ctx, target, pane); err != nil {
		tap.Close()
		return nil, err
	}
	return tap, nil
}

// RepaintPane puts a fresh snapshot of one pane on its session's stream.
func (s *TerminalsService) RepaintPane(ctx context.Context, target, pane string) error {
	client, err := s.client(ctx, target)
	if err != nil {
		return err
	}
	return terminalError(client.RepaintPane(ctx, pane), "repainting pane %q", pane)
}
