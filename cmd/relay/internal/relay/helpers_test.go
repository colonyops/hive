package relay_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/relay/internal/relay"
	"github.com/colonyops/hive/cmd/relay/internal/store/memory"
)

const (
	alice = "alice@example.com"
	bob   = "bob@example.com"
)

type clock struct{ t time.Time }

func newClock() *clock { return &clock{t: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)} }

func (c *clock) Now() time.Time          { return c.t }
func (c *clock) Advance(d time.Duration) { c.t = c.t.Add(d) }

// countingStore counts Touch calls so tests can see the hot path stay a read.
type countingStore struct {
	relay.Store
	touches atomic.Int64
}

func (s *countingStore) Tokens() relay.TokenStore { return countingTokens{s.Store.Tokens(), s} }

type countingTokens struct {
	relay.TokenStore
	s *countingStore
}

func (c countingTokens) Touch(ctx context.Context, id string, at time.Time) error {
	c.s.touches.Add(1)
	return c.TokenStore.Touch(ctx, id, at)
}

func requireKind(t *testing.T, want relay.Kind, err error) {
	t.Helper()
	require.Error(t, err)
	require.Equal(t, want, relay.KindOf(err), "error: %v", err)
}

func newSource(t *testing.T, store relay.Store, email, secret string) (relay.Source, string) {
	t.Helper()
	src, token, err := relay.NewSources(store, relay.DefaultLimits(), "https://relay.test").Create(t.Context(), email, "src", secret)
	require.NoError(t, err)
	return src, token
}

func newStore() relay.Store { return memory.New() }
