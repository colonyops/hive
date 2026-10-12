package relay_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/relay/internal/relay"
	"github.com/colonyops/hive/pkg/randid"
)

func putSweepSource(t *testing.T, store relay.Store, email, id string) {
	t.Helper()
	ctx := t.Context()
	if _, err := store.Accounts().Get(ctx, email); errors.Is(err, relay.ErrNotFound) {
		require.NoError(t, store.Accounts().Put(ctx, relay.Account{Email: email}))
	}
	require.NoError(t, store.Sources().Put(ctx, relay.Source{ID: id, Email: email, Name: id, IngressHash: "ih-" + id}))
}

// appendAged appends one delivery per age, oldest first, each 10 bytes.
func appendAged(t *testing.T, store relay.Store, now time.Time, email, sourceID string, ages ...time.Duration) {
	t.Helper()
	for _, age := range ages {
		d := relay.Delivery{
			ID:         randid.UUIDv7(),
			SourceID:   sourceID,
			ReceivedAt: now.Add(-age),
			Size:       10,
			Record:     bytes.Repeat([]byte("x"), 10),
		}
		require.NoError(t, store.Deliveries().Append(t.Context(), email, d, 1<<30, 0))
	}
}

func sweepLimits(retention time.Duration, perSource int) relay.Limits {
	l := relay.DefaultLimits()
	l.Retention = retention
	l.DeliveriesPerSource = perSource
	return l
}

func newSweeper(store relay.Store, limits relay.Limits, now time.Time) *relay.Sweeper {
	s := relay.NewSweeper(store, limits, zerolog.Nop())
	s.SetNow(func() time.Time { return now })
	return s
}

func TestSweeperTick(t *testing.T) {
	const h = time.Hour
	tests := []struct {
		name      string
		retention time.Duration
		perSource int
		ages      []time.Duration
		wantLeft  int
	}{
		{name: "nothing to do", retention: 24 * h, perSource: 10, ages: []time.Duration{3 * h, 2 * h, h}, wantLeft: 3},
		{name: "expired rows go", retention: 24 * h, perSource: 10, ages: []time.Duration{72 * h, 48 * h, h, 0}, wantLeft: 2},
		{name: "all expired", retention: h, perSource: 10, ages: []time.Duration{3 * h, 2 * h}, wantLeft: 0},
		{name: "over the row cap", retention: 24 * h, perSource: 2, ages: []time.Duration{4 * h, 3 * h, 2 * h, h}, wantLeft: 2},
		{name: "age and cap together", retention: 24 * h, perSource: 2, ages: []time.Duration{48 * h, 3 * h, 2 * h, h}, wantLeft: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			now := newClock().Now()
			store := newStore(t)
			putSweepSource(t, store, alice, "a1")
			appendAged(t, store, now, alice, "a1", tt.ages...)

			require.NoError(t, newSweeper(store, sweepLimits(tt.retention, tt.perSource), now).Tick(ctx))

			rows, _, err := store.Deliveries().ListAfter(ctx, "a1", "", 100)
			require.NoError(t, err)
			require.Len(t, rows, tt.wantLeft)
			cutoff := now.Add(-tt.retention)
			for _, d := range rows {
				assert.False(t, d.ReceivedAt.Before(cutoff), "expired row survived")
			}
			acct, err := store.Accounts().Get(ctx, alice)
			require.NoError(t, err)
			assert.Equal(t, int64(10*tt.wantLeft), acct.BytesUsed)
		})
	}
}

func TestSweeperTrimsSourcesIndependently(t *testing.T) {
	ctx := t.Context()
	now := newClock().Now()
	store := newStore(t)
	putSweepSource(t, store, alice, "a1")
	putSweepSource(t, store, alice, "a2")
	appendAged(t, store, now, alice, "a1", 5*time.Minute, 4*time.Minute, 3*time.Minute, 2*time.Minute)
	appendAged(t, store, now, alice, "a2", time.Minute)

	require.NoError(t, newSweeper(store, sweepLimits(time.Hour, 2), now).Tick(ctx))

	n, _, err := store.Deliveries().Stats(ctx, "a1")
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	n, _, err = store.Deliveries().Stats(ctx, "a2")
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	acct, err := store.Accounts().Get(ctx, alice)
	require.NoError(t, err)
	assert.Equal(t, int64(30), acct.BytesUsed)
}

var errBroken = errors.New("broken source")

type failingStore struct {
	relay.Store
	failSource string
}

func (s failingStore) Deliveries() relay.DeliveryStore {
	return failingDeliveries{s.Store.Deliveries(), s.failSource}
}

type failingDeliveries struct {
	relay.DeliveryStore
	failSource string
}

func (f failingDeliveries) DeleteBefore(ctx context.Context, email, sourceID string, before time.Time) (int, int64, error) {
	if sourceID == f.failSource {
		return 0, 0, errBroken
	}
	return f.DeliveryStore.DeleteBefore(ctx, email, sourceID, before)
}

func TestSweeperSkipsAFailingSource(t *testing.T) {
	ctx := t.Context()
	now := newClock().Now()
	store := newStore(t)
	putSweepSource(t, store, alice, "a1")
	putSweepSource(t, store, alice, "a2")
	putSweepSource(t, store, bob, "b1")
	for _, src := range []struct{ email, id string }{{alice, "a1"}, {alice, "a2"}, {bob, "b1"}} {
		appendAged(t, store, now, src.email, src.id, 48*time.Hour, time.Minute)
	}

	err := newSweeper(failingStore{store, "a1"}, sweepLimits(24*time.Hour, 10), now).Tick(ctx)
	require.ErrorIs(t, err, errBroken)

	for id, want := range map[string]int{"a1": 2, "a2": 1, "b1": 1} {
		n, _, err := store.Deliveries().Stats(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, want, n, id)
	}
}

func TestSweeperRunStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	now := newClock().Now()
	store := newStore(t)
	putSweepSource(t, store, alice, "a1")
	appendAged(t, store, now, alice, "a1", 48*time.Hour)

	done := make(chan error, 1)
	go func() { done <- newSweeper(store, sweepLimits(time.Hour, 10), now).Run(ctx, time.Hour) }()
	require.Eventually(t, func() bool {
		n, _, err := store.Deliveries().Stats(ctx, "a1")
		return err == nil && n == 0
	}, 5*time.Second, 10*time.Millisecond)
	cancel()
	require.NoError(t, <-done)
}

func TestSweeperDeletesExpiredCodes(t *testing.T) {
	ctx := t.Context()
	now := newClock().Now()
	store := newStore(t)
	require.NoError(t, store.AuthCodes().Put(ctx, alice, "old", now.Add(-time.Minute)))
	require.NoError(t, store.AuthCodes().Put(ctx, bob, "fresh", now.Add(time.Minute)))

	require.NoError(t, newSweeper(store, sweepLimits(time.Hour, 10), now).Tick(ctx))

	n, err := store.AuthCodes().DeleteExpired(ctx, now.Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 1, n, "only the fresh code should be left")
}
