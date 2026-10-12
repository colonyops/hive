// Package storetest is the contract every relay.Store driver passes.
package storetest

import (
	"bytes"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/relay/internal/relay"
	"github.com/colonyops/hive/pkg/randid"
)

const (
	alice = "alice@example.com"
	bob   = "bob@example.com"
)

var epoch = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// Run exercises open's store against the relay.Store contract. open returns
// a fresh, empty store for each subtest.
func Run(t *testing.T, open func(t *testing.T) relay.Store) {
	t.Helper()
	fresh := func(t *testing.T) relay.Store {
		t.Helper()
		s := open(t)
		t.Cleanup(func() { assert.NoError(t, s.Close()) })
		return s
	}
	t.Run("accounts", func(t *testing.T) { testAccounts(t, fresh) })
	t.Run("tokens", func(t *testing.T) { testTokens(t, fresh) })
	t.Run("sources", func(t *testing.T) { testSources(t, fresh) })
	t.Run("deliveries", func(t *testing.T) { testDeliveries(t, fresh) })
	t.Run("auth codes", func(t *testing.T) { testAuthCodes(t, fresh) })
	t.Run("outbox", func(t *testing.T) { testOutbox(t, fresh) })
	t.Run("recipients", func(t *testing.T) { testRecipients(t, fresh) })
}

func putAccount(t *testing.T, s relay.Store, email string) {
	t.Helper()
	require.NoError(t, s.Accounts().Put(t.Context(), relay.Account{Email: email, CreatedAt: epoch}))
}

func putSource(t *testing.T, s relay.Store, email, id string) relay.Source {
	t.Helper()
	src := relay.Source{
		ID:          id,
		Email:       email,
		Name:        id,
		IngressHash: "ingress-" + id,
		CreatedAt:   epoch,
	}
	require.NoError(t, s.Sources().Put(t.Context(), src))
	return src
}

func delivery(sourceID string, size int, at time.Time) relay.Delivery {
	return relay.Delivery{
		ID:         randid.UUIDv7(),
		SourceID:   sourceID,
		ReceivedAt: at,
		Size:       int64(size),
		Record:     bytes.Repeat([]byte("x"), size),
	}
}

func appendN(t *testing.T, s relay.Store, email, sourceID string, n, size int) []relay.Delivery {
	t.Helper()
	out := make([]relay.Delivery, n)
	for i := range out {
		out[i] = delivery(sourceID, size, epoch.Add(time.Duration(i)*time.Minute))
		require.NoError(t, s.Deliveries().Append(t.Context(), email, out[i], 1<<30, 1<<20))
	}
	return out
}

func bytesUsed(t *testing.T, s relay.Store, email string) int64 {
	t.Helper()
	a, err := s.Accounts().Get(t.Context(), email)
	require.NoError(t, err)
	return a.BytesUsed
}

func ids(ds []relay.Delivery) []string {
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.ID
	}
	return out
}

