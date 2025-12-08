package utils

import (
	"bytes"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeferredWriter_Write(t *testing.T) {
	t.Run("buffers writes", func(t *testing.T) {
		d := &DeferredWriter{}

		n, err := d.Write([]byte("hello "))
		require.NoError(t, err)
		assert.Equal(t, 6, n)

		n, err = d.Write([]byte("world"))
		require.NoError(t, err)
		assert.Equal(t, 5, n)

		// Verify content by flushing
		var out bytes.Buffer
		err = d.Flush(&out)
		require.NoError(t, err)
		assert.Equal(t, "hello world", out.String())
	})

	t.Run("concurrent writes are safe", func(t *testing.T) {
		d := &DeferredWriter{}
		var wg sync.WaitGroup

		// Write concurrently
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _ = d.Write([]byte("x"))
			}()
		}

		wg.Wait()

		var out bytes.Buffer
		err := d.Flush(&out)
		require.NoError(t, err)
		assert.Len(t, out.String(), 100)
	})
}

func TestDeferredWriter_Flush(t *testing.T) {
	t.Run("writes to destination", func(t *testing.T) {
		d := &DeferredWriter{}
		_, _ = d.Write([]byte("test data"))

		var out bytes.Buffer
		err := d.Flush(&out)
		require.NoError(t, err)
		assert.Equal(t, "test data", out.String())
	})

	t.Run("clears buffer after flush", func(t *testing.T) {
		d := &DeferredWriter{}
		_, _ = d.Write([]byte("test data"))

		var out1 bytes.Buffer
		err := d.Flush(&out1)
		require.NoError(t, err)
		assert.Equal(t, "test data", out1.String())

		// Second flush should be empty
		var out2 bytes.Buffer
		err = d.Flush(&out2)
		require.NoError(t, err)
		assert.Empty(t, out2.String())
	})

	t.Run("empty buffer returns nil", func(t *testing.T) {
		d := &DeferredWriter{}

		var out bytes.Buffer
		err := d.Flush(&out)
		require.NoError(t, err)
		assert.Empty(t, out.String())
	})
}
