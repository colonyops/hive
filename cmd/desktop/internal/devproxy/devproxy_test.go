package devproxy

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProbeRecognizesOurOwnServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, HealthPath, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"devserver":true}`))
	}))
	defer server.Close()

	assert.Equal(t, StatusRunning, Probe(t.Context(), server.URL))
}

// A trailing slash on the configured base must not produce a double-slashed
// probe path, because settings accepts one and the desktop client tolerates it.
func TestProbeToleratesTrailingSlash(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, HealthPath, r.URL.Path)
		_, _ = w.Write([]byte(`{"devserver":true}`))
	}))
	defer server.Close()

	assert.Equal(t, StatusRunning, Probe(t.Context(), server.URL+"/"))
}

// Anything that answers but is not devserver must be StatusForeign, never
// StatusAbsent: binding over a stranger's listener is impossible, so reporting
// "nothing there" would turn a clear error into a confusing one.
func TestProbeRejectsForeignOccupants(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"wrong marker", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"devserver":false}`))
		}},
		{"not json", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`hello`))
		}},
		{"error status", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.handler)
			defer server.Close()
			assert.Equal(t, StatusForeign, Probe(t.Context(), server.URL))
		})
	}
}

func TestProbeReportsAbsentWhenNothingListens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := server.URL
	server.Close()

	assert.Equal(t, StatusAbsent, Probe(t.Context(), url))
}

// The checked-in config owns the port, so launch.env follows an edit there
// without anyone updating a second copy.
func TestListenFromConfigReadsRepoConfig(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, filepath.Dir(RepoConfigPath)), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, RepoConfigPath), []byte("listen: 127.0.0.1:9001\n"), 0o600))

	assert.Equal(t, "127.0.0.1:9001", ListenFromConfig(root))
}

func TestListenFromConfigFallsBackToDefault(t *testing.T) {
	assert.Equal(t, DefaultListen, ListenFromConfig(t.TempDir()), "no config at all")

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, filepath.Dir(RepoConfigPath)), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, RepoConfigPath), []byte("upstream: https://api.github.com\n"), 0o600))
	assert.Equal(t, DefaultListen, ListenFromConfig(root), "config declaring no listen")
}

func TestBaseURL(t *testing.T) {
	assert.Equal(t, "http://127.0.0.1:7777", BaseURL(DefaultListen))
}

// The repo config is what the singleton actually runs with, so its declared
// port has to be the one every worktree's launch.env is pointed at.
func TestRepoConfigDeclaresTheDefaultListen(t *testing.T) {
	assert.Equal(t, DefaultListen, ListenFromConfig(filepath.Join("..", "..", "..")))
}
