package app

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/relay"
	"github.com/colonyops/hive/internal/platform/credentials"
)

func relayServiceAnswering(t *testing.T, status int, message string) (*RelayService, string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"kind":"x","message":"` + message + `"}`))
	}))
	t.Cleanup(srv.Close)
	instances := relay.NewInstanceStore(filepath.Join(t.TempDir(), "relay-instances.json"))
	auth := relay.NewAuthenticator(credentials.NewMemoryStore(), instances, "test", zerolog.Nop(), nil)
	return newRelayService(auth, nil), srv.URL
}

func TestRelaySignInErrorKinds(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		status int
		want   Kind
	}{
		{name: "forbidden", status: http.StatusForbidden, want: KindUnauthenticated},
		{name: "wrong code", status: http.StatusUnauthorized, want: KindUnauthenticated},
		{name: "rate limited", status: http.StatusTooManyRequests, want: KindUnavailable},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc, url := relayServiceAnswering(t, tc.status, "relay says no")

			err := svc.StartEmail(t.Context(), url, "me@example.com")
			require.Error(t, err)
			assert.Equal(t, tc.want, KindOf(err))

			_, err = svc.VerifyEmail(t.Context(), url, "me@example.com", "12345678")
			assert.Equal(t, tc.want, KindOf(err))

			_, err = svc.Login(t.Context(), url, "me@example.com", "pw")
			assert.Equal(t, tc.want, KindOf(err))
		})
	}
}

// The relay's message is what tells the user why, so it must reach the wire.
func TestRelaySignInKeepsTheRelayMessage(t *testing.T) {
	t.Parallel()
	svc, url := relayServiceAnswering(t, http.StatusUnauthorized, "invalid code; 2 attempts left")

	_, err := svc.VerifyEmail(t.Context(), url, "me@example.com", "00000000")
	require.ErrorContains(t, err, "invalid code; 2 attempts left")
}

func TestRelaySignInBadURLIsUnauthenticated(t *testing.T) {
	t.Parallel()
	svc, _ := relayServiceAnswering(t, http.StatusOK, "")

	_, err := svc.Login(t.Context(), "http://relay.example.com", "me@example.com", "pw")
	require.Error(t, err)
	assert.Equal(t, KindUnauthenticated, KindOf(err))
}
