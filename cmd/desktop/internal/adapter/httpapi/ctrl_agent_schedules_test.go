package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/agentws"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
)

// seededWorkspace is the workspace openAgentWorkspaces creates the first time
// a root is made, which is every harness run.
const seededWorkspace = "hive"

// A schedule is written and read back on the workspace view: workspaces/update
// saves it and answers with what it wrote, and workspaces/open lists it joined
// with its run state. There is no schedules list route of its own.
func TestAgentWorkspaceViewCarriesTheSchedulesTheEditorWrote(t *testing.T) {
	h := newAgentHarness(t)

	resp := h.post(t, AgentWorkspacesPathPrefix+"schedules/runs", "", agentScheduleRunsRequest{Workspace: seededWorkspace, ID: "weekly"})
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "the schedules surface rides the terminal bearer gate")

	saved := h.saveWorkspaceSchedules(t, agentWorkspaceScheduleEdit{
		ID: "weekly", Name: "Weekly summary", Cron: "0 9 * * 5", Prompt: "Summarize the week.",
	})
	require.Len(t, saved.Schedules, 1, "the workspace view answers with the schedules it just wrote")
	assert.Equal(t, "weekly", saved.Schedules[0].ID)
	assert.Equal(t, "run", saved.Schedules[0].OnMissed)
	require.NotNil(t, saved.Schedules[0].NextRunAt)
	assert.Nil(t, saved.Schedules[0].LastRun)

	opened := h.post(t, AgentWorkspacesPathPrefix+"workspaces/open", testToken, agentWorkspaceDirRequest{Dir: seededWorkspace})
	defer func() { _ = opened.Body.Close() }()
	require.Equal(t, http.StatusOK, opened.StatusCode)
	var open agentWorkspaceOpenResponse
	require.NoError(t, json.NewDecoder(opened.Body).Decode(&open))
	require.Len(t, open.Workspace.Schedules, 1)
	assert.Equal(t, "Weekly summary", open.Workspace.Schedules[0].Name)

	cleared := h.saveWorkspaceSchedules(t)
	require.NotNil(t, cleared.Schedules, "schedules is never null on the wire")
	assert.Empty(t, cleared.Schedules, "an omitted list deletes every entry")
}

// saveWorkspaceSchedules writes the seeded workspace's schedules through the
// editor's own route and returns the workspace view it answers with.
func (h *terminalHarness) saveWorkspaceSchedules(t *testing.T, schedules ...agentWorkspaceScheduleEdit) agentWorkspaceView {
	t.Helper()
	resp := h.post(t, AgentWorkspacesPathPrefix+"workspaces/update", testToken, agentWorkspaceEditRequest{
		Dir: seededWorkspace, Name: "Hive", Command: "claude" + agentws.PromptTail, Schedules: schedules,
	})
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var view agentWorkspaceView
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&view))
	return view
}

// The frontend sends limit 0 for "however many you think", so the default has
// to be applied somewhere; this pins where.
func TestAgentScheduleRunsAppliesTheDefaultLimit(t *testing.T) {
	h := newAgentHarness(t)

	// A run points at its chat only while the chat exists, so the runs here
	// share one real session record.
	chat, err := h.core.Stores.AgentSessions.Create(t.Context(), stores.AgentSessionCreate{
		Workspace: seededWorkspace, Name: "s1", Agent: "claude", AgentSessionID: "a",
	})
	require.NoError(t, err)
	const inserted = 55
	for i := range inserted {
		_, err := h.core.Stores.Schedules.InsertRun(t.Context(), stores.ScheduleRun{
			Workspace: seededWorkspace, ScheduleID: "weekly", ScheduleName: "Weekly summary",
			ScheduledFor: int64(i), StartedAt: int64(i),
			Reason: "due", Status: "launched", SessionID: chat.ID,
		})
		require.NoError(t, err)
	}

	resp := h.post(t, AgentWorkspacesPathPrefix+"schedules/runs", testToken, agentScheduleRunsRequest{
		Workspace: seededWorkspace, ID: "weekly", Limit: 0,
	})
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body agentScheduleRunsResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Len(t, body.Runs, 50)
	assert.Equal(t, int64(inserted-1), body.Runs[0].StartedAt, "newest first")
	require.NotNil(t, body.Runs[0].SessionID)

	noID := h.post(t, AgentWorkspacesPathPrefix+"schedules/runs", testToken, agentScheduleRunsRequest{
		Workspace: seededWorkspace,
	})
	_ = noID.Body.Close()
	assert.Equal(t, http.StatusUnprocessableEntity, noID.StatusCode, "history is per schedule; there is no workspace-wide listing")
}
