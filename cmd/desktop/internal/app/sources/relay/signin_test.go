package relay

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/sourcehttp"
	"github.com/colonyops/hive/internal/platform/credentials"
)

const (
	testCode     = "12345678"
	testPassword = "hunter2"
	deviceToken  = "rk_device"
)

type authRequest struct {
	Path          string
	Authorization string
	Body          map[string]string
}

// fakeSignIn serves the relay's three sign-in routes. status, when set,
// answers every route with it and message.
type fakeSignIn struct {
	*httptest.Server

	mu       sync.Mutex
	requests []authRequest
	status   int
	message  string
}

func newFakeSignIn(t *testing.T) *fakeSignIn {
	t.Helper()
	f := &fakeSignIn{}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeSignIn) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var body map[string]string
	_ = json.NewDecoder(r.Body).Decode(&body)
	f.requests = append(f.requests, authRequest{Path: r.URL.Path, Authorization: r.Header.Get("Authorization"), Body: body})

	if f.status != 0 {
		if f.status == http.StatusTooManyRequests {
			w.Header().Set("Retry-After", "30")
		}
		w.WriteHeader(f.status)
		_ = json.NewEncoder(w).Encode(map[string]string{"kind": "refused", "message": f.message})
		return
	}
	signedIn := func(ok bool) {
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"kind":"unauthorized","message":"nope"}`))
			return
		}
		// The relay answers with the normalized email, not the typed one.
		_ = json.NewEncoder(w).Encode(SignedIn{
			Token: deviceToken, ID: "tok1", Email: strings.ToLower(body["email"]), Kind: "device", Device: body["device"],
		})
	}
	switch r.URL.Path {
	case "/v1/auth/start":
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{}`))
	case "/v1/auth/verify":
		signedIn(body["code"] == testCode)
	case "/v1/auth/login":
		signedIn(body["password"] == testPassword)
	case "/v1/recipients":
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(RecipientView{ID: "rcp1", Recipient: body["recipient"], Device: body["device"]})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *fakeSignIn) last(t *testing.T) authRequest {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	require.NotEmpty(t, f.requests)
	return f.requests[len(f.requests)-1]
}

func (f *fakeSignIn) find(t *testing.T, path string) authRequest {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, req := range f.requests {
		if req.Path == path {
			return req
		}
	}
	require.Failf(t, "no request", "no request to %s", path)
	return authRequest{}
}

func assertNothingStored(t *testing.T, f *authFixture) {
	t.Helper()
	refs, err := f.creds.List()
	require.NoError(t, err)
	assert.Empty(t, refs)
	assert.Empty(t, f.changed)
}

func assertDeviceTokenStored(t *testing.T, f *authFixture, relayURL string, instance Instance) {
	t.Helper()
	host := strings.TrimPrefix(relayURL, "http://")
	assert.Equal(t, Instance{Account: host + "-me@example.com", URL: relayURL, Email: "me@example.com"}, instance)

	ref := credentials.Ref{Provider: Provider, Account: instance.Account}
	token, err := f.creds.Get(ref)
	require.NoError(t, err)
	assert.Equal(t, deviceToken, token)
	binding, err := f.instances.Get(ref)
	require.NoError(t, err)
	assert.Equal(t, Binding{URL: relayURL, Email: "me@example.com"}, binding)
	assert.Equal(t, []credentials.Ref{ref}, f.changed)

	raw, err := f.creds.Get(ageRef(instance.Account))
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(raw, "AGE-SECRET-KEY-1"))
}

func TestStartEmailAsksForACodeAndStoresNothing(t *testing.T) {
	t.Parallel()
	relay := newFakeSignIn(t)
	f := newAuthenticator(t)

	require.NoError(t, f.auth.StartEmail(t.Context(), relay.URL, " me@example.com "))

	req := relay.last(t)
	assert.Equal(t, "/v1/auth/start", req.Path)
	assert.Equal(t, map[string]string{"email": "me@example.com"}, req.Body)
	assert.Empty(t, req.Authorization)
	assertNothingStored(t, f)
}

