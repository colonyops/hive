package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sessions/all rides the same terminal bearer token and CORS policy as every
// other agent-workspace route (ADR a-workspace-declares-its-own-authority) — it sits in the same operations
// table, but this proves the wiring rather than assuming it.
func TestAgentSessionsAllRequiresTheBearerToken(t *testing.T) {
	h := newAgentHarness(t)

	resp := h.post(t, AgentWorkspacesPathPrefix+"sessions/all", "", struct{}{})
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "no Authorization header is rejected")

	resp = h.post(t, AgentWorkspacesPathPrefix+"sessions/all", "wrong-token", struct{}{})
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "a wrong token is rejected")

	resp = h.post(t, AgentWorkspacesPathPrefix+"sessions/all", testToken, struct{}{})
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode, "the right token is served")
}

// A build with no sessions yet must answer an empty array, never null: the
// frontend calls .length on it unconditionally, the same rule
// TestAgentWireArraysAreNeverNull proves for the other list responses.
func TestAgentSessionsAllListsOverTheWireAsAnEmptyArray(t *testing.T) {
	h := newAgentHarness(t)

	resp := h.post(t, AgentWorkspacesPathPrefix+"sessions/all", testToken, struct{}{})
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body agentSessionsAllResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.NotNil(t, body.Sessions, "sessions must be [] on the wire, never null")
	assert.Empty(t, body.Sessions)
}