func testAccounts(t *testing.T, open func(t *testing.T) relay.Store) {
	t.Run("put get list", func(t *testing.T) {
		s := open(t)
		ctx := t.Context()
		want := relay.Account{Email: bob, Encryption: true, BytesUsed: 7, CreatedAt: epoch}
		require.NoError(t, s.Accounts().Put(ctx, want))
		putAccount(t, s, alice)

		got, err := s.Accounts().Get(ctx, bob)
		require.NoError(t, err)
		assert.Equal(t, want.Email, got.Email)
		assert.True(t, got.Encryption)
		assert.Equal(t, int64(7), got.BytesUsed)
		assert.True(t, epoch.Equal(got.CreatedAt))

		want.Encryption = false
		require.NoError(t, s.Accounts().Put(ctx, want))
		got, err = s.Accounts().Get(ctx, bob)
		require.NoError(t, err)
		assert.False(t, got.Encryption)

		list, err := s.Accounts().List(ctx)
		require.NoError(t, err)
		emails := make([]string, len(list))
		for i, a := range list {
			emails[i] = a.Email
		}
		assert.ElementsMatch(t, []string{alice, bob}, emails)
	})

	t.Run("put keeps bytes and created at", func(t *testing.T) {
		s := open(t)
		ctx := t.Context()
		putAccount(t, s, alice)
		putSource(t, s, alice, "a1")
		appendN(t, s, alice, "a1", 1, 10)
		require.Equal(t, int64(10), bytesUsed(t, s, alice))

		require.NoError(t, s.Accounts().Put(ctx, relay.Account{
			Email:      alice,
			Encryption: true,
			BytesUsed:  0,
			CreatedAt:  epoch.Add(24 * time.Hour),
		}))
		got, err := s.Accounts().Get(ctx, alice)
		require.NoError(t, err)
		assert.Equal(t, int64(10), got.BytesUsed)
		assert.True(t, got.Encryption)
		assert.True(t, epoch.Equal(got.CreatedAt))
	})

	t.Run("get missing", func(t *testing.T) {
		s := open(t)
		_, err := s.Accounts().Get(t.Context(), alice)
		require.ErrorIs(t, err, relay.ErrNotFound)
	})

	t.Run("add bytes", func(t *testing.T) {
		s := open(t)
		putAccount(t, s, alice)
		n, err := s.Accounts().AddBytes(t.Context(), alice, 100)
		require.NoError(t, err)
		assert.Equal(t, int64(100), n)
		n, err = s.Accounts().AddBytes(t.Context(), alice, -40)
		require.NoError(t, err)
		assert.Equal(t, int64(60), n)
		assert.Equal(t, int64(60), bytesUsed(t, s, alice))

		_, err = s.Accounts().AddBytes(t.Context(), bob, 1)
		require.ErrorIs(t, err, relay.ErrNotFound)
	})

	t.Run("delete cascades", func(t *testing.T) {
		s := open(t)
		ctx := t.Context()
		putAccount(t, s, alice)
		putAccount(t, s, bob)
		require.NoError(t, s.Tokens().Put(ctx, relay.Token{ID: "t1", Email: alice, Hash: "h1", Kind: relay.TokenAPI, CreatedAt: epoch}))
		require.NoError(t, s.Tokens().Put(ctx, relay.Token{ID: "t2", Email: bob, Hash: "h2", Kind: relay.TokenAPI, CreatedAt: epoch}))
		putSource(t, s, alice, "src-a")
		putSource(t, s, bob, "src-b")
		appendN(t, s, alice, "src-a", 3, 10)
		appendN(t, s, bob, "src-b", 2, 10)

		require.NoError(t, s.Accounts().Delete(ctx, alice))

		_, err := s.Accounts().Get(ctx, alice)
		require.ErrorIs(t, err, relay.ErrNotFound)
		_, err = s.Tokens().GetByHash(ctx, "h1")
		require.ErrorIs(t, err, relay.ErrNotFound)
		_, err = s.Sources().Get(ctx, alice, "src-a")
		require.ErrorIs(t, err, relay.ErrNotFound)
		n, b, err := s.Deliveries().Stats(ctx, "src-a")
		require.NoError(t, err)
		assert.Zero(t, n)
		assert.Zero(t, b)

		_, err = s.Tokens().GetByHash(ctx, "h2")
		require.NoError(t, err)
		n, _, err = s.Deliveries().Stats(ctx, "src-b")
		require.NoError(t, err)
		assert.Equal(t, 2, n)
		assert.Equal(t, int64(20), bytesUsed(t, s, bob))

		require.ErrorIs(t, s.Accounts().Delete(ctx, alice), relay.ErrNotFound)
	})
}

