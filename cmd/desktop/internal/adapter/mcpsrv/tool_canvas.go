package mcpsrv

import (
	"context"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/colonyops/hive/internal/domain/drive"
	driveops "github.com/colonyops/hive/internal/ops/drive"
	"github.com/colonyops/hive/internal/ops/mcpops"
)

// register declares the drive tool table, under the same rules as
// Controller.register: metadata only, thin handlers, descriptions written for
// a model.
func (ctrl *DriveController) register(srv *mcp.Server) {
	ctrl.registerCanvas(srv)
	ctrl.registerDocuments(srv)
	ctrl.registerTabs(srv)
}

func (ctrl *DriveController) registerCanvas(srv *mcp.Server) {
	mcpops.Mount(srv, driveops.CanvasBlockPut, ctrl.PutBlock, ctrl.toolError)
	mcpops.Mount(srv, driveops.CanvasBlocksPut, ctrl.PutBlocks, ctrl.toolError)
	mcpops.Mount(srv, driveops.CanvasBlockDelete, ctrl.RemoveBlock, ctrl.toolError)
	mcpops.Mount(srv, driveops.CanvasFrontmatterPut, ctrl.SetFrontmatter, ctrl.toolError)
	mcpops.Mount(srv, driveops.CanvasClear, ctrl.ClearCanvas, ctrl.toolError)
	mcpops.Mount(srv, driveops.CanvasDelete, ctrl.DeleteCanvas, ctrl.toolError)
	mcpops.Mount(srv, driveops.CanvasGet, ctrl.ReadCanvas, ctrl.toolError)
	mcpops.Mount(srv, driveops.CanvasList, ctrl.ListCanvases, ctrl.toolError)
}

type canvasBlock struct {
	ID        string `json:"id"              jsonschema:"The agent-chosen id canvas_block_put was called with."`
	Kind      string `json:"kind"            jsonschema:"markdown, html or link."`
	Title     string `json:"title,omitempty"`
	Body      string `json:"body,omitempty"  jsonschema:"The markdown source of a markdown block, or an html block's markup as you wrote it."`
	URL       string `json:"url,omitempty"   jsonschema:"The target of a link block."`
	CreatedAt int64  `json:"createdAt"       jsonschema:"Unix milliseconds when the block first appeared."`
	UpdatedAt int64  `json:"updatedAt"`
}

type canvasResult struct {
	Workspace   string         `json:"workspace"             jsonschema:"What this canvas belongs to: a workspace, owner/repo for a hive session's repository, or @global."`
	Name        string         `json:"name"`
	Title       string         `json:"title,omitempty"`
	Session     string         `json:"session"               jsonschema:"The chat UUID that created this canvas, or empty."`
	HiveSession string         `json:"hiveSession,omitempty" jsonschema:"The hive session that created this canvas, when no chat did."`
	CreatedAt   int64          `json:"createdAt"`
	UpdatedAt   int64          `json:"updatedAt"`
	Frontmatter map[string]any `json:"frontmatter"           jsonschema:"Agent-editable front matter. created_at and updated_at are exposed separately and added on Markdown export."`
	Blocks      []canvasBlock  `json:"blocks"                jsonschema:"Every block, in display order."`
}

type canvasMetaResult struct {
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Session     string `json:"session"               jsonschema:"The chat UUID that created this canvas, or empty."`
	HiveSession string `json:"hiveSession,omitempty" jsonschema:"The hive session that created this canvas, when no chat did."`
	CreatedAt   int64  `json:"createdAt"`
	UpdatedAt   int64  `json:"updatedAt"`
	BlockCount  int    `json:"blockCount"`
}

// canvasWriteResult is what a mutation answers: the canvas metadata rather
// than every block, so revising a large canvas does not re-send the whole
// surface into the agent's context. Block carries the stored block for a
// single-block write; canvas_get returns the full surface.
type canvasWriteResult struct {
	Workspace   string         `json:"workspace"             jsonschema:"What this canvas belongs to: a workspace, owner/repo for a hive session's repository, or @global."`
	Name        string         `json:"name"`
	Title       string         `json:"title,omitempty"`
	Session     string         `json:"session"               jsonschema:"The chat UUID that created this canvas, or empty."`
	HiveSession string         `json:"hiveSession,omitempty" jsonschema:"The hive session that created this canvas, when no chat did."`
	CreatedAt   int64          `json:"createdAt"`
	UpdatedAt   int64          `json:"updatedAt"`
	Frontmatter map[string]any `json:"frontmatter"           jsonschema:"Agent-editable front matter. created_at and updated_at are exposed separately and added on Markdown export."`
	BlockCount  int            `json:"blockCount"`
	Block       *canvasBlock   `json:"block,omitempty"       jsonschema:"The block as stored, echoed for a single-block write."`
}

type canvasListResult struct {
	Canvases []canvasMetaResult `json:"canvases" jsonschema:"Every canvas the owner holds, most recently updated first."`
}

type deleteCanvasResult struct {
	Canvas  string    `json:"canvas"  jsonschema:"The canvas that was deleted, as named."`
	Deleted bool      `json:"deleted" jsonschema:"Always true: a delete that did not happen is an error."`
	At      time.Time `json:"at"      jsonschema:"When it was deleted."`
}

