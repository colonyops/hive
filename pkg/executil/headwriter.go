package executil

import (
	"bytes"
	"sync"
)

// HeadWriter keeps the first Max bytes written to it and discards the rest.
// Write always reports the full length, so a noisy child never blocks on a
// full pipe and io.Copy never fails with io.ErrShortWrite. It is safe for
// concurrent use, so one HeadWriter can take both stdout and stderr.
type HeadWriter struct {
	Max int

	mu        sync.Mutex
	buf       []byte
	truncated bool
}

func (w *HeadWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	keep := min(max(w.Max-len(w.buf), 0), len(p))
	w.buf = append(w.buf, p[:keep]...)
	if keep < len(p) {
		w.truncated = true
	}
	return len(p), nil
}

// Bytes returns a copy of the bytes kept so far.
func (w *HeadWriter) Bytes() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	return bytes.Clone(w.buf)
}

func (w *HeadWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return string(w.buf)
}

// Truncated reports whether any written byte was discarded.
func (w *HeadWriter) Truncated() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.truncated
}