func testTokens(t *testing.T, open func(t *testing.T) relay.Store) {
	s := open(t)
	ctx := t.Context()
	putAccount(t, s, alice)
	putAccount(t, s, bob)
	t1 := relay.Token{ID: "t1", Email: alice, Hash: "h1", Kind: relay.TokenDevice, Device: "laptop", CreatedAt: epoch}
	t2 := relay.Token{ID: "t2", Email: alice, Hash: "h2", Kind: relay.TokenAPI, Device: "ci", CreatedAt: epoch.Add(time.Second)}
	t3 := relay.Token{ID: "t3", Email: bob, Hash: "h3", Kind: relay.TokenAPI, CreatedAt: epoch}
	for _, tok := range []relay.Token{t2, t1, t3} {
		require.NoError(t, s.Tokens().Put(ctx, tok))
	}

	got, err := s.Tokens().GetByHash(ctx, "h1")
	require.NoError(t, err)
	assert.Equal(t, "t1", got.ID)
	assert.Equal(t, alice, got.Email)
	assert.Equal(t, relay.TokenDevice, got.Kind)
	assert.Equal(t, "laptop", got.Device)
	assert.True(t, got.LastUsedAt.IsZero())

	_, err = s.Tokens().GetByHash(ctx, "nope")
	require.ErrorIs(t, err, relay.ErrNotFound)

	list, err := s.Tokens().ListByEmail(ctx, alice)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "t1", list[0].ID)
	assert.Equal(t, "t2", list[1].ID)

	used := epoch.Add(time.Hour)
	require.NoError(t, s.Tokens().Touch(ctx, "t1", used))
	got, err = s.Tokens().GetByHash(ctx, "h1")
	require.NoError(t, err)
	assert.True(t, used.Equal(got.LastUsedAt))
	require.ErrorIs(t, s.Tokens().Touch(ctx, "nope", used), relay.ErrNotFound)

	require.ErrorIs(t, s.Tokens().Delete(ctx, bob, "t1"), relay.ErrNotFound, "another account's token")
	require.NoError(t, s.Tokens().Delete(ctx, alice, "t1"))
	_, err = s.Tokens().GetByHash(ctx, "h1")
	require.ErrorIs(t, err, relay.ErrNotFound)
	require.ErrorIs(t, s.Tokens().Delete(ctx, alice, "t1"), relay.ErrNotFound)
}

func testSources(t *testing.T, open func(t *testing.T) relay.Store) {
	t.Run("get list count", func(t *testing.T) {
		s := open(t)
		ctx := t.Context()
		putAccount(t, s, alice)
		putAccount(t, s, bob)
		a2 := putSource(t, s, alice, "a2")
		a1 := relay.Source{ID: "a1", Email: alice, Name: "one", IngressHash: "ih-a1", SecretHash: "sh", CreatedAt: epoch.Add(-time.Hour)}
		require.NoError(t, s.Sources().Put(ctx, a1))
		putSource(t, s, bob, "b1")

		got, err := s.Sources().Get(ctx, alice, "a1")
		require.NoError(t, err)
		assert.Equal(t, a1.Name, got.Name)
		assert.Equal(t, a1.SecretHash, got.SecretHash)
		assert.Equal(t, a1.IngressHash, got.IngressHash)

		_, err = s.Sources().Get(ctx, bob, "a1")
		require.ErrorIs(t, err, relay.ErrNotFound, "another account's source")
		_, err = s.Sources().Get(ctx, alice, "missing")
		require.ErrorIs(t, err, relay.ErrNotFound)

		got, err = s.Sources().GetByIngressHash(ctx, a2.IngressHash)
		require.NoError(t, err)
		assert.Equal(t, "a2", got.ID)
		_, err = s.Sources().GetByIngressHash(ctx, "nope")
		require.ErrorIs(t, err, relay.ErrNotFound)

		n, err := s.Sources().CountByEmail(ctx, alice)
		require.NoError(t, err)
		assert.Equal(t, 2, n)
		n, err = s.Sources().CountByEmail(ctx, "nobody@example.com")
		require.NoError(t, err)
		assert.Zero(t, n)

		list, err := s.Sources().ListByEmail(ctx, alice)
		require.NoError(t, err)
		require.Len(t, list, 2)
		assert.Equal(t, "a1", list[0].ID)
		assert.Equal(t, "a2", list[1].ID)

		a1.Name = "renamed"
		require.NoError(t, s.Sources().Put(ctx, a1))
		got, err = s.Sources().Get(ctx, alice, "a1")
		require.NoError(t, err)
		assert.Equal(t, "renamed", got.Name)
	})

	t.Run("delete cascades and credits", func(t *testing.T) {
		s := open(t)
		ctx := t.Context()
		putAccount(t, s, alice)
		putSource(t, s, alice, "a1")
		putSource(t, s, alice, "a2")
		appendN(t, s, alice, "a1", 3, 100)
		appendN(t, s, alice, "a2", 2, 50)
		require.Equal(t, int64(400), bytesUsed(t, s, alice))

		require.ErrorIs(t, s.Sources().Delete(ctx, bob, "a1"), relay.ErrNotFound)
		require.NoError(t, s.Sources().Delete(ctx, alice, "a1"))
		assert.Equal(t, int64(100), bytesUsed(t, s, alice))
		n, _, err := s.Deliveries().Stats(ctx, "a1")
		require.NoError(t, err)
		assert.Zero(t, n)
		_, err = s.Sources().Get(ctx, alice, "a1")
		require.ErrorIs(t, err, relay.ErrNotFound)
		require.ErrorIs(t, s.Sources().Delete(ctx, alice, "a1"), relay.ErrNotFound)
	})
}

