package tui

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelWriter_Write(t *testing.T) {
	t.Run("sends data to channel", func(t *testing.T) {
		ch := make(chan string, 1)
		ctx := context.Background()
		w := &channelWriter{ch: ch, ctx: ctx}

		n, err := w.Write([]byte("hello"))
		require.NoError(t, err)
		assert.Equal(t, 5, n)

		select {
		case msg := <-ch:
			assert.Equal(t, "hello", msg)
		case <-time.After(100 * time.Millisecond):
			t.Fatal("timeout waiting for message")
		}
	})

	t.Run("returns error on cancelled context", func(t *testing.T) {
		ch := make(chan string) // unbuffered, will block
		ctx, cancel := context.WithCancel(context.Background())
		w := &channelWriter{ch: ch, ctx: ctx}

		// Cancel immediately so write can't succeed
		cancel()

		_, err := w.Write([]byte("hello"))
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("returns error on timeout context", func(t *testing.T) {
		ch := make(chan string) // unbuffered, will block
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		w := &channelWriter{ch: ch, ctx: ctx}

		_, err := w.Write([]byte("hello"))
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("handles multiple writes", func(t *testing.T) {
		ch := make(chan string, 3)
		ctx := context.Background()
		w := &channelWriter{ch: ch, ctx: ctx}

		_, _ = w.Write([]byte("one"))
		_, _ = w.Write([]byte("two"))
		_, _ = w.Write([]byte("three"))

		assert.Equal(t, "one", <-ch)
		assert.Equal(t, "two", <-ch)
		assert.Equal(t, "three", <-ch)
	})
}
