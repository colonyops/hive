package relay

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/platform/credentials"
)

type authFixture struct {
	auth      *Authenticator
	creds     credentials.Store
	instances *InstanceStore
	changed   []credentials.Ref
}

func newAuthenticator(t *testing.T) *authFixture {
	t.Helper()
	f := &authFixture{
		creds:     credentials.NewMemoryStore(),
		instances: NewInstanceStore(filepath.Join(t.TempDir(), "relay-instances.json")),
	}
	f.auth = NewAuthenticator(f.creds, f.instances, "1.2.3", zerolog.Nop(), func(ref credentials.Ref) { f.changed = append(f.changed, ref) })
	return f
}

func TestConnectWithKeyStoresTheKeyAndBinding(t *testing.T) {
	t.Parallel()
	relay := newFakeRelay(t)
	f := newAuthenticator(t)
	auth, creds, instances := f.auth, f.creds, f.instances

	instance, err := auth.ConnectWithKey(t.Context(), relay.URL+"/", " "+testKey+" ")
	require.NoError(t, err)

	host := strings.TrimPrefix(relay.URL, "http://")
	assert.Equal(t, Instance{Account: host + "-me@example.com", URL: relay.URL, Email: "me@example.com"}, instance)

	ref := credentials.Ref{Provider: Provider, Account: instance.Account}
	key, err := creds.Get(ref)
	require.NoError(t, err)
	assert.Equal(t, testKey, key)
	binding, err := instances.Get(ref)
	require.NoError(t, err)
	assert.Equal(t, Binding{URL: relay.URL, Email: "me@example.com"}, binding)
	assert.Equal(t, []credentials.Ref{ref}, f.changed)

	accounts, err := auth.Accounts(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []Instance{instance}, accounts)
}

func TestConnectWithKeyRejectedStoresNothing(t *testing.T) {
	t.Parallel()
	relay := newFakeRelay(t)
	relay.status = http.StatusUnauthorized
	f := newAuthenticator(t)
	auth, creds := f.auth, f.creds

	_, err := auth.ConnectWithKey(t.Context(), relay.URL, testKey)
	require.ErrorContains(t, err, "rejected the API key")

	refs, err := creds.List()
	require.NoError(t, err)
	assert.Empty(t, refs)
	assert.Empty(t, f.changed)
}

func TestConnectWithKeyRequiresHTTPSOffLoopback(t *testing.T) {
	t.Parallel()
	auth := newAuthenticator(t).auth
	_, err := auth.ConnectWithKey(t.Context(), "http://relay.example.com", testKey)
	require.ErrorContains(t, err, "must use https")
	_, err = auth.ConnectWithKey(t.Context(), "https://user:pw@relay.example.com", testKey)
	require.ErrorContains(t, err, "userinfo")
}

func TestDisconnectRemovesBothAndIsIdempotent(t *testing.T) {
	t.Parallel()
	relay := newFakeRelay(t)
	f := newAuthenticator(t)
	auth, creds, instances := f.auth, f.creds, f.instances
	instance, err := auth.ConnectWithKey(t.Context(), relay.URL, testKey)
	require.NoError(t, err)

	require.NoError(t, auth.Disconnect(t.Context(), instance.Account))
	require.NoError(t, auth.Disconnect(t.Context(), instance.Account))

	ref := credentials.Ref{Provider: Provider, Account: instance.Account}
	_, err = creds.Get(ref)
	require.ErrorIs(t, err, credentials.ErrNotFound)
	binding, err := instances.Get(ref)
	require.NoError(t, err)
	assert.Empty(t, binding.URL)
}

func TestMethodsReturnsTheRelayAnswer(t *testing.T) {
	t.Parallel()
	relay := newFakeRelay(t)
	auth := newAuthenticator(t).auth
	methods, err := auth.Methods(t.Context(), relay.URL)
	require.NoError(t, err)
	assert.Equal(t, Methods{APIKey: true}, methods)
}

func TestAccountIDKeepsTheRefValid(t *testing.T) {
	t.Parallel()
	ref := credentials.Ref{Provider: Provider, Account: accountID("relay.example.com", "a/b@example.com")}
	require.NoError(t, ref.Validate())
	assert.Equal(t, "relay.example.com-a_b@example.com", ref.Account)
}

