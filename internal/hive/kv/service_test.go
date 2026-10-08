package kv_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"

	"github.com/colonyops/hive/internal/domain/kv"
	kvsvc "github.com/colonyops/hive/internal/hive/kv"
)

type countingStore struct {
	kv.KV
	sweeps atomic.Int32
	err    error
}

func (s *countingStore) SweepExpired(context.Context) error {
	s.sweeps.Add(1)
	return s.err
}

func TestSweepRunsEveryIntervalUntilCanceled(t *testing.T) {
	store := &countingStore{err: errors.New("database is locked")}
	svc := kvsvc.NewService(zerolog.Nop(), store)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		svc.Sweep(ctx, time.Millisecond)
		close(done)
	}()

	assert.Eventually(t, func() bool { return store.sweeps.Load() >= 2 }, time.Second, time.Millisecond,
		"a failed sweep does not stop the next one")
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Sweep did not return after its context was canceled")
	}
}
