package mcpsrv

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/colonyops/hive/cmd/desktop/internal/app/canvas"
)

// register declares the canvas tool table, under the same rules as
// Controller.register: metadata only, thin handlers, descriptions written for
// a model.
func (ctrl *CanvasController) register(srv *mcp.Server) {
	addTool(srv, &mcp.Tool{
		Name:  "put_block",
		Title: "Put a canvas block",
		Description: "Create or replace one block on a named canvas. The first put_block under a new canvas name creates " +
			"that canvas; a workspace keeps as many as you make, so name canvases by artifact (release-notes, " +
			"perf-report), not by conversation. You choose the block id too: reusing an id updates that block in place, " +
			"keeping its position, while a new id appends at the end — stable ids are how you revise a status line " +
			"instead of stacking copies; before places or moves a block ahead of an existing one instead. kind is " +
			"markdown (body required, title optional; rendered as GitHub-flavored markdown with raw HTML escaped, not " +
			"rendered; a standalone fenced mermaid code block renders as a diagram), html (body required; semantic markup " +
			"laid out with the app's hv- classes — read the hive-canvas docs first, since an unknown tag, class or attribute is " +
			"refused rather than dropped) or link (title and url " +
			"required; http, https or mailto only). Answers with the canvas metadata and the stored block; read_canvas " +
			"returns the full surface. The pane does not open by itself: a write while " +
			"it is closed lights an unseen dot on the pane's toggle — use open_canvas when the result deserves the " +
			"user's attention now. Writing a first layout of several blocks? put_blocks does it in one call.",
	}, ctrl.PutBlock)

	addTool(srv, &mcp.Tool{
		Name:  "put_blocks",
		Title: "Put a batch of canvas blocks",
		Description: "Write several blocks to one canvas in a single atomic call — one write, one pane render, so an " +
			"initial layout appears whole instead of assembling block by block. Blocks follow put_block's rules and " +
			"apply in order; every block is validated first, so a rejected batch leaves the canvas untouched. Answers " +
			"with the canvas metadata. For revising one block, prefer put_block.",
	}, ctrl.PutBlocks)

	addTool(srv, &mcp.Tool{
		Name:  "remove_block",
		Title: "Remove a canvas block",
		Description: "Remove one block by the id you gave it in put_block, and answer with the canvas metadata that " +
			"remains. An id that is not on the canvas is not_found — nothing is ever reported removed that was not there.",
	}, ctrl.RemoveBlock)

	addTool(srv, &mcp.Tool{
		Name:  "clear_canvas",
		Title: "Clear a canvas",
		Description: "Remove every block from one canvas at once and answer with its now-empty metadata. The canvas, its " +
			"name and its title survive — put_block after a clear starts a fresh layout in the same pane. Prefer " +
			"put_block with stable ids for revisions; clear is for abandoning a layout wholesale, and delete_canvas for " +
			"discarding the canvas itself.",
	}, ctrl.ClearCanvas)

	addTool(srv, &mcp.Tool{
		Name:  "delete_canvas",
		Title: "Delete a canvas",
		Description: "Remove one canvas entirely — its file is deleted and it leaves the pane's " +
			"picker. A name that does not exist is not_found. Only delete a canvas you made obsolete yourself; the user " +
			"may be keeping the others.",
	}, ctrl.DeleteCanvas)

	addTool(srv, &mcp.Tool{
		Name:  "read_canvas",
		Title: "Read a canvas",
		Description: "Read one canvas exactly as the user sees it: every block in order, with your ids, kinds and " +
			"content — an html block comes back as the markup you wrote, before the app sanitizes it for display. A name " +
			"nothing was written under is not_found — use list_canvases to see what exists. Use this to re-orient after " +
			"a long conversation instead of assuming what you last wrote.",
	}, ctrl.ReadCanvas)

	addTool(srv, &mcp.Tool{
		Name:  "open_canvas",
		Title: "Open the canvas pane",
		Description: "Ask Hive to open the canvas pane beside this chat or Code session, optionally pinned to one canvas by name " +
			"(the name must exist — put_block first). It applies only while the user is viewing it; it never pulls " +
			"them away from something else, and a write while the pane is closed already shows an unseen dot. Open when " +
			"you finish something worth looking at, not on every write. A global canvas has no pane; the user reads it in " +
			"the full-page canvas view.",
	}, ctrl.OpenCanvas)

	addTool(srv, &mcp.Tool{
		Name:  "close_canvas",
		Title: "Close the canvas pane",
		Description: "Ask Hive to close the canvas pane beside this chat or Code session, returning the full width to the " +
			"conversation. Like open_canvas it applies only while the user is viewing it. The canvases " +
			"themselves are untouched — this is the pane, not the content.",
	}, ctrl.CloseCanvas)

	addTool(srv, &mcp.Tool{
		Name:  "list_canvases",
		Title: "List the canvases",
		Description: "List every canvas where yours are kept — this chat's workspace, or the repository of this hive " +
			"session — most recently updated first: name, title, block count and which chat or session created it. " +
			"Canvases outlive what made them, so this may include artifacts from earlier conversations — read one " +
			"before assuming it is yours to overwrite.",
	}, ctrl.ListCanvases)
}

