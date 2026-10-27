package driveapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/platform/driveapi"
)

func token() func() string { return func() string { return "tok" } }

func TestHost(t *testing.T) {
	tests := map[string]string{
		"drive.example.com":           "drive.example.com",
		"Drive.Example.COM:8443":      "drive.example.com:8443",
		"https://drive.example.com/":  "drive.example.com",
		"http://127.0.0.1:8080":       "127.0.0.1:8080",
		"localhost:8080":              "localhost:8080",
		"  https://drive.example.com": "drive.example.com",
		"[::1]:8080":                  "[::1]:8080",
	}
	for in, want := range tests {
		got, err := driveapi.Host(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
}

func TestHostRejects(t *testing.T) {
	for _, in := range []string{
		"",
		"ftp://drive.example.com",
		"http://drive.example.com",
		"https://drive.example.com/api",
		"https://user:pw@drive.example.com",
		"https://drive.example.com?x=1",
		"https://",
	} {
		_, err := driveapi.Host(in)
		require.Error(t, err, "%q", in)
		_, err = driveapi.NewClient(in, token())
		assert.Error(t, err, "%q", in)
	}
}

func TestClientSendsBearerTokenAndDecodesErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer tok", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(driveapi.ErrorBody{Kind: "forbidden", Message: "read-only token"})
	}))
	t.Cleanup(srv.Close)
	client, err := driveapi.NewClient(srv.URL, token())
	require.NoError(t, err)

	_, err = client.Me(t.Context())
	var apiErr *driveapi.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusForbidden, apiErr.Status)
	assert.Equal(t, "forbidden", apiErr.Body.Kind)
	assert.Contains(t, err.Error(), "read-only token")
}

func TestClientKeepsStatusWhenErrorBodyIsNotJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "<html>bad gateway</html>", http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)
	client, err := driveapi.NewClient(srv.URL, token())
	require.NoError(t, err)

	_, err = client.List(t.Context(), drive.Shared)
	var apiErr *driveapi.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusBadGateway, apiErr.Status)
	assert.Empty(t, apiErr.Body.Kind)
}

func TestClientEscapesEachPathSegment(t *testing.T) {
	var gotPath, gotEscaped string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotEscaped = r.URL.Path, r.URL.EscapedPath()
		_ = json.NewEncoder(w).Encode(driveapi.Document{Kind: drive.KindMarkdown})
	}))
	t.Cleanup(srv.Close)
	client, err := driveapi.NewClient(srv.URL, token())
	require.NoError(t, err)

	space, err := drive.WorkspaceSpace("my chats")
	require.NoError(t, err)
	_, err = client.Put(t.Context(), space, "my plans/a #1?.md", []byte("x"))
	require.NoError(t, err)
	assert.Equal(t, "/api/v1/workspaces/my chats/docs/my plans/a #1?.md", gotPath)
	assert.Equal(t, "/api/v1/workspaces/my%20chats/docs/my%20plans/a%20%231%3F.md", gotEscaped)

	_, err = client.Share(t.Context(), space, "my plans/a #1?.md", "7d")
	require.NoError(t, err)
	assert.Equal(t, "/api/v1/workspaces/my%20chats/shares/my%20plans/a%20%231%3F.md", gotEscaped)
}

func TestGetReturnsBodyWithKnownMetadata(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))
	t.Cleanup(srv.Close)
	client, err := driveapi.NewClient(srv.URL, token())
	require.NoError(t, err)

	doc, body, err := client.Get(t.Context(), drive.Shared, "a.md")
	require.NoError(t, err)
	assert.Equal(t, "hello", string(body))
	assert.Equal(t, drive.KindMarkdown, doc.Kind)
	assert.Equal(t, int64(5), doc.Size)
	assert.Equal(t, "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", doc.SHA256)
}
