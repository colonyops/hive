package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/canvas"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
)

func TestAgentCanvasReadsOverTheWire(t *testing.T) {
	h := newAgentHarness(t)

	rec, err := h.core.Stores.AgentSessions.Create(t.Context(), stores.AgentSessionCreate{
		Workspace: "demo", Name: "chat", Agent: "claude",
	})
	require.NoError(t, err)

	resp := h.post(t, AgentWorkspacesPathPrefix+"canvas", "", agentCanvasRequest{Workspace: "demo", Name: "plan"})
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "the canvas read rides the terminal bearer gate")

	resp = h.post(t, AgentWorkspacesPathPrefix+"canvas", testToken, agentCanvasRequest{Workspace: "demo", Name: "plan"})
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var view agentCanvasView
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&view))
	assert.Equal(t, "demo", view.Workspace)
	assert.Equal(t, "plan", view.Name)
	require.NotNil(t, view.Frontmatter, "frontmatter is never null on the wire")
	require.NotNil(t, view.Blocks, "blocks is never null on the wire")
	assert.Empty(t, view.Blocks, "a name nothing was written under answers empty, not an error")

	_, err = h.core.Canvas.PutBlock(t.Context(), rec.ID, "plan", "The Plan", "", canvas.Block{ID: "a", Kind: canvas.KindMarkdown, Body: "hello"})
	require.NoError(t, err)
	_, err = h.core.Canvas.SetFrontmatter(t.Context(), rec.ID, "plan", map[string]any{"tags": []any{"release"}})
	require.NoError(t, err)

	resp = h.post(t, AgentWorkspacesPathPrefix+"canvas", testToken, agentCanvasRequest{Workspace: "demo", Name: "plan"})
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&view))
	assert.Equal(t, "The Plan", view.Title)
	assert.Equal(t, rec.ID, view.Session)
	assert.Equal(t, map[string]any{"tags": []any{"release"}}, view.Frontmatter)
	require.Len(t, view.Blocks, 1)

	listResp := h.post(t, AgentWorkspacesPathPrefix+"canvases", testToken, agentCanvasListRequest{Workspace: "demo"})
	defer func() { _ = listResp.Body.Close() }()
	require.Equal(t, http.StatusOK, listResp.StatusCode)
	var list agentCanvasListResponse
	require.NoError(t, json.NewDecoder(listResp.Body).Decode(&list))
	require.Len(t, list.Canvases, 1)
	assert.Equal(t, "plan", list.Canvases[0].Name)
	assert.Equal(t, "The Plan", list.Canvases[0].Title)
	assert.Equal(t, rec.ID, list.Canvases[0].Session)
	assert.Equal(t, 1, list.Canvases[0].BlockCount)

	empty := h.post(t, AgentWorkspacesPathPrefix+"canvases", testToken, agentCanvasListRequest{Workspace: "other"})
	defer func() { _ = empty.Body.Close() }()
	require.Equal(t, http.StatusOK, empty.StatusCode)
	var emptyList agentCanvasListResponse
	require.NoError(t, json.NewDecoder(empty.Body).Decode(&emptyList))
	require.NotNil(t, emptyList.Canvases, "canvases is never null on the wire")
	assert.Empty(t, emptyList.Canvases)

	mdResp := h.post(t, AgentWorkspacesPathPrefix+"canvas/markdown", testToken, agentCanvasRequest{Workspace: "demo", Name: "plan"})
	defer func() { _ = mdResp.Body.Close() }()
	require.Equal(t, http.StatusOK, mdResp.StatusCode)
	var md agentCanvasMarkdownResponse
	require.NoError(t, json.NewDecoder(mdResp.Body).Decode(&md))
	assert.Contains(t, md.Markdown, "tags:\n    - release\n---\n\n# The Plan\n\nhello\n")

	missing := h.post(t, AgentWorkspacesPathPrefix+"canvas/markdown", testToken, agentCanvasRequest{Workspace: "demo", Name: "ghost"})
	_ = missing.Body.Close()
	assert.Equal(t, http.StatusNotFound, missing.StatusCode, "copying a canvas that does not exist is a surfaced mistake")

	dest := filepath.Join(t.TempDir(), "plan.md")
	exportResp := h.post(t, AgentWorkspacesPathPrefix+"canvas/export", testToken, agentCanvasExportRequest{Workspace: "demo", Name: "plan", Path: dest})
	defer func() { _ = exportResp.Body.Close() }()
	require.Equal(t, http.StatusOK, exportResp.StatusCode)
	written, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, md.Markdown, string(written))

	deleteResp := h.post(t, AgentWorkspacesPathPrefix+"canvas/delete", testToken, agentCanvasRequest{Workspace: "demo", Name: "plan"})
	defer func() { _ = deleteResp.Body.Close() }()
	require.Equal(t, http.StatusOK, deleteResp.StatusCode)
	var deleted agentCanvasDeleteResponse
	require.NoError(t, json.NewDecoder(deleteResp.Body).Decode(&deleted))
	assert.Equal(t, "plan", deleted.Deleted)

	missingDelete := h.post(t, AgentWorkspacesPathPrefix+"canvas/delete", testToken, agentCanvasRequest{Workspace: "demo", Name: "plan"})
	_ = missingDelete.Body.Close()
	assert.Equal(t, http.StatusNotFound, missingDelete.StatusCode)
}

func TestAgentCanvasRepositoriesOverTheWire(t *testing.T) {
	h := newAgentHarness(t)

	resp := h.post(t, AgentWorkspacesPathPrefix+"canvas/repositories", "", struct{}{})
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	resp = h.post(t, AgentWorkspacesPathPrefix+"canvas/repositories", testToken, struct{}{})
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body agentCanvasRepositoriesResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.NotNil(t, body.Repositories, "repositories is never null on the wire")
	assert.Empty(t, body.Repositories)
}
