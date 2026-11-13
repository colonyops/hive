package executil

import (
	"bytes"
	"sync"
)

// TailWriter keeps the last Max bytes written to it and discards the rest.
// Write always reports the full length, so a noisy child never blocks on a
// full pipe and io.Copy never fails with io.ErrShortWrite. It is safe for
// concurrent use.
type TailWriter struct {
	Max int

	mu        sync.Mutex
	buf       []byte
	truncated bool
}

func (w *TailWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.Max <= 0 {
		return len(p), nil
	}
	if len(p) >= w.Max {
		w.buf = append(w.buf[:0], p[len(p)-w.Max:]...)
		w.truncated = true
		return len(p), nil
	}
	overflow := len(w.buf) + len(p) - w.Max
	if overflow > 0 {
		w.buf = append(w.buf[overflow:], p...)
		w.truncated = true
	} else {
		w.buf = append(w.buf, p...)
	}
	return len(p), nil
}

// Bytes returns a copy of the bytes kept so far.
func (w *TailWriter) Bytes() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	return bytes.Clone(w.buf)
}

func (w *TailWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return string(w.buf)
}

// Truncated reports whether any written byte was discarded.
func (w *TailWriter) Truncated() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.truncated
}
