package relay_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/relay/internal/relay"
)

func TestAuthMintAndAuthenticate(t *testing.T) {
	store := newStore(t)
	clk := newClock()
	auth := relay.NewAuth(store, nil)
	auth.SetNow(clk.Now)

	plain, tok, err := auth.MintToken(t.Context(), "  Alice@Example.COM ", "laptop", relay.TokenDevice)
	require.NoError(t, err)
	assert.NotEmpty(t, plain)
	assert.Equal(t, alice, tok.Email)
	assert.Equal(t, relay.HashToken(plain), tok.Hash)
	assert.Equal(t, "laptop", tok.Device)

	acct, err := store.Accounts().Get(t.Context(), alice)
	require.NoError(t, err)
	assert.True(t, clk.Now().Equal(acct.CreatedAt))

	gotAcct, gotTok, err := auth.Authenticate(t.Context(), plain)
	require.NoError(t, err)
	assert.Equal(t, alice, gotAcct.Email)
	assert.Equal(t, tok.ID, gotTok.ID)

	_, _, err = auth.MintToken(t.Context(), alice, "ci", relay.TokenKind("root"))
	requireKind(t, relay.KindInvalid, err)
	_, _, err = auth.MintToken(t.Context(), " ", "ci", relay.TokenAPI)
	requireKind(t, relay.KindInvalid, err)
}

func TestAuthAuthenticateErrors(t *testing.T) {
	tests := []struct {
		name   string
		bearer func(plain string) string
		allow  func(string) bool
		want   relay.Kind
	}{
		{name: "empty", bearer: func(string) string { return "" }, want: relay.KindUnauthorized},
		{name: "wrong", bearer: func(p string) string { return p + "x" }, want: relay.KindUnauthorized},
		{name: "not allowed", bearer: func(p string) string { return p }, allow: func(string) bool { return false }, want: relay.KindForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := relay.NewAuth(newStore(t), tt.allow)
			plain, _, err := auth.MintToken(t.Context(), alice, "laptop", relay.TokenDevice)
			require.NoError(t, err)
			_, _, err = auth.Authenticate(t.Context(), tt.bearer(plain))
			requireKind(t, tt.want, err)
		})
	}
}

func TestAuthTouchesOncePerHour(t *testing.T) {
	store := &countingStore{Store: newStore(t)}
	clk := newClock()
	auth := relay.NewAuth(store, nil)
	auth.SetNow(clk.Now)
	plain, tok, err := auth.MintToken(t.Context(), alice, "laptop", relay.TokenDevice)
	require.NoError(t, err)

	_, got, err := auth.Authenticate(t.Context(), plain)
	require.NoError(t, err)
	assert.True(t, clk.Now().Equal(got.LastUsedAt))
	first := clk.Now()

	clk.Advance(30 * time.Minute)
	_, _, err = auth.Authenticate(t.Context(), plain)
	require.NoError(t, err)
	assert.Equal(t, int64(1), store.touches.Load())

	stored, err := store.Tokens().GetByHash(t.Context(), tok.Hash)
	require.NoError(t, err)
	assert.True(t, first.Equal(stored.LastUsedAt))

	clk.Advance(31 * time.Minute)
	_, _, err = auth.Authenticate(t.Context(), plain)
	require.NoError(t, err)
	assert.Equal(t, int64(2), store.touches.Load())

	// A fresh Auth (a restart) trusts the stored LastUsedAt.
	restarted := relay.NewAuth(store, nil)
	restarted.SetNow(clk.Now)
	_, _, err = restarted.Authenticate(t.Context(), plain)
	require.NoError(t, err)
	assert.Equal(t, int64(2), store.touches.Load())
}

func TestAuthListAndRevoke(t *testing.T) {
	auth := relay.NewAuth(newStore(t), nil)
	plain, tok, err := auth.MintToken(t.Context(), alice, "laptop", relay.TokenDevice)
	require.NoError(t, err)
	_, _, err = auth.MintToken(t.Context(), alice, "ci", relay.TokenAPI)
	require.NoError(t, err)

	toks, err := auth.ListTokens(t.Context(), alice)
	require.NoError(t, err)
	assert.Len(t, toks, 2)

	requireKind(t, relay.KindNotFound, auth.RevokeToken(t.Context(), bob, tok.ID))
	require.NoError(t, auth.RevokeToken(t.Context(), alice, tok.ID))
	requireKind(t, relay.KindNotFound, auth.RevokeToken(t.Context(), alice, tok.ID))

	_, _, err = auth.Authenticate(t.Context(), plain)
	requireKind(t, relay.KindUnauthorized, err)
}

func TestNewToken(t *testing.T) {
	a, err := relay.NewToken()
	require.NoError(t, err)
	b, err := relay.NewToken()
	require.NoError(t, err)
	assert.Len(t, a, 43)
	assert.NotEqual(t, a, b)
	assert.Len(t, relay.HashToken(a), 64)
}
