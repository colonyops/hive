package relay_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/relay/internal/relay"
)

func TestSourcesLifecycle(t *testing.T) {
	store := newStore()
	ctx := t.Context()
	svc := relay.NewSources(store, relay.DefaultLimits(), "https://relay.test")

	src, token, err := svc.Create(ctx, alice, "gotify", "s3cret")
	require.NoError(t, err)
	assert.Equal(t, relay.HashToken(token), src.IngressHash)
	assert.Equal(t, relay.HashToken("s3cret"), src.SecretHash)
	_, err = store.Accounts().Get(ctx, alice)
	require.NoError(t, err, "create makes the account")

	second, _, err := svc.Create(ctx, alice, "ntfy", "")
	require.NoError(t, err)
	assert.Empty(t, second.SecretHash)

	list, err := svc.List(ctx, alice)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, src.ID, list[0].ID)
	assert.Equal(t, second.ID, list[1].ID)

	ingress := relay.NewIngress(store, relay.DefaultLimits())
	_, err = ingress.Accept(ctx, token, "s3cret", relay.Record{Method: "POST", Body: []byte("hi")})
	require.NoError(t, err)
	view, err := svc.Get(ctx, alice, src.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, view.Deliveries)
	assert.Positive(t, view.Bytes)

	_, err = svc.Get(ctx, bob, src.ID)
	requireKind(t, relay.KindNotFound, err)

	name, empty := "renamed", ""
	updated, err := svc.Update(ctx, alice, src.ID, &name, nil)
	require.NoError(t, err)
	assert.Equal(t, "renamed", updated.Name)
	assert.Equal(t, src.SecretHash, updated.SecretHash)
	updated, err = svc.Update(ctx, alice, src.ID, nil, &empty)
	require.NoError(t, err)
	assert.Equal(t, "renamed", updated.Name)
	assert.Empty(t, updated.SecretHash)

	newToken, err := svc.Rotate(ctx, alice, src.ID)
	require.NoError(t, err)
	_, err = store.Sources().GetByIngressHash(ctx, relay.HashToken(token))
	require.ErrorIs(t, err, relay.ErrNotFound)
	got, err := store.Sources().GetByIngressHash(ctx, relay.HashToken(newToken))
	require.NoError(t, err)
	assert.Equal(t, src.ID, got.ID)

	require.NoError(t, svc.Delete(ctx, alice, src.ID))
	requireKind(t, relay.KindNotFound, svc.Delete(ctx, alice, src.ID))
	acct, err := store.Accounts().Get(ctx, alice)
	require.NoError(t, err)
	assert.Zero(t, acct.BytesUsed)
}

func TestSourcesCap(t *testing.T) {
	limits := relay.DefaultLimits()
	limits.SourcesPerAccount = 2
	svc := relay.NewSources(newStore(), limits, "https://relay.test")
	for range 2 {
		_, _, err := svc.Create(t.Context(), alice, "s", "")
		require.NoError(t, err)
	}
	_, _, err := svc.Create(t.Context(), alice, "s", "")
	requireKind(t, relay.KindInvalid, err)
	_, _, err = svc.Create(t.Context(), bob, "s", "")
	require.NoError(t, err)
}

func TestSourcesIngressURL(t *testing.T) {
	tests := []struct{ public, want string }{
		{"https://relay.test", "https://relay.test/in/tok"},
		{"https://relay.test/", "https://relay.test/in/tok"},
		{"https://relay.test/base/", "https://relay.test/base/in/tok"},
	}
	for _, tt := range tests {
		t.Run(tt.public, func(t *testing.T) {
			assert.Equal(t, tt.want, relay.NewSources(newStore(), relay.DefaultLimits(), tt.public).IngressURL("tok"))
		})
	}
}
