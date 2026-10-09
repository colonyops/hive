package httpapi

import (
	"net/http"

	"github.com/hay-kot/criterio"
	"github.com/hay-kot/httpkit/server"

	"github.com/colonyops/hive/cmd/desktop/internal/app/canvas"
)

// The frontend reads canvases and may delete one after user confirmation.
// Block and front matter edits remain agent-owned through hive-canvas; export
// writes a rendering elsewhere on disk.

type agentCanvasBlock struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	URL       string `json:"url"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

// Workspace is the canvas's owner key in every type here: a workspace's
// directory name, or owner/repo for a repository's canvases.
type agentCanvasView struct {
	Workspace   string             `json:"workspace"`
	Name        string             `json:"name"`
	Title       string             `json:"title"`
	Session     string             `json:"session"`
	HiveSession string             `json:"hiveSession"`
	CreatedAt   int64              `json:"createdAt"`
	UpdatedAt   int64              `json:"updatedAt"`
	Frontmatter map[string]any     `json:"frontmatter"`
	Blocks      []agentCanvasBlock `json:"blocks"`
}

type agentCanvasMeta struct {
	Workspace   string `json:"workspace"`
	Name        string `json:"name"`
	Title       string `json:"title"`
	Session     string `json:"session"`
	HiveSession string `json:"hiveSession"`
	CreatedAt   int64  `json:"createdAt"`
	UpdatedAt   int64  `json:"updatedAt"`
	BlockCount  int    `json:"blockCount"`
}

type agentCanvasRequest struct {
	Workspace string `json:"workspace"`
	Name      string `json:"name"`
}

func (b agentCanvasRequest) Validate() error {
	return criterio.ValidateStruct(
		criterio.Run("workspace", b.Workspace, criterio.Required),
		criterio.Run("name", b.Name, criterio.Required),
	)
}

// AgentCanvas reads one canvas by workspace and name. A name nothing was
// written under answers an empty canvas, so the pane never errors on a
// canvas deleted while it was open.
func (ctrl *Controller) AgentCanvas(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[agentCanvasRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	c, err := ctrl.core.Canvas.GetForOwner(r.Context(), body.Workspace, body.Name)
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, toAgentCanvasView(c))
}

type agentCanvasListRequest struct {
	Workspace string `json:"workspace"`
}

func (b agentCanvasListRequest) Validate() error {
	return criterio.Run("workspace", b.Workspace, criterio.Required)
}

type agentCanvasListResponse struct {
	Canvases []agentCanvasMeta `json:"canvases"`
}

// AgentCanvasList lists a workspace's canvases, most recently updated first.
func (ctrl *Controller) AgentCanvasList(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[agentCanvasListRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	metas, err := ctrl.core.Canvas.ListForOwner(r.Context(), body.Workspace)
	if err != nil {
		return err
	}
	views := make([]agentCanvasMeta, 0, len(metas))
	for _, m := range metas {
		views = append(views, agentCanvasMeta{
			Workspace: m.Workspace, Name: m.Name, Title: m.Title, Session: m.Session, HiveSession: m.HiveSession,
			CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt, BlockCount: m.BlockCount,
		})
	}
	return server.JSON(w, http.StatusOK, agentCanvasListResponse{Canvases: views})
}

type agentCanvasRepositoriesResponse struct {
	Repositories []string `json:"repositories"`
}

// AgentCanvasRepositories lists the repositories that hold a canvas, by owner
// key, so the full-page view can offer them beside the workspaces.
func (ctrl *Controller) AgentCanvasRepositories(w http.ResponseWriter, r *http.Request) error {
	if _, err := terminalBody[struct{}](ctrl, w, r); err != nil {
		return err
	}
	keys, err := ctrl.core.Canvas.Repositories(r.Context())
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, agentCanvasRepositoriesResponse{Repositories: keys})
}

type agentCanvasDeleteResponse struct {
	Deleted string `json:"deleted"`
}

// AgentCanvasDelete removes the selected canvas from its owner after the
// frontend confirms the destructive action.
func (ctrl *Controller) AgentCanvasDelete(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[agentCanvasRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	if err := ctrl.core.Canvas.DeleteForOwner(r.Context(), body.Workspace, body.Name); err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, agentCanvasDeleteResponse{Deleted: body.Name})
}

type agentCanvasMarkdownResponse struct {
	Markdown string `json:"markdown"`
}

// AgentCanvasMarkdown renders one canvas as a standalone markdown document,
// for the pane's copy action.
func (ctrl *Controller) AgentCanvasMarkdown(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[agentCanvasRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	markdown, err := ctrl.core.Canvas.MarkdownForOwner(r.Context(), body.Workspace, body.Name)
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, agentCanvasMarkdownResponse{Markdown: markdown})
}

type agentCanvasExportRequest struct {
	Workspace string `json:"workspace"`
	Name      string `json:"name"`
	Path      string `json:"path"`
}

func (b agentCanvasExportRequest) Validate() error {
	return criterio.ValidateStruct(
		criterio.Run("workspace", b.Workspace, criterio.Required),
		criterio.Run("name", b.Name, criterio.Required),
		criterio.Run("path", b.Path, criterio.Required),
	)
}

type agentCanvasExportResponse struct {
	Path string `json:"path"`
}

// AgentCanvasExport writes one canvas's markdown rendering to a path the
// user chose in the native save dialog.
func (ctrl *Controller) AgentCanvasExport(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[agentCanvasExportRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	if err := ctrl.core.Canvas.ExportForOwner(r.Context(), body.Workspace, body.Name, body.Path); err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, agentCanvasExportResponse{Path: body.Path})
}

func toAgentCanvasView(c canvas.Canvas) agentCanvasView {
	blocks := make([]agentCanvasBlock, 0, len(c.Blocks))
	for _, b := range c.Blocks {
		blocks = append(blocks, agentCanvasBlock{
			ID: b.ID, Kind: b.Kind, Title: b.Title, Body: b.Body, URL: b.URL,
			CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt,
		})
	}
	return agentCanvasView{
		Workspace: c.Workspace, Name: c.Name, Title: c.Title, Session: c.Session, HiveSession: c.HiveSession,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, Frontmatter: c.Frontmatter, Blocks: blocks,
	}
}
