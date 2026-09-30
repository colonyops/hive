package github_test

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/credentials"
	ghsource "github.com/colonyops/hive/cmd/desktop/internal/app/sources/github"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/github/ghclient"
)

// connectAPIServer fakes the two GitHub endpoints the live connection
// touches: /user for validation and the device flow pair.
func connectAPIServer(t *testing.T, validTokens map[string]string) *httptest.Server {
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

func newLiveConnectionForTest(t *testing.T, creds credentials.Store, validTokens map[string]string, onChange func()) ghsource.Connection {
	t.Helper()
	server := connectAPIServer(t, validTokens)
	client := ghclient.NewClient(ghclient.WithAPIBase(server.URL), ghclient.WithAuthBase(server.URL))
	return ghsource.NewLiveConnection(client, creds, onChange)
}

// seededCreds is a credential store already holding one GitHub account.
// Credentials are keyed by login, so "a token is stored" also has to say
// whose it is.
func seededCreds(t *testing.T, login, token string) credentials.Store {
	t.Helper()
	creds := credentials.NewMemoryStore()
	if token != "" {
		require.NoError(t, creds.Set(credentials.Ref{Provider: ghsource.Provider, Account: login}, token))
	}
	return creds
}

// storedToken reads the one stored GitHub token directly, bypassing the
// environment override so a test can tell "stored" from "overridden".
func storedToken(t *testing.T, creds credentials.Store) string {
	t.Helper()
	refs, err := credentials.ListProvider(creds, ghsource.Provider)
	require.NoError(t, err)
	if len(refs) == 0 {
		return ""
	}
	value, err := creds.Get(refs[0])
	require.NoError(t, err)
	return value
}

// deniedDeviceFlowServer fakes a device flow whose token endpoint always
// denies authorization, so pollFlow's failure path runs.
func deniedDeviceFlowServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/login/device/code", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"device_code":"dev1","user_code":"AAAA-BBBB","verification_uri":"https://github.com/login/device","expires_in":900,"interval":1}`))
	})
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":"access_denied"}`))
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func TestLiveConnectionStatusNoToken(t *testing.T) {
	t.Parallel()

	conn := newLiveConnectionForTest(t, credentials.NewMemoryStore(), nil, nil)
	status := conn.Status(t.Context())
	assert.Equal(t, ghsource.StateDisconnected, status.State)
	assert.Empty(t, status.Message)
}

func TestLiveConnectionStatusValidStoredToken(t *testing.T) {
	t.Parallel()

	conn := newLiveConnectionForTest(t, seededCreds(t, "octocat", "tok1"), map[string]string{"tok1": "octocat"}, nil)
	status := conn.Status(t.Context())
	assert.Equal(t, ghsource.StateConnected, status.State)
	assert.Equal(t, "octocat", status.Login)
}

func TestLiveConnectionStatusRevokedToken(t *testing.T) {
	t.Parallel()

	conn := newLiveConnectionForTest(t, seededCreds(t, "octocat", "revoked"), map[string]string{}, nil)
	status := conn.Status(t.Context())
	assert.Equal(t, ghsource.StateDisconnected, status.State)
	assert.NotEmpty(t, status.Message)
}

func TestLiveConnectionSetTokenValidatesAndStores(t *testing.T) {
	t.Parallel()

	store := credentials.NewMemoryStore()
	var notified sync.WaitGroup
	notified.Add(1)
	conn := newLiveConnectionForTest(t, store, map[string]string{"pat-1": "octocat"}, notified.Done)

	status, err := conn.SetToken(t.Context(), " pat-1 ")
	require.NoError(t, err)
	assert.Equal(t, ghsource.StateConnected, status.State)
	assert.Equal(t, "octocat", status.Login)

	assert.Equal(t, "pat-1", storedToken(t, store))
	notified.Wait()
}

func TestLiveConnectionSetTokenRejected(t *testing.T) {
	t.Parallel()

	store := credentials.NewMemoryStore()
	conn := newLiveConnectionForTest(t, store, map[string]string{}, nil)

	_, err := conn.SetToken(t.Context(), "bad-token")
	require.ErrorContains(t, err, "rejected")

	assert.Empty(t, storedToken(t, store))
}

func TestLiveConnectionDeviceFlowGrantStoresToken(t *testing.T) {
	t.Parallel()

	store := credentials.NewMemoryStore()
	changed := make(chan struct{}, 1)
	conn := newLiveConnectionForTest(t, store, map[string]string{"granted-token": "octocat"}, func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	})

	info, err := conn.StartDeviceFlow(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "AAAA-BBBB", info.UserCode)
	assert.Equal(t, "https://github.com/login/device", info.VerificationURI)

	select {
	case <-changed:
	case <-time.After(5 * time.Second):
		t.Fatal("device flow grant did not notify")
	}

	assert.Equal(t, "granted-token", storedToken(t, store))
	assert.Equal(t, ghsource.StateConnected, conn.Status(t.Context()).State)
}

func TestLiveConnectionDeviceFlowDeniedSurfacesMessage(t *testing.T) {
	t.Parallel()

	server := deniedDeviceFlowServer(t)
	client := ghclient.NewClient(ghclient.WithAPIBase(server.URL), ghclient.WithAuthBase(server.URL))
	changed := make(chan struct{}, 1)
	conn := ghsource.NewLiveConnection(client, credentials.NewMemoryStore(), func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	})

	_, err := conn.StartDeviceFlow(t.Context())
	require.NoError(t, err)

	select {
	case <-changed:
	case <-time.After(5 * time.Second):
		t.Fatal("device flow denial did not notify")
	}

	status := conn.Status(t.Context())
	assert.Equal(t, ghsource.StateDisconnected, status.State)
	assert.Contains(t, status.Message, "authorization denied")

	// Starting a fresh attempt must clear the stale failure message right
	// away, before the new attempt's own outcome (still ~1s out) arrives.
	_, err = conn.StartDeviceFlow(t.Context())
	require.NoError(t, err)
	assert.Empty(t, conn.Status(t.Context()).Message)
}

