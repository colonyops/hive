package usageanalytics

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	domain "github.com/colonyops/hive/internal/domain/usageanalytics"
	store "github.com/colonyops/hive/internal/store/usageanalytics"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

type testSink struct {
	mu       sync.Mutex
	events   []domain.Captured
	attempts int
	failures int
	block    bool
	closed   bool
	prunes   int
}

func (s *testSink) WriteBatch(ctx context.Context, batch []domain.Captured) error {
	if s.block {
		<-ctx.Done()
		return ctx.Err()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attempts++
	if s.failures > 0 {
		s.failures--
		return errors.New("busy")
	}
	s.events = append(s.events, batch...)
	return nil
}
func (s *testSink) Prune(context.Context, time.Time) error { s.prunes++; return nil }
func (s *testSink) Close() error                           { s.closed = true; return nil }

func TestDisabledDoesNotOpenHistory(t *testing.T) {
	dir := t.TempDir()
	s := New(t.Context(), zerolog.Nop(), Options{DataDir: dir})
	s.Record(t.Context(), domain.TerminalStarted(false))
	s.Close()
	require.False(t, s.Active())
	_, err := os.Stat(filepath.Join(dir, store.Filename))
	require.True(t, os.IsNotExist(err))
}

func TestWorkerPeriodicSizeRetryAndShutdown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sink := &testSink{failures: 2}
		s := newService(t.Context(), zerolog.Nop(), Options{Surface: "desktop"}, sink)
		s.Record(t.Context(), domain.TerminalStarted(false))
		time.Sleep(11 * time.Second)
		synctest.Wait()
		require.Len(t, sink.events, 1)
		require.Equal(t, 3, sink.attempts)
		for range 32 {
			s.Record(t.Context(), domain.TerminalStarted(true))
		}
		synctest.Wait()
		require.Len(t, sink.events, 33)
		s.Record(t.Context(), domain.SessionCreated("full", false))
		s.Close()
		require.Len(t, sink.events, 34)
		require.True(t, sink.closed)
		require.Equal(t, 1, sink.prunes)
		s.Close()
		s.Record(t.Context(), domain.TerminalStarted(false))
		require.Len(t, sink.events, 34)
	})
}

func TestPressureCancellationAndBoundedShutdown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sink := &testSink{block: true}
		s := newService(t.Context(), zerolog.Nop(), Options{Surface: "desktop"}, sink)
		for range 32 {
			s.Record(t.Context(), domain.TerminalStarted(false))
		}
		synctest.Wait()
		for range 128 {
			s.Record(t.Context(), domain.TerminalStarted(false))
		}
		started := time.Now()
		s.Record(t.Context(), domain.TerminalStarted(false))
		require.Equal(t, 25*time.Millisecond, time.Since(started))
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		s.Record(ctx, domain.TerminalStarted(false))
		require.Len(t, s.queue, 128)
		started = time.Now()
		s.Close()
		require.LessOrEqual(t, time.Since(started), 2*time.Second)
		require.True(t, sink.closed)
	})
}

func TestRetryCapAndDiscardQueuedEvents(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sink := &testSink{failures: 10}
		s := newService(t.Context(), zerolog.Nop(), Options{Surface: "desktop"}, sink)
		s.Record(t.Context(), domain.TerminalStarted(false))
		time.Sleep(11 * time.Second)
		synctest.Wait()
		require.Equal(t, 3, sink.attempts)
		require.Empty(t, sink.events)
		s.Record(t.Context(), domain.TerminalStarted(false))
		time.Sleep(time.Nanosecond)
		s.DiscardBefore(time.Now())
		s.Close()
		require.Empty(t, sink.events)
	})
}
