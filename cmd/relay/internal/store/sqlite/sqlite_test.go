package sqlite_test

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/relay/internal/relay"
	"github.com/colonyops/hive/cmd/relay/internal/store/sqlite"
	"github.com/colonyops/hive/pkg/randid"
)

func open(t *testing.T, path string) relay.Store {
	t.Helper()
	s, err := sqlite.Open(t.Context(), path)
	require.NoError(t, err)
	return s
}

func TestOverCapAndRestart(t *testing.T) {
	const email = "alice@example.com"
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "relay.db")
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

	s := open(t, path)
	require.NoError(t, s.Accounts().Put(ctx, relay.Account{Email: email, CreatedAt: at}))
	require.NoError(t, s.Sources().Put(ctx, relay.Source{ID: "src", Email: email, Name: "src", IngressHash: "ih", CreatedAt: at}))

	var want []string
	for range 3 {
		d := relay.Delivery{ID: randid.UUIDv7(), SourceID: "src", ReceivedAt: at, Size: 30, Record: bytes.Repeat([]byte("x"), 30)}
		require.NoError(t, s.Deliveries().Append(ctx, email, d, 100, 10))
		want = append(want, d.ID)
	}

	over := relay.Delivery{ID: randid.UUIDv7(), SourceID: "src", ReceivedAt: at, Size: 11, Record: []byte("y")}
	err := s.Deliveries().Append(ctx, email, over, 100, 10)
	require.Equal(t, relay.KindOverCap, relay.KindOf(err))
	acct, err := s.Accounts().Get(ctx, email)
	require.NoError(t, err)
	assert.Equal(t, int64(90), acct.BytesUsed)
	n, _, err := s.Deliveries().Stats(ctx, "src")
	require.NoError(t, err)
	assert.Equal(t, 3, n)

	require.NoError(t, s.Close())
	s = open(t, path)
	t.Cleanup(func() { assert.NoError(t, s.Close()) })

	got, more, err := s.Deliveries().ListAfter(ctx, "src", "", 10)
	require.NoError(t, err)
	assert.False(t, more)
	ids := make([]string, len(got))
	for i, d := range got {
		ids[i] = d.ID
	}
	assert.Equal(t, want, ids)
	acct, err = s.Accounts().Get(ctx, email)
	require.NoError(t, err)
	assert.Equal(t, int64(90), acct.BytesUsed)
}