func storedIdentity(t *testing.T, creds credentials.Store, account string) *age.X25519Identity {
	t.Helper()
	raw, err := creds.Get(ageRef(account))
	require.NoError(t, err)
	identity, err := age.ParseX25519Identity(raw)
	require.NoError(t, err)
	return identity
}

func TestConnectRegistersThisDeviceAsARecipient(t *testing.T) {
	t.Parallel()
	relay := newFakeRelay(t)
	f := newAuthenticator(t)

	instance, err := f.auth.ConnectWithKey(t.Context(), relay.URL, testKey)
	require.NoError(t, err)
	identity := storedIdentity(t, f.creds, instance.Account)
	require.Len(t, relay.recipients, 1)
	assert.Equal(t, identity.Recipient().String(), relay.recipients[0].Recipient)
	assert.Equal(t, deviceName(), relay.recipients[0].Device)

	_, err = f.auth.ConnectWithKey(t.Context(), relay.URL, testKey)
	require.NoError(t, err)
	assert.Equal(t, identity.String(), storedIdentity(t, f.creds, instance.Account).String(), "a reconnect keeps the identity")
	assert.Len(t, relay.recipients, 1)
}

func TestConnectWhoseRecipientIsRefusedStoresNothing(t *testing.T) {
	t.Parallel()
	relay := newFakeRelay(t)
	relay.recipientStatus = http.StatusInternalServerError
	f := newAuthenticator(t)

	_, err := f.auth.ConnectWithKey(t.Context(), relay.URL, testKey)
	require.ErrorContains(t, err, "encryption key")

	refs, err := f.creds.List()
	require.NoError(t, err)
	assert.Empty(t, refs)
	host := strings.TrimPrefix(relay.URL, "http://")
	binding, err := f.instances.Get(credentials.Ref{Provider: Provider, Account: host + "-me@example.com"})
	require.NoError(t, err)
	assert.Empty(t, binding.URL)
	assert.Empty(t, f.changed)
}

func TestReconnectWhoseRecipientIsRefusedKeepsThePreviousConnection(t *testing.T) {
	t.Parallel()
	relay := newFakeRelay(t)
	f := newAuthenticator(t)
	instance, err := f.auth.ConnectWithKey(t.Context(), relay.URL, testKey)
	require.NoError(t, err)
	identity := storedIdentity(t, f.creds, instance.Account)

	relay.recipientStatus = http.StatusInternalServerError
	_, err = f.auth.ConnectWithKey(t.Context(), relay.URL, testKey)
	require.Error(t, err)

	ref := credentials.Ref{Provider: Provider, Account: instance.Account}
	key, err := f.creds.Get(ref)
	require.NoError(t, err)
	assert.Equal(t, testKey, key)
	assert.Equal(t, identity.String(), storedIdentity(t, f.creds, instance.Account).String())
	binding, err := f.instances.Get(ref)
	require.NoError(t, err)
	assert.Equal(t, relay.URL, binding.URL)
}

func TestDisconnectRemovesTheRecipientAndIdentity(t *testing.T) {
	t.Parallel()
	relay := newFakeRelay(t)
	f := newAuthenticator(t)
	instance, err := f.auth.ConnectWithKey(t.Context(), relay.URL, testKey)
	require.NoError(t, err)
	relay.recipients = append(relay.recipients, RecipientView{ID: "other", Recipient: newIdentity(t).Recipient().String()})

	require.NoError(t, f.auth.Disconnect(t.Context(), instance.Account))

	assert.Equal(t, []string{"rcp1"}, relay.removedRecipient)
	_, err = f.creds.Get(ageRef(instance.Account))
	require.ErrorIs(t, err, credentials.ErrNotFound)
	_, err = f.creds.Get(credentials.Ref{Provider: Provider, Account: instance.Account})
	require.ErrorIs(t, err, credentials.ErrNotFound)
}

func TestDisconnectSucceedsWhenTheRelayIsGone(t *testing.T) {
	t.Parallel()
	relay := newFakeRelay(t)
	f := newAuthenticator(t)
	instance, err := f.auth.ConnectWithKey(t.Context(), relay.URL, testKey)
	require.NoError(t, err)
	relay.Close()

	require.NoError(t, f.auth.Disconnect(t.Context(), instance.Account))
	refs, err := f.creds.List()
	require.NoError(t, err)
	assert.Empty(t, refs)
}
