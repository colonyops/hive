package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

// The schedule fields the Chats area reads: a workspace always carries its
// schedules array, and a chat a schedule started names it. Starting a session
// needs tmux, which this harness has no use for otherwise, so the DTO mappers
// are what get pinned.
func TestAgentViewsCarryTheScheduleFields(t *testing.T) {
	session, err := json.Marshal(toAgentSessionView(app.SessionView{ID: "0199bd8f-e6a8-74e2-a10b-3d717cf6abf5", ScheduleID: "weekly"}))
	require.NoError(t, err)
	assert.Contains(t, string(session), `"scheduleId":"weekly"`)

	byHand, err := json.Marshal(toAgentSessionView(app.SessionView{ID: "0199bd8f-f306-713f-a745-79173c8ea605"}))
	require.NoError(t, err)
	assert.Contains(t, string(byHand), `"scheduleId":""`, "a chat a person started names no schedule")

	workspace, err := json.Marshal(toAgentWorkspaceView(app.WorkspaceView{Dir: "demo"}))
	require.NoError(t, err)
	assert.Contains(t, string(workspace), `"schedules":[]`, "never null on the wire")
}

// Array-valued fields must encode as [] on the wire, never null: encoding/json
// marshals a nil Go slice as the JSON literal null, and the frontend calls
// .length on these fields unconditionally. The seeded hive/ workspace declares
// no mcps and its open misses nothing, which is exactly the shape that carried
// null before the DTO mappers normalized it.
func TestAgentWireArraysAreNeverNull(t *testing.T) {
	h := newAgentHarness(t)

	assertNoNullArrays := func(t *testing.T, resp *http.Response, fields ...string) {
		t.Helper()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		raw, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		var doc map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(raw, &doc))
		for _, field := range fields {
			assert.NotEqualf(t, "null", string(doc[field]), "%s must be [] on the wire, got %s in %s", field, doc[field], raw)
		}
		assert.NotContainsf(t, string(raw), `"mcps":null`, "a workspace view carried a null mcps array: %s", raw)
		assert.NotContainsf(t, string(raw), `"schedules":null`, "a workspace view carried a null schedules array: %s", raw)
	}

	resp := h.post(t, AgentWorkspacesPathPrefix+"workspaces", testToken, struct{}{})
	defer func() { _ = resp.Body.Close() }()
	assertNoNullArrays(t, resp, "workspaces")

	resp2 := h.post(t, AgentWorkspacesPathPrefix+"workspaces/open", testToken, map[string]string{"dir": "hive"})
	defer func() { _ = resp2.Body.Close() }()
	assertNoNullArrays(t, resp2, "sessions", "missingMcps", "missingPackages")

	resp3 := h.post(t, AgentWorkspacesPathPrefix+"skills", testToken, struct{}{})
	defer func() { _ = resp3.Body.Close() }()
	assertNoNullArrays(t, resp3, "packages", "skills")
}

// The package catalogue is the workspace editor's read. A fresh install has
// the seeded hive package, and its members are the skills this build ships —
// resolved through the glob, not enumerated anywhere.
func TestAgentSkillPackagesServeTheSeededHivePackage(t *testing.T) {
	h := newAgentHarness(t)

	resp := h.post(t, AgentWorkspacesPathPrefix+"skills", testToken, struct{}{})
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body agentSkillPackagesResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Empty(t, body.Problem)
	require.NotEmpty(t, body.Packages)

	var hive agentSkillPackageView
	for _, pkg := range body.Packages {
		if pkg.Name == "hive" {
			hive = pkg
		}
	}
	require.Equal(t, "hive", hive.Name, "the seeded package must be served")
	require.NotEmpty(t, hive.Members, "hive-* selects the shipped skills")
	for _, member := range hive.Members {
		assert.True(t, member.Shipped)
		assert.True(t, strings.HasPrefix(member.Slug, "hive-"), "member %q does not match the package pattern", member.Slug)
	}

	// The name-space rides along so the editor can say what an enabled name
	// that is not a package actually is (hay-kot/hive-desktop#307).
	require.NotEmpty(t, body.Skills)
	for _, name := range body.Skills {
		assert.Equal(t, []string{"hive"}, name.SelectedBy, "the seeded package selects every shipped skill")
	}
}
