package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartDeviceFlow(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/login/device/code", r.URL.Path)
		assert.NoError(t, r.ParseForm())
		assert.Equal(t, "client123", r.PostForm.Get("client_id"))
		assert.Equal(t, "repo notifications", r.PostForm.Get("scope"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"device_code": "dev123",
			"user_code": "7B4C-Q22F",
			"verification_uri": "https://github.com/login/device",
			"expires_in": 900,
			"interval": 5
		}`))
	}))
	defer server.Close()

	auth, err := NewClient(WithAuthBase(server.URL)).StartDeviceFlow(t.Context(), "client123", []string{"repo", "notifications"})
	require.NoError(t, err)
	assert.Equal(t, "7B4C-Q22F", auth.UserCode)
	assert.Equal(t, "https://github.com/login/device", auth.VerificationURI)
	assert.Equal(t, 5, auth.Interval)
}

func TestStartDeviceFlowDefaultsInterval(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"device_code":"d","user_code":"u","verification_uri":"v"}`))
	}))
	defer server.Close()

	auth, err := NewClient(WithAuthBase(server.URL)).StartDeviceFlow(t.Context(), "c", nil)
	require.NoError(t, err)
	assert.Equal(t, 5, auth.Interval)
	assert.Equal(t, 900, auth.ExpiresIn)
}

func TestPollDeviceFlowPendingThenGranted(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/login/oauth/access_token", r.URL.Path)
		assert.NoError(t, r.ParseForm())
		assert.Equal(t, "dev123", r.PostForm.Get("device_code"))
		assert.Equal(t, "urn:ietf:params:oauth:grant-type:device_code", r.PostForm.Get("grant_type"))
		w.Header().Set("Content-Type", "application/json")
		if calls.Add(1) < 3 {
			_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"gho_granted"}`))
	}))
	defer server.Close()

	// Interval 0 keeps the test fast; the wait path still executes.
	auth := DeviceAuth{DeviceCode: "dev123", Interval: 0}
	token, err := NewClient(WithAuthBase(server.URL)).PollDeviceFlow(t.Context(), "client123", auth)
	require.NoError(t, err)
	assert.Equal(t, "gho_granted", token)
	assert.Equal(t, int32(3), calls.Load())
}

func TestPollDeviceFlowDenied(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":"access_denied"}`))
	}))
	defer server.Close()

	_, err := NewClient(WithAuthBase(server.URL)).PollDeviceFlow(t.Context(), "c", DeviceAuth{DeviceCode: "d"})
	require.ErrorContains(t, err, "authorization denied")
}

func TestPollDeviceFlowExpired(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":"expired_token"}`))
	}))
	defer server.Close()

	_, err := NewClient(WithAuthBase(server.URL)).PollDeviceFlow(t.Context(), "c", DeviceAuth{DeviceCode: "d"})
	require.ErrorContains(t, err, "code expired")
}

func TestPollDeviceFlowHonorsContextCancel(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := NewClient(WithAuthBase(server.URL)).PollDeviceFlow(ctx, "c", DeviceAuth{DeviceCode: "d", Interval: 1})
	require.ErrorIs(t, err, context.Canceled)
}
