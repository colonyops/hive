package relay_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"testing"

	"filippo.io/age"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/relay/internal/relay"
)

func newIdentity(t *testing.T) *age.X25519Identity {
	t.Helper()
	id, err := age.GenerateX25519Identity()
	require.NoError(t, err)
	return id
}

func open(blob []byte, id age.Identity) ([]byte, error) {
	r, err := age.Decrypt(bytes.NewReader(blob), id)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(r)
}

type encEnv struct {
	store    relay.Store
	accounts *relay.Accounts
	ingress  *relay.Ingress
	feed     *relay.Feed
	src      relay.Source
	token    string
}

func newEncEnv(t *testing.T) *encEnv {
	t.Helper()
	store := newStore()
	src, token := newSource(t, store, alice, "")
	return &encEnv{
		store:    store,
		accounts: relay.NewAccounts(store),
		ingress:  relay.NewIngress(store, relay.DefaultLimits()),
		feed:     relay.NewFeed(store),
		src:      src,
		token:    token,
	}
}

func (e *encEnv) deliver(t *testing.T, body string) relay.Delivery {
	t.Helper()
	id, err := e.ingress.Accept(t.Context(), e.token, "", relay.Record{Method: "POST", Body: []byte(body), ContentType: "application/json"})
	require.NoError(t, err)
	rows, _, err := e.store.Deliveries().ListAfter(t.Context(), e.src.ID, "", 1000)
	require.NoError(t, err)
	for _, d := range rows {
		if d.ID == id {
			return d
		}
	}
	t.Fatalf("delivery %s not stored", id)
	return relay.Delivery{}
}

func TestSetEncryptionNeedsARecipient(t *testing.T) {
	e := newEncEnv(t)
	_, err := e.accounts.SetEncryption(t.Context(), alice, true)
	requireKind(t, relay.KindInvalid, err)

	acct, err := e.accounts.SetEncryption(t.Context(), alice, false)
	require.NoError(t, err)
	assert.False(t, acct.Encryption)

	_, err = e.accounts.SetEncryption(t.Context(), "nobody@example.com", false)
	requireKind(t, relay.KindNotFound, err)
}

func TestAddRecipientValidates(t *testing.T) {
	e := newEncEnv(t)
	ctx := t.Context()
	_, err := e.accounts.AddRecipient(ctx, alice, "not-a-key", "")
	requireKind(t, relay.KindInvalid, err)
	_, err = e.accounts.AddRecipient(ctx, alice, newIdentity(t).Recipient().String(), string(bytes.Repeat([]byte("d"), 81)))
	requireKind(t, relay.KindInvalid, err)

	key := newIdentity(t).Recipient().String()
	first, err := e.accounts.AddRecipient(ctx, alice, key, "laptop")
	require.NoError(t, err)
	assert.Equal(t, "laptop", first.Device)
	again, err := e.accounts.AddRecipient(ctx, alice, key, "desk")
	require.NoError(t, err)
	assert.Equal(t, first.ID, again.ID)
	assert.Equal(t, "desk", again.Device)

	rs, err := e.accounts.ListRecipients(ctx, alice)
	require.NoError(t, err)
	assert.Len(t, rs, 1)

	requireKind(t, relay.KindNotFound, e.accounts.RemoveRecipient(ctx, alice, "missing"))
	requireKind(t, relay.KindNotFound, e.accounts.RemoveRecipient(ctx, bob, first.ID))
}

