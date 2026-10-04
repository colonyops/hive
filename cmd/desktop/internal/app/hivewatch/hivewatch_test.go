package hivewatch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

type change struct{ prev, next int }

type fakeSource struct {
	mu      sync.Mutex
	value   int
	err     error
	reads   int
	changes []change
}

func (f *fakeSource) set(value int, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.value, f.err = value, err
}

func (f *fakeSource) readCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reads
}

func (f *fakeSource) seen() []change {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]change(nil), f.changes...)
}

func (f *fakeSource) probe() Probe {
	return NewProbe(Spec[int]{
		Name: "fake",
		Read: func(context.Context) (int, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.reads++
			return f.value, f.err
		},
		Equal: func(a, b int) bool { return a == b },
		OnChange: func(_ context.Context, prev, next int) {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.changes = append(f.changes, change{prev, next})
		},
	})
}

func TestTick_FirstReadIsTheBaseline(t *testing.T) {
	src := &fakeSource{value: 1}
	w := New(time.Hour, zerolog.Nop(), src.probe())

	w.Tick(t.Context())
	w.Tick(t.Context())

	require.Empty(t, src.seen())
}

func TestTick_ReportsPreviousAndNext(t *testing.T) {
	src := &fakeSource{value: 1}
	w := New(time.Hour, zerolog.Nop(), src.probe())
	w.Tick(t.Context())

	src.set(2, nil)
	w.Tick(t.Context())

	require.Equal(t, []change{{1, 2}}, src.seen())
}

func TestTick_FailedReadKeepsTheBaseline(t *testing.T) {
	src := &fakeSource{value: 1}
	w := New(time.Hour, zerolog.Nop(), src.probe())
	w.Tick(t.Context())

	src.set(0, errors.New("database is locked"))
	w.Tick(t.Context())
	require.Empty(t, src.seen())

	src.set(2, nil)
	w.Tick(t.Context())
	require.Equal(t, []change{{1, 2}}, src.seen())
}

func TestTick_RunsEveryProbe(t *testing.T) {
	a, b := &fakeSource{value: 1}, &fakeSource{value: 10}
	w := New(time.Hour, zerolog.Nop(), a.probe(), b.probe())
	w.Tick(t.Context())

	b.set(11, nil)
	w.Tick(t.Context())

	require.Empty(t, a.seen())
	require.Equal(t, []change{{10, 11}}, b.seen())
}

func TestStart_PrimesBeforeReturning(t *testing.T) {
	src := &fakeSource{value: 1}
	w := New(time.Hour, zerolog.Nop(), src.probe())

	w.Start(t.Context())
	t.Cleanup(w.Stop)

	require.Equal(t, 1, src.readCount())
}

func TestStart_LoopReportsAChange(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		src := &fakeSource{value: 1}
		w := New(time.Second, zerolog.Nop(), src.probe())
		w.Start(t.Context())
		t.Cleanup(w.Stop)

		src.set(2, nil)
		time.Sleep(time.Second)
		synctest.Wait()

		require.Equal(t, []change{{1, 2}}, src.seen())
	})
}

func TestStop_WaitsForTheLoop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		src := &fakeSource{}
		w := New(time.Second, zerolog.Nop(), src.probe())
		w.Start(t.Context())
		time.Sleep(time.Second)
		synctest.Wait()
		require.Equal(t, 2, src.readCount(), "the baseline and one tick")

		w.Stop()
		w.Stop()

		time.Sleep(time.Hour)
		synctest.Wait()
		require.Equal(t, 2, src.readCount(), "a read ran after Stop returned")
	})
}

func TestStop_BeforeStartIsANoop(t *testing.T) {
	New(time.Hour, zerolog.Nop()).Stop()
}

func TestTick_LogsAnOutageOnceAndItsRecovery(t *testing.T) {
	var buf bytes.Buffer
	src := &fakeSource{value: 1}
	w := New(time.Hour, zerolog.New(&buf).Level(zerolog.InfoLevel), src.probe())
	w.Tick(t.Context())

	src.set(0, errors.New("database is locked"))
	w.Tick(t.Context())
	w.Tick(t.Context())
	src.set(1, nil)
	w.Tick(t.Context())

	var msgs []string
	for line := range strings.Lines(buf.String()) {
		var entry struct{ Level, Message, Probe, Cmp string }
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		require.Equal(t, "fake", entry.Probe)
		require.Equal(t, "hivewatch", entry.Cmp)
		msgs = append(msgs, entry.Level+": "+entry.Message)
	}
	require.Equal(t, []string{
		"warn: probe read failing; updates paused",
		"info: probe read recovered",
	}, msgs)
}
