package herdrcore

import (
	"encoding/json"
	"io"
	"sync"
	"time"
)

var timeZero time.Time

// Stream is one attach channel. Read returns the terminal's output and
// credits the core for what was read, so a slow reader gets a repaint
// rather than an unbounded backlog. A stalled Stream never blocks the
// client's other calls or streams.
type Stream struct {
	c              *Client
	channel        uint32
	automaticInput bool

	mu     sync.Mutex
	cond   *sync.Cond
	queue  [][]byte
	closed bool
	reason *CloseReason
	err    error
}

func newStream(c *Client, channel uint32, automaticInput bool) *Stream {
	s := &Stream{c: c, channel: channel, automaticInput: automaticInput}
	s.cond = sync.NewCond(&s.mu)
	return s
}

func (s *Stream) deliver(f frame) {
	switch f.kind {
	case KindData:
		s.mu.Lock()
		s.queue = append(s.queue, f.payload)
		s.mu.Unlock()
		s.cond.Broadcast()
	case KindClose:
		var r CloseReason
		var reason *CloseReason
		if len(f.payload) > 0 && json.Unmarshal(f.payload, &r) == nil {
			reason = &r
		}
		s.c.forget(s.channel)
		s.end(reason, io.EOF)
	}
}

func (s *Stream) end(reason *CloseReason, err error) {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		s.reason = reason
		s.err = err
	}
	s.mu.Unlock()
	s.cond.Broadcast()
}

// Read returns output bytes; io.EOF once the stream has ended and drained.
func (s *Stream) Read(p []byte) (int, error) {
	s.mu.Lock()
	for len(s.queue) == 0 && !s.closed {
		s.cond.Wait()
	}
	if len(s.queue) == 0 {
		err := s.err
		s.mu.Unlock()
		if err == nil {
			err = io.EOF
		}
		return 0, err
	}
	n := copy(p, s.queue[0])
	if n == len(s.queue[0]) {
		s.queue[0] = nil
		s.queue = s.queue[1:]
	} else {
		s.queue[0] = s.queue[0][n:]
	}
	s.mu.Unlock()
	_ = s.c.send(s.channel, KindCredit, creditPayload(n))
	return n, nil
}

// Write sends typed input to the program. It marks this viewer active.
func (s *Stream) Write(p []byte) (int, error) {
	if err := s.sendChunks(KindData, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

// WriteAutomatic sends terminal-generated replies (query answers), which do
// not mark the viewer active. On a path without AUTOMATIC_INPUT support it
// falls back to plain input, as the protocol requires.
func (s *Stream) WriteAutomatic(p []byte) error {
	if !s.automaticInput {
		return s.sendChunks(KindData, p)
	}
	return s.sendChunks(KindAutomaticInput, p)
}

func (s *Stream) sendChunks(kind byte, p []byte) error {
	const chunk = 1 << 20
	for len(p) > 0 {
		n := min(len(p), chunk)
		if err := s.c.send(s.channel, kind, p[:n]); err != nil {
			return err
		}
		p = p[n:]
	}
	return nil
}

// Resize sets the viewer's size in cells.
func (s *Stream) Resize(cols, rows uint16) error {
	return s.c.send(s.channel, KindResize, sizePayload(cols, rows))
}

// Focus reports that the viewer gained or lost focus.
func (s *Stream) Focus(focused bool) error {
	b := byte(0)
	if focused {
		b = 1
	}
	return s.c.send(s.channel, KindFocus, []byte{b})
}

// Facts reports the viewer's colors and cell size.
func (s *Stream) Facts(d Display) error {
	body, err := json.Marshal(d)
	if err != nil {
		return err
	}
	return s.c.send(s.channel, KindFacts, body)
}

// Close detaches the viewer. The program keeps running.
func (s *Stream) Close() error {
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return nil
	}
	err := s.c.send(s.channel, KindClose, nil)
	s.c.forget(s.channel)
	s.end(nil, io.EOF)
	return err
}

// Ended is the core's reason for ending the stream, or nil when the viewer
// detached or the connection dropped.
func (s *Stream) Ended() *CloseReason {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reason
}