func TestEncryptedDeliveries(t *testing.T) {
	e := newEncEnv(t)
	ctx := t.Context()
	one, two := newIdentity(t), newIdentity(t)
	r1, err := e.accounts.AddRecipient(ctx, alice, one.Recipient().String(), "one")
	require.NoError(t, err)
	_, err = e.accounts.AddRecipient(ctx, alice, two.Recipient().String(), "two")
	require.NoError(t, err)

	plain := e.deliver(t, `{"n":0}`)
	assert.False(t, plain.Enc)

	acct, err := e.accounts.SetEncryption(ctx, alice, true)
	require.NoError(t, err)
	require.True(t, acct.Encryption)

	sealed := e.deliver(t, `{"n":1}`)
	require.True(t, sealed.Enc)
	assert.Equal(t, int64(len(sealed.Record)), sealed.Size)
	assert.NotContains(t, string(sealed.Record), `"n":1`)
	for _, id := range []age.Identity{one, two} {
		raw, err := open(sealed.Record, id)
		require.NoError(t, err)
		var rec relay.Record
		require.NoError(t, json.Unmarshal(raw, &rec))
		assert.Equal(t, "POST", rec.Method)
		assert.JSONEq(t, `{"n":1}`, string(rec.Body))
	}

	acct, err = e.store.Accounts().Get(ctx, alice)
	require.NoError(t, err)
	assert.Equal(t, plain.Size+sealed.Size, acct.BytesUsed)

	require.NoError(t, e.accounts.RemoveRecipient(ctx, alice, r1.ID))
	later := e.deliver(t, `{"n":2}`)
	_, err = open(later.Record, one)
	require.Error(t, err)
	raw, err := open(later.Record, two)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "POST")
	_, err = open(sealed.Record, one)
	require.NoError(t, err, "rows sealed before the removal stay open to the removed key")

	rs, err := e.accounts.ListRecipients(ctx, alice)
	require.NoError(t, err)
	require.Len(t, rs, 1)
	requireKind(t, relay.KindInvalid, e.accounts.RemoveRecipient(ctx, alice, rs[0].ID))

	_, err = e.accounts.SetEncryption(ctx, alice, false)
	require.NoError(t, err)
	after := e.deliver(t, `{"n":3}`)
	assert.False(t, after.Enc)

	page, err := e.feed.ListAfter(ctx, alice, e.src.ID, "", 10)
	require.NoError(t, err)
	require.Len(t, page.Deliveries, 4)
	encs := make([]string, len(page.Deliveries))
	for i, d := range page.Deliveries {
		encs[i] = d.Enc
	}
	assert.Equal(t, []string{"", "age", "age", ""}, encs)
	blob, err := base64.StdEncoding.DecodeString(page.Deliveries[1].Blob)
	require.NoError(t, err)
	assert.Equal(t, sealed.Record, blob)
	assert.Empty(t, page.Deliveries[1].Body)
	assert.Nil(t, page.Deliveries[1].Headers)
	assert.JSONEq(t, `{"n":3}`, page.Deliveries[3].Body)

	require.NoError(t, e.accounts.RemoveRecipient(ctx, alice, rs[0].ID), "the last recipient goes once encryption is off")
}

func TestIngressWithEncryptionAndNoRecipientsIsInternal(t *testing.T) {
	e := newEncEnv(t)
	acct, err := e.store.Accounts().Get(t.Context(), alice)
	require.NoError(t, err)
	acct.Encryption = true
	require.NoError(t, e.store.Accounts().Put(t.Context(), acct))

	_, err = e.ingress.Accept(t.Context(), e.token, "", relay.Record{Method: "POST"})
	require.Error(t, err)
	assert.Empty(t, relay.KindOf(err))
}

func TestSealOpensWithEitherIdentity(t *testing.T) {
	one, two := newIdentity(t), newIdentity(t)
	blob, err := relay.Seal([]byte("hello"), []string{one.Recipient().String(), two.Recipient().String()})
	require.NoError(t, err)
	for _, id := range []age.Identity{one, two} {
		got, err := open(blob, id)
		require.NoError(t, err)
		assert.Equal(t, "hello", string(got))
	}
	_, err = relay.Seal([]byte("x"), nil)
	require.Error(t, err)
	_, err = relay.Seal([]byte("x"), []string{"bogus"})
	require.Error(t, err)
}
