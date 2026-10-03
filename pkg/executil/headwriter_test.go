package executil

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHeadWriterKeepsTheHeadAndDrainsTheRest(t *testing.T) {
	w := &HeadWriter{Max: 8}

	n, err := w.Write([]byte("12345"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.False(t, w.Truncated())

	n, err = w.Write([]byte("67890abc"))
	require.NoError(t, err)
	assert.Equal(t, 8, n, "a write past the cap still reports its full length")
	assert.Equal(t, "12345678", string(w.Bytes()))
	assert.True(t, w.Truncated())

	n, err = w.Write([]byte("more"))
	require.NoError(t, err)
	assert.Equal(t, 4, n)
	assert.Equal(t, "12345678", w.String())
}

func TestHeadWriterExactFitIsNotTruncated(t *testing.T) {
	w := &HeadWriter{Max: 4}
	_, _ = w.Write([]byte("abcd"))
	_, _ = w.Write(nil)
	assert.False(t, w.Truncated())
	assert.Equal(t, "abcd", w.String())
}

func TestHeadWriterBytesIsACopy(t *testing.T) {
	w := &HeadWriter{Max: 8}
	_, _ = w.Write([]byte("abc"))
	b := w.Bytes()
	b[0] = 'x'
	assert.Equal(t, "abc", w.String())
}

func TestHeadWriterConcurrentWrites(t *testing.T) {
	const writers, writes, max = 8, 200, 1000
	w := &HeadWriter{Max: max}

	var wg sync.WaitGroup
	var shortWrites atomic.Int64
	for range writers {
		wg.Go(func() {
			for range writes {
				if n, err := w.Write([]byte("xy")); err != nil || n != 2 {
					shortWrites.Add(1)
				}
			}
		})
	}
	wg.Wait()

	assert.Zero(t, shortWrites.Load(), "every write reports its full length")

	assert.Equal(t, strings.Repeat("xy", max/2), w.String(), "writes never interleave inside one call")
	assert.True(t, w.Truncated())
}
