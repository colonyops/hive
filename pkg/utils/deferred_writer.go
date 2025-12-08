package utils

import (
	"bytes"
	"io"
	"sync"
)

// DeferredWriter buffers all writes in memory until Flush is called.
// Safe for concurrent use.
type DeferredWriter struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

// Write stores data in the internal buffer.
func (d *DeferredWriter) Write(p []byte) (n int, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.buf.Write(p)
}

// Flush writes all buffered data to w and clears the buffer.
func (d *DeferredWriter) Flush(w io.Writer) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.buf.Len() == 0 {
		return nil
	}

	_, err := d.buf.WriteTo(w)
	return err
}