func testDeliveries(t *testing.T, open func(t *testing.T) relay.Store) {
	setup := func(t *testing.T) relay.Store {
		t.Helper()
		s := open(t)
		putAccount(t, s, alice)
		putSource(t, s, alice, "a1")
		return s
	}

	t.Run("append keeps order and debits", func(t *testing.T) {
		s := setup(t)
		ctx := t.Context()
		want := appendN(t, s, alice, "a1", 5, 10)
		assert.Equal(t, int64(50), bytesUsed(t, s, alice))

		got, more, err := s.Deliveries().ListAfter(ctx, "a1", "", 100)
		require.NoError(t, err)
		assert.False(t, more)
		assert.Equal(t, ids(want), ids(got))
		assert.Equal(t, want[0].Record, got[0].Record)
		assert.Equal(t, "a1", got[0].SourceID)
		assert.True(t, want[0].ReceivedAt.Equal(got[0].ReceivedAt))
		assert.Equal(t, want[0].Size, got[0].Size)

		got[0].Record[0] = 'y'
		again, _, err := s.Deliveries().ListAfter(ctx, "a1", "", 1)
		require.NoError(t, err)
		assert.Equal(t, byte('x'), again[0].Record[0], "callers must not reach stored bytes")
	})

	t.Run("append rejects unknown account and source", func(t *testing.T) {
		s := setup(t)
		ctx := t.Context()
		require.ErrorIs(t, s.Deliveries().Append(ctx, bob, delivery("a1", 1, epoch), 1<<20, 10), relay.ErrNotFound)
		require.ErrorIs(t, s.Deliveries().Append(ctx, alice, delivery("missing", 1, epoch), 1<<20, 10), relay.ErrNotFound)
		assert.Zero(t, bytesUsed(t, s, alice))
	})

	t.Run("list after pages", func(t *testing.T) {
		s := setup(t)
		ctx := t.Context()
		want := appendN(t, s, alice, "a1", 5, 10)

		page, more, err := s.Deliveries().ListAfter(ctx, "a1", "", 2)
		require.NoError(t, err)
		assert.True(t, more)
		assert.Equal(t, ids(want[:2]), ids(page))

		page, more, err = s.Deliveries().ListAfter(ctx, "a1", page[1].ID, 2)
		require.NoError(t, err)
		assert.True(t, more)
		assert.Equal(t, ids(want[2:4]), ids(page))

		page, more, err = s.Deliveries().ListAfter(ctx, "a1", page[1].ID, 2)
		require.NoError(t, err)
		assert.False(t, more)
		assert.Equal(t, ids(want[4:]), ids(page))

		page, more, err = s.Deliveries().ListAfter(ctx, "a1", want[4].ID, 2)
		require.NoError(t, err)
		assert.False(t, more)
		assert.Empty(t, page)

		page, _, err = s.Deliveries().ListAfter(ctx, "unknown", "", 2)
		require.NoError(t, err)
		assert.Empty(t, page)
	})

	t.Run("over cap stores nothing", func(t *testing.T) {
		s := setup(t)
		ctx := t.Context()
		require.NoError(t, s.Deliveries().Append(ctx, alice, delivery("a1", 60, epoch), 100, 10))
		err := s.Deliveries().Append(ctx, alice, delivery("a1", 41, epoch), 100, 10)
		require.Error(t, err)
		assert.Equal(t, relay.KindOverCap, relay.KindOf(err))
		assert.Equal(t, int64(60), bytesUsed(t, s, alice))
		n, b, err := s.Deliveries().Stats(ctx, "a1")
		require.NoError(t, err)
		assert.Equal(t, 1, n)
		assert.Equal(t, int64(60), b)

		require.NoError(t, s.Deliveries().Append(ctx, alice, delivery("a1", 40, epoch), 100, 10), "exactly at the cap fits")
	})

	t.Run("inline trim", func(t *testing.T) {
		s := setup(t)
		ctx := t.Context()
		var all []relay.Delivery
		for i := range 5 {
			d := delivery("a1", 10*(i+1), epoch)
			require.NoError(t, s.Deliveries().Append(ctx, alice, d, 1<<20, 3))
			all = append(all, d)
		}
		got, _, err := s.Deliveries().ListAfter(ctx, "a1", "", 10)
		require.NoError(t, err)
		assert.Equal(t, ids(all[2:]), ids(got))
		assert.Equal(t, int64(30+40+50), bytesUsed(t, s, alice))
	})

	t.Run("delete before", func(t *testing.T) {
		s := setup(t)
		ctx := t.Context()
		want := appendN(t, s, alice, "a1", 5, 10)
		n, b, err := s.Deliveries().DeleteBefore(ctx, alice, "a1", want[2].ReceivedAt)
		require.NoError(t, err)
		assert.Equal(t, 2, n)
		assert.Equal(t, int64(20), b)
		assert.Equal(t, int64(30), bytesUsed(t, s, alice))
		got, _, err := s.Deliveries().ListAfter(ctx, "a1", "", 10)
		require.NoError(t, err)
		assert.Equal(t, ids(want[2:]), ids(got))

		_, _, err = s.Deliveries().DeleteBefore(ctx, bob, "a1", epoch.Add(time.Hour))
		require.ErrorIs(t, err, relay.ErrNotFound)
	})

	t.Run("trim to", func(t *testing.T) {
		s := setup(t)
		ctx := t.Context()
		want := appendN(t, s, alice, "a1", 5, 10)
		n, b, err := s.Deliveries().TrimTo(ctx, alice, "a1", 2)
		require.NoError(t, err)
		assert.Equal(t, 3, n)
		assert.Equal(t, int64(30), b)
		assert.Equal(t, int64(20), bytesUsed(t, s, alice))
		got, _, err := s.Deliveries().ListAfter(ctx, "a1", "", 10)
		require.NoError(t, err)
		assert.Equal(t, ids(want[3:]), ids(got))

		n, b, err = s.Deliveries().TrimTo(ctx, alice, "a1", 2)
		require.NoError(t, err)
		assert.Zero(t, n)
		assert.Zero(t, b)

		_, _, err = s.Deliveries().TrimTo(ctx, bob, "a1", 0)
		require.ErrorIs(t, err, relay.ErrNotFound)
	})

	t.Run("stats", func(t *testing.T) {
		s := setup(t)
		appendN(t, s, alice, "a1", 4, 25)
		n, b, err := s.Deliveries().Stats(t.Context(), "a1")
		require.NoError(t, err)
		assert.Equal(t, 4, n)
		assert.Equal(t, int64(100), b)
	})

	t.Run("concurrent appends hold the cap", func(t *testing.T) {
		s := setup(t)
		ctx := t.Context()
		const (
			workers  = 100
			size     = 1 << 10
			capBytes = 50 << 10
		)
		var (
			wg      sync.WaitGroup
			mu      sync.Mutex
			overCap int
			other   []error
		)
		for range workers {
			wg.Go(func() {
				err := s.Deliveries().Append(ctx, alice, delivery("a1", size, epoch), capBytes, 1<<20)
				if err == nil {
					return
				}
				mu.Lock()
				defer mu.Unlock()
				if relay.KindOf(err) == relay.KindOverCap {
					overCap++
				} else {
					other = append(other, err)
				}
			})
		}
		wg.Wait()

		require.Empty(t, other)
		assert.Positive(t, overCap)
		rows, _, err := s.Deliveries().ListAfter(ctx, "a1", "", workers)
		require.NoError(t, err)
		var sum int64
		for _, d := range rows {
			sum += d.Size
		}
		used := bytesUsed(t, s, alice)
		assert.Equal(t, sum, used)
		assert.LessOrEqual(t, used, int64(capBytes))
		assert.Equal(t, workers, len(rows)+overCap)
	})
}

