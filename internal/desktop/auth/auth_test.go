package auth

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/github"
)

// authAPIServer fakes the two GitHub endpoints liveAuth touches: /user for
// validation and the device flow pair.
func authAPIServer(t *testing.T, validTokens map[string]string) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		token, ok := parseBearer(r.Header.Get("Authorization"))
		login, valid := validTokens[token]
		if !ok || !valid {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"Bad credentials"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"login":"` + login + `","name":"Test User"}`))
	})
	mux.HandleFunc("/login/device/code", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"device_code":"dev1","user_code":"AAAA-BBBB","verification_uri":"https://github.com/login/device","expires_in":900,"interval":1}`))
	})
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"granted-token"}`))
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func parseBearer(header string) (string, bool) {
	const prefix = "Bearer "
	if len(header) <= len(prefix) || header[:len(prefix)] != prefix {
		return "", false
	}
	return header[len(prefix):], true
}

func newLiveAuthForTest(t *testing.T, tokens github.TokenStore, validTokens map[string]string, onChange func()) Backend {
	t.Helper()
	server := authAPIServer(t, validTokens)
	client := github.NewClient(github.WithAPIBase(server.URL), github.WithAuthBase(server.URL))
	return NewLiveBackend(client, tokens, onChange)
}

func TestLiveStatusNoToken(t *testing.T) {
	t.Parallel()

	auth := newLiveAuthForTest(t, github.NewMemoryTokenStore(""), nil, nil)
	status := auth.Status(t.Context())
	assert.Equal(t, StateUnauthenticated, status.State)
	assert.Empty(t, status.Message)
}

func TestLiveStatusValidStoredToken(t *testing.T) {
	t.Parallel()

	auth := newLiveAuthForTest(t, github.NewMemoryTokenStore("tok1"), map[string]string{"tok1": "hayden"}, nil)
	status := auth.Status(t.Context())
	assert.Equal(t, StateAuthenticated, status.State)
	assert.Equal(t, "hayden", status.Login)
}

func TestLiveStatusRevokedToken(t *testing.T) {
	t.Parallel()

	auth := newLiveAuthForTest(t, github.NewMemoryTokenStore("revoked"), map[string]string{}, nil)
	status := auth.Status(t.Context())
	assert.Equal(t, StateUnauthenticated, status.State)
	assert.NotEmpty(t, status.Message)
}

func TestLiveAuthSetTokenValidatesAndStores(t *testing.T) {
	t.Parallel()

	store := github.NewMemoryTokenStore("")
	var notified sync.WaitGroup
	notified.Add(1)
	auth := newLiveAuthForTest(t, store, map[string]string{"pat-1": "hayden"}, notified.Done)

	status, err := auth.SetToken(t.Context(), " pat-1 ")
	require.NoError(t, err)
	assert.Equal(t, StateAuthenticated, status.State)
	assert.Equal(t, "hayden", status.Login)

	stored, err := store.Token()
	require.NoError(t, err)
	assert.Equal(t, "pat-1", stored)
	notified.Wait()
}

func TestLiveAuthSetTokenRejected(t *testing.T) {
	t.Parallel()

	store := github.NewMemoryTokenStore("")
	auth := newLiveAuthForTest(t, store, map[string]string{}, nil)

	_, err := auth.SetToken(t.Context(), "bad-token")
	require.ErrorContains(t, err, "rejected")

	stored, err := store.Token()
	require.NoError(t, err)
	assert.Empty(t, stored)
}

func TestLiveAuthDeviceFlowGrantStoresToken(t *testing.T) {
	t.Parallel()

	store := github.NewMemoryTokenStore("")
	changed := make(chan struct{}, 1)
	auth := newLiveAuthForTest(t, store, map[string]string{"granted-token": "hayden"}, func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	})

	info, err := auth.StartDeviceFlow(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "AAAA-BBBB", info.UserCode)
	assert.Equal(t, "https://github.com/login/device", info.VerificationURI)

	select {
	case <-changed:
	case <-time.After(5 * time.Second):
		t.Fatal("device flow grant did not notify")
	}

	stored, err := store.Token()
	require.NoError(t, err)
	assert.Equal(t, "granted-token", stored)
	assert.Equal(t, StateAuthenticated, auth.Status(t.Context()).State)
}

func TestLiveAuthSignOutClearsToken(t *testing.T) {
	t.Parallel()

	store := github.NewMemoryTokenStore("tok1")
	auth := newLiveAuthForTest(t, store, map[string]string{"tok1": "hayden"}, nil)
	require.Equal(t, StateAuthenticated, auth.Status(t.Context()).State)

	require.NoError(t, auth.SignOut())

	stored, err := store.Token()
	require.NoError(t, err)
	assert.Empty(t, stored)
	assert.Equal(t, StateUnauthenticated, auth.Status(t.Context()).State)
}

func TestMockAuthModes(t *testing.T) {
	t.Parallel()

	assert.Equal(t, StateAuthenticated, NewMockBackend(true, nil).Status(t.Context()).State)
	assert.Equal(t, StateUnauthenticated, NewMockBackend(false, nil).Status(t.Context()).State)
}

func TestMockAuthDeviceFlowAutoGrants(t *testing.T) {
	t.Parallel()

	changed := make(chan struct{}, 1)
	auth := NewMockBackend(false, func() { changed <- struct{}{} })

	info, err := auth.StartDeviceFlow(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "7B4C-Q22F", info.UserCode)

	select {
	case <-changed:
	case <-time.After(5 * time.Second):
		t.Fatal("mock device flow did not grant")
	}
	assert.Equal(t, StateAuthenticated, auth.Status(t.Context()).State)
	assert.Equal(t, "hayden", auth.Status(t.Context()).Login)
}

func TestMockAuthSetTokenAndSignOut(t *testing.T) {
	t.Parallel()

	auth := NewMockBackend(false, nil)

	_, err := auth.SetToken(t.Context(), "")
	require.Error(t, err)

	status, err := auth.SetToken(t.Context(), "anything")
	require.NoError(t, err)
	assert.Equal(t, StateAuthenticated, status.State)

	require.NoError(t, auth.SignOut())
	assert.Equal(t, StateUnauthenticated, auth.Status(t.Context()).State)
}