func TestLiveConnectionDisconnectWithEnvOverrideExplains(t *testing.T) {
	envName := credentials.EnvOverrideName(ghsource.Provider)
	t.Setenv(envName, "env-token")

	store := seededCreds(t, "octocat", "tok1")
	conn := newLiveConnectionForTest(t, store, map[string]string{"tok1": "octocat", "env-token": "octocat"}, nil)
	require.Equal(t, ghsource.StateConnected, conn.Status(t.Context()).State)

	require.NoError(t, conn.Disconnect())

	status := conn.Status(t.Context())
	assert.Equal(t, ghsource.StateDisconnected, status.State)
	assert.Contains(t, status.Message, envName)
}

func TestLiveConnectionDisconnectClearsToken(t *testing.T) {
	t.Parallel()

	store := seededCreds(t, "octocat", "tok1")
	conn := newLiveConnectionForTest(t, store, map[string]string{"tok1": "octocat"}, nil)
	require.Equal(t, ghsource.StateConnected, conn.Status(t.Context()).State)

	require.NoError(t, conn.Disconnect())

	assert.Empty(t, storedToken(t, store))
	assert.Equal(t, ghsource.StateDisconnected, conn.Status(t.Context()).State)
}

func TestMockConnectionModes(t *testing.T) {
	t.Parallel()

	assert.Equal(t, ghsource.StateConnected, ghsource.NewMockConnection(true, credentials.NewMemoryStore(), nil).Status(t.Context()).State)
	assert.Equal(t, ghsource.StateDisconnected, ghsource.NewMockConnection(false, credentials.NewMemoryStore(), nil).Status(t.Context()).State)
}

func TestMockConnectionDeviceFlowAutoGrants(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		changed := make(chan struct{}, 1)
		conn := ghsource.NewMockConnection(false, credentials.NewMemoryStore(), func() { changed <- struct{}{} })

		info, err := conn.StartDeviceFlow(t.Context())
		require.NoError(t, err)
		assert.Equal(t, "7B4C-Q22F", info.UserCode)

		select {
		case <-changed:
		case <-time.After(5 * time.Second):
			t.Fatal("mock device flow did not grant")
		}
		assert.Equal(t, ghsource.StateConnected, conn.Status(t.Context()).State)
		assert.Equal(t, "octocat", conn.Status(t.Context()).Login)
	})
}

// A mock connection that only flipped a status flag would leave the credential
// store empty, and everything that resolves an account off it — a source
// node's credential, seeding a workspace — would fail in mock modes only.
// That is a whole class of e2e failure that never reproduces live, so the
// mock writes and clears a credential exactly as the live connection does.
func TestMockConnectionConnectsAndDisconnectsTheCredential(t *testing.T) {
	t.Parallel()

	t.Run("connected modes start connected", func(t *testing.T) {
		t.Parallel()
		creds := credentials.NewMemoryStore()
		ghsource.NewMockConnection(true, creds, nil)
		assert.Equal(t, "mock-token", storedToken(t, creds))
	})

	t.Run("onboarding starts disconnected and connects on grant", func(t *testing.T) {
		t.Parallel()
		synctest.Test(t, func(t *testing.T) {
			creds := credentials.NewMemoryStore()
			changed := make(chan struct{}, 1)
			conn := ghsource.NewMockConnection(false, creds, func() { changed <- struct{}{} })
			assert.Empty(t, storedToken(t, creds))

			_, err := conn.StartDeviceFlow(t.Context())
			require.NoError(t, err)
			select {
			case <-changed:
			case <-time.After(5 * time.Second):
				t.Fatal("mock device flow did not grant")
			}
			assert.Equal(t, "mock-token", storedToken(t, creds))
		})
	})

	t.Run("disconnect clears it", func(t *testing.T) {
		t.Parallel()
		creds := credentials.NewMemoryStore()
		conn := ghsource.NewMockConnection(true, creds, nil)
		require.NoError(t, conn.Disconnect())
		assert.Empty(t, storedToken(t, creds))
	})
}

func TestMockConnectionCancelPreventsLateGrant(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		changed := make(chan struct{}, 1)
		conn := ghsource.NewMockConnection(false, credentials.NewMemoryStore(), func() {
			select {
			case changed <- struct{}{}:
			default:
			}
		})

		_, err := conn.StartDeviceFlow(t.Context())
		require.NoError(t, err)
		conn.CancelDeviceFlow()

		time.Sleep(2 * time.Second) // fake clock: past the mock grant delay (1.5s)

		assert.Equal(t, ghsource.StateDisconnected, conn.Status(t.Context()).State)
		select {
		case <-changed:
			t.Fatal("onChange fired after cancel")
		default:
		}
	})
}

func TestMockConnectionSetTokenAndDisconnect(t *testing.T) {
	t.Parallel()

	conn := ghsource.NewMockConnection(false, credentials.NewMemoryStore(), nil)

	_, err := conn.SetToken(t.Context(), "")
	require.Error(t, err)

	status, err := conn.SetToken(t.Context(), "anything")
	require.NoError(t, err)
	assert.Equal(t, ghsource.StateConnected, status.State)

	require.NoError(t, conn.Disconnect())
	assert.Equal(t, ghsource.StateDisconnected, conn.Status(t.Context()).State)
}

// The development override (ADR devserver-github-proxy) is only useful if it actually reaches
// the client both callers share, so assert the request lands on the
// substitute base rather than api.github.com.
