package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
)

// The route takes the session's own token and nothing else: no bearer and the
// frontend's terminal token are both refused, and a session's token answers
// with when that session will be ended.
func TestAgentSessionEndTakesTheSessionsOwnToken(t *testing.T) {
	h := newAgentHarness(t)

	anonymous := h.post(t, app.AgentSessionEndPath, "", nil)
	_ = anonymous.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, anonymous.StatusCode, "no bearer")
	frontend := h.post(t, app.AgentSessionEndPath, testToken, nil)
	_ = frontend.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, frontend.StatusCode, "the terminal token is not a session's")

	rec, err := h.core.Stores.AgentSessions.Create(t.Context(), stores.AgentSessionCreate{
		Workspace: seededWorkspace, Name: "s1", Agent: "claude", AgentSessionID: "a", EndToken: "tok-1",
	})
	require.NoError(t, err)

	before := time.Now().UnixMilli()
	resp := h.post(t, app.AgentSessionEndPath, "tok-1", nil)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	var body agentSessionEndResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, rec.ID, body.Session)
	assert.GreaterOrEqual(t, body.EndsAt, before, "the answer names when the session will be ended")
}
