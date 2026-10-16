package httpapi

import (
	"encoding/json"
	"net/http"
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

func TestRemoteCodeOperationsAuthenticateBeforeReadingInput(t *testing.T) {
	h := newTerminalHarness(t)
	for _, path := range []string{"session-options", "create-session", "job", "session-statuses", "session-git-status", "session-pull-request"} {
		for _, token := range []string{"", "wrong"} {
			response := h.post(t, TerminalPathPrefix+"remote/"+path, token, nil)
			assert.Equal(t, http.StatusUnauthorized, response.StatusCode, path)
			require.NoError(t, response.Body.Close())
		}
	}
}

func TestRemoteCodeJobOutcome(t *testing.T) {
	h := newTerminalHarness(t)
	id := h.core.Jobs.Begin(t.Context(), "Create session", "test", "remote-only")
	h.core.Jobs.Fail(t.Context(), id, "clone failed")
	response := h.post(t, TerminalPathPrefix+"remote/job", testToken, remoteJobRequest{ID: id})
	require.Equal(t, http.StatusOK, response.StatusCode)
	var job struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&job))
	require.NoError(t, response.Body.Close())
	assert.Equal(t, "failed", job.Status)
	assert.Equal(t, "clone failed", job.Error)
	response = h.post(t, TerminalPathPrefix+"remote/job", testToken, remoteJobRequest{ID: id + 1})
	assert.Equal(t, http.StatusNotFound, response.StatusCode)
	require.NoError(t, response.Body.Close())
	response = h.post(t, TerminalPathPrefix+"remote/create-session", testToken, remoteCreateSessionRequest{Name: "no-repository"})
	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
	require.NoError(t, response.Body.Close())
}
