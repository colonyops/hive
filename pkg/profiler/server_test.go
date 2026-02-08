package profiler

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServer_StartAndShutdown(t *testing.T) {
	tests := []struct {
		name string
		port int
	}{
		{
			name: "start and shutdown on port 0 (random)",
			port: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := New(tt.port)

			ctx := context.Background()
			require.NoError(t, server.Start(ctx), "Start() error")

			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			assert.NoError(t, server.Shutdown(shutdownCtx), "Shutdown() error")
		})
	}
}

func TestServer_PprofEndpoints(t *testing.T) {
	server := New(0)

	ctx := context.Background()
	require.NoError(t, server.Start(ctx), "Start() error")
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	baseURL := "http://" + server.Addr()

	tests := []struct {
		name     string
		endpoint string
	}{
		{
			name:     "index",
			endpoint: "/debug/pprof/",
		},
		{
			name:     "cmdline",
			endpoint: "/debug/pprof/cmdline",
		},
		{
			name:     "profile",
			endpoint: "/debug/pprof/profile?seconds=1",
		},
		{
			name:     "symbol",
			endpoint: "/debug/pprof/symbol",
		},
		{
			name:     "trace",
			endpoint: "/debug/pprof/trace?seconds=1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := http.Get(baseURL + tt.endpoint)
			require.NoError(t, err, "GET %s error", tt.endpoint)
			defer func() {
				_ = resp.Body.Close()
			}()

			assert.Equal(t, http.StatusOK, resp.StatusCode, "GET %s status = %v, want %v", tt.endpoint, resp.StatusCode, http.StatusOK)
		})
	}
}