func TestStartEmailForbiddenCarriesTheRelayMessage(t *testing.T) {
	t.Parallel()
	relay := newFakeSignIn(t)
	relay.status, relay.message = http.StatusForbidden, "email is not allowed on this relay"
	f := newAuthenticator(t)

	err := f.auth.StartEmail(t.Context(), relay.URL, "me@example.com")
	require.ErrorIs(t, err, ErrForbidden)
	require.EqualError(t, err, "email is not allowed on this relay")
	assertNothingStored(t, f)
}

func TestStartEmailRateLimited(t *testing.T) {
	t.Parallel()
	relay := newFakeSignIn(t)
	relay.status = http.StatusTooManyRequests

	err := newAuthenticator(t).auth.StartEmail(t.Context(), relay.URL, "me@example.com")
	require.ErrorIs(t, err, sourcehttp.ErrRateLimited)
}

func TestVerifyEmailStoresTheDeviceTokenAndBinding(t *testing.T) {
	t.Parallel()
	relay := newFakeSignIn(t)
	f := newAuthenticator(t)

	instance, err := f.auth.VerifyEmail(t.Context(), relay.URL, "Me@example.com", " "+testCode+" ")
	require.NoError(t, err)
	assertDeviceTokenStored(t, f, relay.URL, instance)

	req := relay.find(t, "/v1/auth/verify")
	assert.Empty(t, req.Authorization)
	assert.Equal(t, testCode, req.Body["code"])
	assert.Equal(t, deviceName(), req.Body["device"])
	assert.NotEmpty(t, req.Body["device"])
}

func TestVerifyEmailWrongCodeStoresNothing(t *testing.T) {
	t.Parallel()
	relay := newFakeSignIn(t)
	relay.status, relay.message = http.StatusUnauthorized, "invalid code; 2 attempts left"
	f := newAuthenticator(t)

	_, err := f.auth.VerifyEmail(t.Context(), relay.URL, "me@example.com", "00000000")
	require.ErrorIs(t, err, sourcehttp.ErrUnauthorized)
	require.EqualError(t, err, "invalid code; 2 attempts left")
	assertNothingStored(t, f)
}

func TestLoginStoresTheDeviceTokenAndBinding(t *testing.T) {
	t.Parallel()
	relay := newFakeSignIn(t)
	f := newAuthenticator(t)

	instance, err := f.auth.Login(t.Context(), relay.URL, "me@example.com", testPassword)
	require.NoError(t, err)
	assertDeviceTokenStored(t, f, relay.URL, instance)

	req := relay.find(t, "/v1/auth/login")
	assert.Empty(t, req.Authorization)
	assert.Equal(t, testPassword, req.Body["password"])
	assert.Equal(t, deviceName(), req.Body["device"])

	registered := relay.find(t, "/v1/recipients")
	assert.Equal(t, "Bearer "+deviceToken, registered.Authorization)
	assert.True(t, strings.HasPrefix(registered.Body["recipient"], "age1"))
}

func TestLoginWrongPasswordStoresNothing(t *testing.T) {
	t.Parallel()
	relay := newFakeSignIn(t)
	f := newAuthenticator(t)

	_, err := f.auth.Login(t.Context(), relay.URL, "me@example.com", "wrong")
	require.ErrorIs(t, err, sourcehttp.ErrUnauthorized)
	assertNothingStored(t, f)
}

func TestLoginForbiddenCarriesTheRelayMessage(t *testing.T) {
	t.Parallel()
	relay := newFakeSignIn(t)
	relay.status, relay.message = http.StatusForbidden, "password sign-in is not enabled for this email"
	f := newAuthenticator(t)

	_, err := f.auth.Login(t.Context(), relay.URL, "me@example.com", testPassword)
	require.ErrorIs(t, err, ErrForbidden)
	require.EqualError(t, err, "password sign-in is not enabled for this email")
	assertNothingStored(t, f)
}

func TestSignInRequiresHTTPSAndAnEmail(t *testing.T) {
	t.Parallel()
	auth := newAuthenticator(t).auth
	_, err := auth.Login(t.Context(), "http://relay.example.com", "me@example.com", testPassword)
	require.ErrorContains(t, err, "must use https")
	require.ErrorContains(t, auth.StartEmail(t.Context(), "https://relay.example.com", "  "), "email is required")
}

func TestDeviceNameFitsTheRelayLimit(t *testing.T) {
	t.Parallel()
	name := deviceName()
	assert.NotEmpty(t, name)
	assert.LessOrEqual(t, len([]rune(name)), maxDeviceName)
}