func testAuthCodes(t *testing.T, open func(t *testing.T) relay.Store) {
	const maxAttempts = 3
	expires := epoch.Add(10 * time.Minute)

	t.Run("put consume ok", func(t *testing.T) {
		s := open(t)
		ctx := t.Context()
		require.NoError(t, s.AuthCodes().Put(ctx, alice, "h1", expires))
		ok, _, err := s.AuthCodes().Consume(ctx, alice, "h1", epoch, maxAttempts)
		require.NoError(t, err)
		assert.True(t, ok)

		ok, left, err := s.AuthCodes().Consume(ctx, alice, "h1", epoch, maxAttempts)
		require.NoError(t, err)
		assert.False(t, ok, "a consumed code works once")
		assert.Zero(t, left)
	})

	t.Run("wrong code counts down then deletes", func(t *testing.T) {
		s := open(t)
		ctx := t.Context()
		require.NoError(t, s.AuthCodes().Put(ctx, alice, "h1", expires))
		for want := maxAttempts - 1; want > 0; want-- {
			ok, left, err := s.AuthCodes().Consume(ctx, alice, "nope", epoch, maxAttempts)
			require.NoError(t, err)
			assert.False(t, ok)
			assert.Equal(t, want, left)
		}
		ok, left, err := s.AuthCodes().Consume(ctx, alice, "nope", epoch, maxAttempts)
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Zero(t, left)

		ok, _, err = s.AuthCodes().Consume(ctx, alice, "h1", epoch, maxAttempts)
		require.NoError(t, err)
		assert.False(t, ok, "the cap deletes the code")
	})

	t.Run("expired is a miss", func(t *testing.T) {
		s := open(t)
		ctx := t.Context()
		require.NoError(t, s.AuthCodes().Put(ctx, alice, "h1", expires))
		ok, left, err := s.AuthCodes().Consume(ctx, alice, "h1", expires, maxAttempts)
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Zero(t, left)

		require.NoError(t, s.AuthCodes().Put(ctx, alice, "h1", expires))
		ok, _, err = s.AuthCodes().Consume(ctx, alice, "h1", epoch, maxAttempts)
		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("put replaces and resets attempts", func(t *testing.T) {
		s := open(t)
		ctx := t.Context()
		require.NoError(t, s.AuthCodes().Put(ctx, alice, "h1", expires))
		_, left, err := s.AuthCodes().Consume(ctx, alice, "nope", epoch, maxAttempts)
		require.NoError(t, err)
		require.Equal(t, maxAttempts-1, left)

		require.NoError(t, s.AuthCodes().Put(ctx, alice, "h2", expires))
		_, left, err = s.AuthCodes().Consume(ctx, alice, "h1", epoch, maxAttempts)
		require.NoError(t, err)
		assert.Equal(t, maxAttempts-1, left, "the old code is gone and attempts reset")
		ok, _, err := s.AuthCodes().Consume(ctx, alice, "h2", epoch, maxAttempts)
		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("delete expired", func(t *testing.T) {
		s := open(t)
		ctx := t.Context()
		require.NoError(t, s.AuthCodes().Put(ctx, alice, "h1", epoch.Add(time.Minute)))
		require.NoError(t, s.AuthCodes().Put(ctx, bob, "h2", epoch.Add(time.Hour)))
		n, err := s.AuthCodes().DeleteExpired(ctx, epoch.Add(2*time.Minute))
		require.NoError(t, err)
		assert.Equal(t, 1, n)
		ok, _, err := s.AuthCodes().Consume(ctx, bob, "h2", epoch, maxAttempts)
		require.NoError(t, err)
		assert.True(t, ok)
	})
}

func mail(id string, created time.Time) relay.Mail {
	return relay.Mail{ID: id, To: alice, Subject: "s-" + id, Body: "b-" + id, CreatedAt: created, NextAt: created}
}

func mailIDs(ms []relay.Mail) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.ID
	}
	return out
}

func testOutbox(t *testing.T, open func(t *testing.T) relay.Store) {
	t.Run("append and due order", func(t *testing.T) {
		s := open(t)
		ctx := t.Context()
		require.NoError(t, s.Outbox().Append(ctx, mail("m2", epoch.Add(time.Second))))
		require.NoError(t, s.Outbox().Append(ctx, mail("m1", epoch)))
		require.NoError(t, s.Outbox().Append(ctx, mail("m3", epoch.Add(time.Hour))))

		due, err := s.Outbox().Due(ctx, epoch.Add(time.Minute), 10)
		require.NoError(t, err)
		assert.Equal(t, []string{"m1", "m2"}, mailIDs(due))
		assert.Equal(t, alice, due[0].To)
		assert.Equal(t, "s-m1", due[0].Subject)
		assert.Equal(t, "b-m1", due[0].Body)
		assert.True(t, epoch.Equal(due[0].CreatedAt))
		assert.True(t, due[0].SentAt.IsZero())

		due, err = s.Outbox().Due(ctx, epoch.Add(time.Minute), 1)
		require.NoError(t, err)
		assert.Equal(t, []string{"m1"}, mailIDs(due))
	})

	t.Run("mark sent excludes", func(t *testing.T) {
		s := open(t)
		ctx := t.Context()
		require.NoError(t, s.Outbox().Append(ctx, mail("m1", epoch)))
		require.NoError(t, s.Outbox().Append(ctx, mail("m2", epoch)))
		require.NoError(t, s.Outbox().MarkSent(ctx, "m1", epoch))
		due, err := s.Outbox().Due(ctx, epoch, 10)
		require.NoError(t, err)
		assert.Equal(t, []string{"m2"}, mailIDs(due))
		assert.ErrorIs(t, s.Outbox().MarkSent(ctx, "missing", epoch), relay.ErrNotFound)
	})

	t.Run("mark failed defers and bumps attempts", func(t *testing.T) {
		s := open(t)
		ctx := t.Context()
		require.NoError(t, s.Outbox().Append(ctx, mail("m1", epoch)))
		next := epoch.Add(30 * time.Second)
		require.NoError(t, s.Outbox().MarkFailed(ctx, "m1", next))

		due, err := s.Outbox().Due(ctx, epoch.Add(29*time.Second), 10)
		require.NoError(t, err)
		assert.Empty(t, due)

		due, err = s.Outbox().Due(ctx, next, 10)
		require.NoError(t, err)
		require.Len(t, due, 1)
		assert.Equal(t, 1, due[0].Attempts)
		assert.True(t, next.Equal(due[0].NextAt))
		assert.ErrorIs(t, s.Outbox().MarkFailed(ctx, "missing", next), relay.ErrNotFound)
	})
}

func recipient(email, id, key string, at time.Time) relay.Recipient {
	return relay.Recipient{ID: id, Email: email, Recipient: key, Device: "dev-" + id, CreatedAt: at}
}

func recipientIDs(rs []relay.Recipient) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.ID
	}
	return out
}