type canvasSessionInput struct {
	Session string `json:"session" jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which files the canvas in one namespace every such agent shares. Never guess or reuse another value."`
}

type canvasNameInput struct {
	Session string `json:"session" jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which files the canvas in one namespace every such agent shares. Never guess or reuse another value."`
	Canvas  string `json:"canvas"  jsonschema:"The canvas name: short, lowercase, filename-like (letters, digits, dots, hyphens, underscores; starts and ends alphanumeric). It is the canvas's identity in this workspace and its file name on disk."`
}

type putBlockInput struct {
	Session     string `json:"session"               jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which files the canvas in one namespace every such agent shares. Never guess or reuse another value."`
	Canvas      string `json:"canvas"                jsonschema:"The canvas name: short, lowercase, filename-like (letters, digits, dots, hyphens, underscores; starts and ends alphanumeric). It is the canvas's identity in this workspace and its file name on disk."`
	CanvasTitle string `json:"canvasTitle,omitempty" jsonschema:"Display title for the whole canvas, shown in the pane's picker. Set it on the canvas's first write; a later non-empty value renames, empty leaves the stored title unchanged."`
	Before      string `json:"before,omitempty"      jsonschema:"An existing block id to place this block ahead of — inserting a new id there, or moving a reused one (its createdAt survives the move). Omit to keep a reused id's position or append a new one."`
	ID          string `json:"id"                    jsonschema:"Your name for the block. Reusing an id updates that block in place; a new id appends."`
	Kind        string `json:"kind"                  jsonschema:"markdown, html or link."`
	Title       string `json:"title,omitempty"       jsonschema:"Heading shown above a markdown or html body (optional); the visible text of a link (required)."`
	Body        string `json:"body,omitempty"        jsonschema:"The markdown source of a markdown block, which may include standalone fenced mermaid diagrams, or the markup of an html block."`
	URL         string `json:"url,omitempty"         jsonschema:"The target of a link block; http, https or mailto only."`
}

type batchBlockInput struct {
	ID    string `json:"id"              jsonschema:"Your name for the block. Reusing an id updates that block in place; a new id appends."`
	Kind  string `json:"kind"            jsonschema:"markdown, html or link."`
	Title string `json:"title,omitempty" jsonschema:"Heading shown above a markdown or html body (optional); the visible text of a link (required)."`
	Body  string `json:"body,omitempty"  jsonschema:"The markdown source of a markdown block, which may include standalone fenced mermaid diagrams, or the markup of an html block."`
	URL   string `json:"url,omitempty"   jsonschema:"The target of a link block; http, https or mailto only."`
}

type putBlocksInput struct {
	Session     string            `json:"session"               jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which files the canvas in one namespace every such agent shares. Never guess or reuse another value."`
	Canvas      string            `json:"canvas"                jsonschema:"The canvas name: short, lowercase, filename-like (letters, digits, dots, hyphens, underscores; starts and ends alphanumeric). It is the canvas's identity in this workspace and its file name on disk."`
	CanvasTitle string            `json:"canvasTitle,omitempty" jsonschema:"Display title for the whole canvas, shown in the pane's picker. Set it on the canvas's first write; a later non-empty value renames, empty leaves the stored title unchanged."`
	Blocks      []batchBlockInput `json:"blocks"                jsonschema:"The blocks to write, applied in order (50 max per call)."`
}

type removeBlockInput struct {
	Session string `json:"session" jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which files the canvas in one namespace every such agent shares. Never guess or reuse another value."`
	Canvas  string `json:"canvas"  jsonschema:"The canvas name: short, lowercase, filename-like (letters, digits, dots, hyphens, underscores; starts and ends alphanumeric). It is the canvas's identity in this workspace and its file name on disk."`
	ID      string `json:"id"      jsonschema:"The id of the block to remove, as given to put_block."`
}