func (ctrl *DriveController) PutBlock(ctx context.Context, in driveops.CanvasBlockPutIn) (canvasWriteResult, error) {
	c, err := ctrl.core.Drive.PutBlock(ctx, in.Caller, in.Canvas, in.CanvasTitle, in.Before, drive.Block{
		ID: in.ID, Kind: in.Kind, Title: in.Title, Body: in.Body, URL: in.URL,
	})
	if err != nil {
		return canvasWriteResult{}, err
	}
	return canvasWriteResultFrom(c, in.ID), nil
}

func (ctrl *DriveController) PutBlocks(ctx context.Context, in driveops.CanvasBlocksPutIn) (canvasWriteResult, error) {
	blocks := make([]drive.Block, 0, len(in.Blocks))
	for _, b := range in.Blocks {
		blocks = append(blocks, drive.Block{ID: b.ID, Kind: b.Kind, Title: b.Title, Body: b.Body, URL: b.URL})
	}
	c, err := ctrl.core.Drive.PutBlocks(ctx, in.Caller, in.Canvas, in.CanvasTitle, blocks)
	if err != nil {
		return canvasWriteResult{}, err
	}
	return canvasWriteResultFrom(c, ""), nil
}

func (ctrl *DriveController) RemoveBlock(ctx context.Context, in driveops.CanvasBlockDeleteIn) (canvasWriteResult, error) {
	c, err := ctrl.core.Drive.RemoveBlock(ctx, in.Caller, in.Canvas, in.ID)
	if err != nil {
		return canvasWriteResult{}, err
	}
	return canvasWriteResultFrom(c, ""), nil
}

func (ctrl *DriveController) SetFrontmatter(ctx context.Context, in driveops.CanvasFrontmatterPutIn) (canvasWriteResult, error) {
	c, err := ctrl.core.Drive.SetFrontmatter(ctx, in.Caller, in.Canvas, in.Frontmatter)
	if err != nil {
		return canvasWriteResult{}, err
	}
	return canvasWriteResultFrom(c, ""), nil
}

func (ctrl *DriveController) ClearCanvas(ctx context.Context, in driveops.CanvasIn) (canvasWriteResult, error) {
	c, err := ctrl.core.Drive.ClearCanvas(ctx, in.Caller, in.Canvas)
	if err != nil {
		return canvasWriteResult{}, err
	}
	return canvasWriteResultFrom(c, ""), nil
}

func (ctrl *DriveController) DeleteCanvas(ctx context.Context, in driveops.CanvasIn) (deleteCanvasResult, error) {
	if err := ctrl.core.Drive.DeleteCanvas(ctx, in.Caller, in.Canvas); err != nil {
		return deleteCanvasResult{}, err
	}
	return deleteCanvasResult{Canvas: in.Canvas, Deleted: true, At: time.Now()}, nil
}

func (ctrl *DriveController) ReadCanvas(ctx context.Context, in driveops.CanvasIn) (canvasResult, error) {
	c, err := ctrl.core.Drive.GetCanvas(ctx, in.Caller, in.Canvas)
	if err != nil {
		return canvasResult{}, err
	}
	return canvasResultFrom(c), nil
}

func (ctrl *DriveController) ListCanvases(ctx context.Context, in driveops.CanvasListIn) (canvasListResult, error) {
	metas, err := ctrl.core.Drive.ListCanvases(ctx, in.Caller)
	if err != nil {
		return canvasListResult{}, err
	}
	out := canvasListResult{Canvases: make([]canvasMetaResult, 0, len(metas))}
	for _, m := range metas {
		out.Canvases = append(out.Canvases, canvasMetaResult{
			Name: drive.CanvasName(m.Path), Title: m.Title, Session: m.Session, HiveSession: m.HiveSession,
			CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt, BlockCount: m.BlockCount,
		})
	}
	return out, nil
}

func canvasWriteResultFrom(c drive.Canvas, blockID string) canvasWriteResult {
	res := canvasWriteResult{
		Workspace: canvasOwner(c.Space), Name: drive.CanvasName(c.Path), Title: c.Title, Session: c.Session, HiveSession: c.HiveSession,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, Frontmatter: c.Frontmatter, BlockCount: len(c.Blocks),
	}
	for _, b := range c.Blocks {
		if b.ID == blockID {
			res.Block = &canvasBlock{
				ID: b.ID, Kind: b.Kind, Title: b.Title, Body: b.Body, URL: b.URL,
				CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt,
			}
			break
		}
	}
	return res
}

func canvasResultFrom(c drive.Canvas) canvasResult {
	blocks := make([]canvasBlock, 0, len(c.Blocks))
	for _, b := range c.Blocks {
		blocks = append(blocks, canvasBlock{
			ID: b.ID, Kind: b.Kind, Title: b.Title, Body: b.Body, URL: b.URL,
			CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt,
		})
	}
	return canvasResult{
		Workspace: canvasOwner(c.Space), Name: drive.CanvasName(c.Path), Title: c.Title, Session: c.Session, HiveSession: c.HiveSession,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, Frontmatter: c.Frontmatter, Blocks: blocks,
	}
}

// legacyGlobalOwner is the owner key hive-canvas results still name the shared
// space by; the @ kept it out of the workspace names hive creates.
const legacyGlobalOwner = "@global"

// canvasOwner maps a space key onto the owner key the hive-canvas results
// still name: shared is @global, workspaces/<name> is the name.
func canvasOwner(spaceKey string) string {
	if spaceKey == drive.SharedKey {
		return legacyGlobalOwner
	}
	if name, ok := strings.CutPrefix(spaceKey, drive.WorkspacesDir+"/"); ok {
		return name
	}
	return spaceKey
}
