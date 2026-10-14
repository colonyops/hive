package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoteSessionsRequiresToken(t *testing.T) {
	h := newTerminalHarness(t)
	for _, token := range []string{"", "wrong"} {
		response := h.post(t, TerminalPathPrefix+"remote/sessions", token, nil)
		assert.Equal(t, http.StatusUnauthorized, response.StatusCode)
		require.NoError(t, response.Body.Close())
	}
}

func TestConnectionFileReplacesCredentialPrivately(t *testing.T) {
	path := filepath.Join(t.TempDir(), "connection.json")
	require.NoError(t, os.WriteFile(path, []byte("stale"), 0o644))
	require.NoError(t, WriteConnectionFile(path, "127.0.0.1", 19001, "first"))
	require.NoError(t, WriteConnectionFile(path, "127.0.0.1", 19001, "rotated"))
	stat, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), stat.Mode().Perm())
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var got map[string]string
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, map[string]string{"url": "http://127.0.0.1:19001", "token": "rotated"}, got)
}

func TestRemoteInboxDiscoveryAndFeedIsolation(t *testing.T) {
	h := newTerminalHarness(t)
	for _, path := range []string{"remote/inboxes", "remote/feed"} {
		response := h.post(t, TerminalPathPrefix+path, "wrong", nil)
		assert.Equal(t, http.StatusUnauthorized, response.StatusCode)
		require.NoError(t, response.Body.Close())
	}
	response := h.post(t, TerminalPathPrefix+"remote/inboxes", testToken, nil)
	require.Equal(t, http.StatusOK, response.StatusCode)
	var listing remoteInboxesResponse
	require.NoError(t, json.NewDecoder(response.Body).Decode(&listing))
	require.NoError(t, response.Body.Close())
	require.NotEmpty(t, listing.Inboxes)
	for _, inbox := range listing.Inboxes {
		response = h.post(t, TerminalPathPrefix+"remote/feed", testToken, remoteFeedRequest{ProfileID: inbox.ID, FeedID: "another-profile/feed"})
		assert.Equal(t, http.StatusNotFound, response.StatusCode)
		require.NoError(t, response.Body.Close())
		for _, feed := range inbox.Feeds {
			response = h.post(t, TerminalPathPrefix+"remote/feed", testToken, remoteFeedRequest{ProfileID: inbox.ID, FeedID: feed.ID})
			assert.Equal(t, http.StatusOK, response.StatusCode)
			var items remoteFeedResponse
			require.NoError(t, json.NewDecoder(response.Body).Decode(&items))
			require.NotNil(t, items.Items)
			require.NoError(t, response.Body.Close())
		}
	}
}