func testRecipients(t *testing.T, open func(t *testing.T) relay.Store) {
	t.Run("put and list order", func(t *testing.T) {
		s := open(t)
		ctx := t.Context()
		putAccount(t, s, alice)
		putAccount(t, s, bob)
		require.NoError(t, s.Recipients().Put(ctx, recipient(alice, "r3", "age1c", epoch.Add(time.Minute))))
		require.NoError(t, s.Recipients().Put(ctx, recipient(alice, "r2", "age1b", epoch)))
		require.NoError(t, s.Recipients().Put(ctx, recipient(alice, "r1", "age1a", epoch)))
		require.NoError(t, s.Recipients().Put(ctx, recipient(bob, "b1", "age1a", epoch)))

		got, err := s.Recipients().ListByEmail(ctx, alice)
		require.NoError(t, err)
		assert.Equal(t, []string{"r1", "r2", "r3"}, recipientIDs(got))
		assert.Equal(t, "age1a", got[0].Recipient)
		assert.Equal(t, "dev-r1", got[0].Device)
		assert.Equal(t, alice, got[0].Email)
		assert.True(t, epoch.Equal(got[0].CreatedAt))

		none, err := s.Recipients().ListByEmail(ctx, "nobody@example.com")
		require.NoError(t, err)
		assert.Empty(t, none)
	})

	t.Run("put upserts on the same recipient", func(t *testing.T) {
		s := open(t)
		ctx := t.Context()
		putAccount(t, s, alice)
		require.NoError(t, s.Recipients().Put(ctx, recipient(alice, "r1", "age1a", epoch)))
		again := recipient(alice, "r2", "age1a", epoch.Add(time.Hour))
		again.Device = "renamed"
		require.NoError(t, s.Recipients().Put(ctx, again))

		got, err := s.Recipients().ListByEmail(ctx, alice)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "r1", got[0].ID)
		assert.Equal(t, "renamed", got[0].Device)
		assert.True(t, epoch.Equal(got[0].CreatedAt))
	})

	t.Run("delete is scoped by email", func(t *testing.T) {
		s := open(t)
		ctx := t.Context()
		putAccount(t, s, alice)
		putAccount(t, s, bob)
		require.NoError(t, s.Recipients().Put(ctx, recipient(alice, "r1", "age1a", epoch)))

		require.ErrorIs(t, s.Recipients().Delete(ctx, bob, "r1"), relay.ErrNotFound)
		require.ErrorIs(t, s.Recipients().Delete(ctx, alice, "missing"), relay.ErrNotFound)
		require.NoError(t, s.Recipients().Delete(ctx, alice, "r1"))
		require.ErrorIs(t, s.Recipients().Delete(ctx, alice, "r1"), relay.ErrNotFound)
	})

	t.Run("account delete cascades", func(t *testing.T) {
		s := open(t)
		ctx := t.Context()
		putAccount(t, s, alice)
		putAccount(t, s, bob)
		require.NoError(t, s.Recipients().Put(ctx, recipient(alice, "r1", "age1a", epoch)))
		require.NoError(t, s.Recipients().Put(ctx, recipient(bob, "b1", "age1b", epoch)))

		require.NoError(t, s.Accounts().Delete(ctx, alice))
		got, err := s.Recipients().ListByEmail(ctx, alice)
		require.NoError(t, err)
		assert.Empty(t, got)
		got, err = s.Recipients().ListByEmail(ctx, bob)
		require.NoError(t, err)
		assert.Len(t, got, 1)
	})
}