type canvasBlock struct {
	ID        string `json:"id"              jsonschema:"The agent-chosen id put_block was called with."`
	Kind      string `json:"kind"            jsonschema:"markdown, html or link."`
	Title     string `json:"title,omitempty"`
	Body      string `json:"body,omitempty"  jsonschema:"The markdown source of a markdown block, or an html block's markup as you wrote it."`
	URL       string `json:"url,omitempty"   jsonschema:"The target of a link block."`
	CreatedAt int64  `json:"createdAt"       jsonschema:"Unix milliseconds when the block first appeared."`
	UpdatedAt int64  `json:"updatedAt"`
}

type canvasResult struct {
	Workspace   string        `json:"workspace"             jsonschema:"What this canvas belongs to: a workspace, owner/repo for a hive session's repository, or @global."`
	Name        string        `json:"name"`
	Title       string        `json:"title,omitempty"`
	Session     string        `json:"session"               jsonschema:"The chat UUID that created this canvas, or empty."`
	HiveSession string        `json:"hiveSession,omitempty" jsonschema:"The hive session that created this canvas, when no chat did."`
	CreatedAt   int64         `json:"createdAt"`
	UpdatedAt   int64         `json:"updatedAt"`
	Blocks      []canvasBlock `json:"blocks"                jsonschema:"Every block, in display order."`
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
// single-block write; read_canvas returns the full surface.
type canvasWriteResult struct {
	Workspace   string       `json:"workspace"             jsonschema:"What this canvas belongs to: a workspace, owner/repo for a hive session's repository, or @global."`
	Name        string       `json:"name"`
	Title       string       `json:"title,omitempty"`
	Session     string       `json:"session"               jsonschema:"The chat UUID that created this canvas, or empty."`
	HiveSession string       `json:"hiveSession,omitempty" jsonschema:"The hive session that created this canvas, when no chat did."`
	CreatedAt   int64        `json:"createdAt"`
	UpdatedAt   int64        `json:"updatedAt"`
	BlockCount  int          `json:"blockCount"`
	Block       *canvasBlock `json:"block,omitempty"       jsonschema:"The block as stored, echoed for a single-block write."`
}

type canvasListResult struct {
	Canvases []canvasMetaResult `json:"canvases" jsonschema:"Every canvas the owner holds, most recently updated first."`
}

type deleteCanvasResult struct {
	Deleted string `json:"deleted" jsonschema:"The name of the canvas that was deleted."`
}

type openCanvasInput struct {
	Session string `json:"session"          jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which files the canvas in one namespace every such agent shares. Never guess or reuse another value."`
	Canvas  string `json:"canvas,omitempty" jsonschema:"Canvas to pin the pane to, by name; must exist. Omit to open on the pane's own pick."`
}

type paneResult struct {
	Requested string `json:"requested" jsonschema:"The request handed to the UI: open or close. Delivery is best-effort — it applies only while the user is viewing this chat or session, and there is no acknowledgment either way."`
}

func (ctrl *CanvasController) PutBlock(ctx context.Context, _ *mcp.CallToolRequest, in putBlockInput) (*mcp.CallToolResult, canvasWriteResult, error) {
	c, err := ctrl.core.Canvas.PutBlock(ctx, in.Session, in.Canvas, in.CanvasTitle, in.Before, canvas.Block{
		ID: in.ID, Kind: in.Kind, Title: in.Title, Body: in.Body, URL: in.URL,
	})
	if err != nil {
		return nil, canvasWriteResult{}, ctrl.toolError(err)
	}
	return nil, canvasWriteResultFrom(c, in.ID), nil
}

func (ctrl *CanvasController) PutBlocks(ctx context.Context, _ *mcp.CallToolRequest, in putBlocksInput) (*mcp.CallToolResult, canvasWriteResult, error) {
	blocks := make([]canvas.Block, 0, len(in.Blocks))
	for _, b := range in.Blocks {
		blocks = append(blocks, canvas.Block{ID: b.ID, Kind: b.Kind, Title: b.Title, Body: b.Body, URL: b.URL})
	}
	c, err := ctrl.core.Canvas.PutBlocks(ctx, in.Session, in.Canvas, in.CanvasTitle, blocks)
	if err != nil {
		return nil, canvasWriteResult{}, ctrl.toolError(err)
	}
	return nil, canvasWriteResultFrom(c, ""), nil
}

func (ctrl *CanvasController) RemoveBlock(ctx context.Context, _ *mcp.CallToolRequest, in removeBlockInput) (*mcp.CallToolResult, canvasWriteResult, error) {
	c, err := ctrl.core.Canvas.RemoveBlock(ctx, in.Session, in.Canvas, in.ID)
	if err != nil {
		return nil, canvasWriteResult{}, ctrl.toolError(err)
	}
	return nil, canvasWriteResultFrom(c, ""), nil
}

func (ctrl *CanvasController) ClearCanvas(ctx context.Context, _ *mcp.CallToolRequest, in canvasNameInput) (*mcp.CallToolResult, canvasWriteResult, error) {
	c, err := ctrl.core.Canvas.Clear(ctx, in.Session, in.Canvas)
	if err != nil {
		return nil, canvasWriteResult{}, ctrl.toolError(err)
	}
	return nil, canvasWriteResultFrom(c, ""), nil
}

func (ctrl *CanvasController) DeleteCanvas(ctx context.Context, _ *mcp.CallToolRequest, in canvasNameInput) (*mcp.CallToolResult, deleteCanvasResult, error) {
	if err := ctrl.core.Canvas.Delete(ctx, in.Session, in.Canvas); err != nil {
		return nil, deleteCanvasResult{}, ctrl.toolError(err)
	}
	return nil, deleteCanvasResult{Deleted: in.Canvas}, nil
}

func (ctrl *CanvasController) ReadCanvas(ctx context.Context, _ *mcp.CallToolRequest, in canvasNameInput) (*mcp.CallToolResult, canvasResult, error) {
	c, err := ctrl.core.Canvas.Get(ctx, in.Session, in.Canvas)
	if err != nil {
		return nil, canvasResult{}, ctrl.toolError(err)
	}
	return nil, canvasResultFrom(c), nil
}

func (ctrl *CanvasController) OpenCanvas(ctx context.Context, _ *mcp.CallToolRequest, in openCanvasInput) (*mcp.CallToolResult, paneResult, error) {
	if err := ctrl.core.Canvas.SetPaneOpen(ctx, in.Session, in.Canvas, true); err != nil {
		return nil, paneResult{}, ctrl.toolError(err)
	}
	return nil, paneResult{Requested: "open"}, nil
}

func (ctrl *CanvasController) CloseCanvas(ctx context.Context, _ *mcp.CallToolRequest, in canvasSessionInput) (*mcp.CallToolResult, paneResult, error) {
	if err := ctrl.core.Canvas.SetPaneOpen(ctx, in.Session, "", false); err != nil {
		return nil, paneResult{}, ctrl.toolError(err)
	}
	return nil, paneResult{Requested: "close"}, nil
}

func (ctrl *CanvasController) ListCanvases(ctx context.Context, _ *mcp.CallToolRequest, in canvasSessionInput) (*mcp.CallToolResult, canvasListResult, error) {
	metas, err := ctrl.core.Canvas.List(ctx, in.Session)
	if err != nil {
		return nil, canvasListResult{}, ctrl.toolError(err)
	}
	out := canvasListResult{Canvases: make([]canvasMetaResult, 0, len(metas))}
	for _, m := range metas {
		out.Canvases = append(out.Canvases, canvasMetaResult{
			Name: m.Name, Title: m.Title, Session: m.Session, HiveSession: m.HiveSession,
			CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt, BlockCount: m.BlockCount,
		})
	}
	return nil, out, nil
}

func canvasWriteResultFrom(c canvas.Canvas, blockID string) canvasWriteResult {
	res := canvasWriteResult{
		Workspace: c.Workspace, Name: c.Name, Title: c.Title, Session: c.Session, HiveSession: c.HiveSession,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, BlockCount: len(c.Blocks),
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

func canvasResultFrom(c canvas.Canvas) canvasResult {
	blocks := make([]canvasBlock, 0, len(c.Blocks))
	for _, b := range c.Blocks {
		blocks = append(blocks, canvasBlock{
			ID: b.ID, Kind: b.Kind, Title: b.Title, Body: b.Body, URL: b.URL,
			CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt,
		})
	}
	return canvasResult{
		Workspace: c.Workspace, Name: c.Name, Title: c.Title, Session: c.Session, HiveSession: c.HiveSession,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, Blocks: blocks,
	}
}
