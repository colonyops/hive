package mcpsrv

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/colonyops/hive/internal/domain/drive"
)

// register declares the drive tool table, under the same rules as
// Controller.register: metadata only, thin handlers, descriptions written for
// a model.
func (ctrl *DriveController) register(srv *mcp.Server) {
	ctrl.registerCanvas(srv)
	ctrl.registerDocuments(srv)
}

func (ctrl *DriveController) registerCanvas(srv *mcp.Server) {
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
			"laid out with the app's hv- classes — read the hive-drive docs first, since an unknown tag, class or attribute is " +
			"refused rather than dropped) or link (title and url " +
			"required; http, https or mailto only). Answers with the canvas metadata and the stored block; read_canvas " +
			"returns the full surface. No Drive tab opens by itself: a write while one " +
			"is open behind another tab lights a dot on it, and with none open it shows nothing until the user opens " +
			"Drive. Use open_canvas when the result deserves the user's attention now. Writing a first layout of several blocks? put_blocks does it in one call.",
	}, ctrl.PutBlock)

	addTool(srv, &mcp.Tool{
		Name:  "put_blocks",
		Title: "Put a batch of canvas blocks",
		Description: "Write several blocks to one canvas in a single atomic call — one write, one render, so an " +
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
		Name:  "set_frontmatter",
		Title: "Set canvas front matter",
		Description: "Replace all editable front matter on an existing canvas. Use short keys and scalar values or " +
			"lists of scalars; tags is conventionally a list of strings. Hive owns created_at and updated_at, updates " +
			"them automatically, and refuses either key here. Pass an empty object to remove all editable fields. " +
			"Copy and save include the resulting YAML front matter. Answers with the canvas metadata.",
	}, ctrl.SetFrontmatter)

	addTool(srv, &mcp.Tool{
		Name:  "clear_canvas",
		Title: "Clear a canvas",
		Description: "Remove every block from one canvas at once and answer with its now-empty metadata. The canvas, its " +
			"name and its title survive — put_block after a clear starts a fresh layout in the same canvas. Prefer " +
			"put_block with stable ids for revisions; clear is for abandoning a layout wholesale, and delete_canvas for " +
			"discarding the canvas itself.",
	}, ctrl.ClearCanvas)

	addTool(srv, &mcp.Tool{
		Name:  "delete_canvas",
		Title: "Delete a canvas",
		Description: "Remove one canvas entirely — its file is deleted and it leaves the Drive " +
			"tab's picker. A name that does not exist is not_found. Only delete a canvas you made obsolete yourself; the user " +
			"may be keeping the others.",
	}, ctrl.DeleteCanvas)

	addTool(srv, &mcp.Tool{
		Name:  "read_canvas",
		Title: "Read a canvas",
		Description: "Read one canvas exactly as the user sees it: its editable front matter and every block in order, " +
			"with your ids, kinds and content — an html block comes back as the markup you wrote, before the app sanitizes it for display. A name " +
			"nothing was written under is not_found — use list_canvases to see what exists. Use this to re-orient after " +
			"a long conversation instead of assuming what you last wrote.",
	}, ctrl.ReadCanvas)

	addTool(srv, &mcp.Tool{
		Name:  "open_canvas",
		Title: "Open a canvas in the Drive tab",
		Description: "Ask Hive to open the Drive tab of this chat or Code session, optionally on one canvas by " +
			"name (the name must exist — put_block first). With no Drive tab open, Hive adds one behind the tab in front, " +
			"with an unseen dot; it never takes the screen from what the user is doing. It applies only while the user is " +
			"viewing this chat or session. A write while a Drive tab is open behind another tab already lights a dot on it, " +
			"so call this when you finish something worth looking at now, not on every write. A global canvas lives in the " +
			"shared space, which has no Drive tab; the user reads it in the Drive page.",
	}, ctrl.OpenCanvas)

	addTool(srv, &mcp.Tool{
		Name:  "close_canvas",
		Title: "Close the Drive tab",
		Description: "Ask Hive to close the Drive tab of this chat or Code session. Like open_canvas it applies only while the " +
			"user is viewing this chat or session. Canvases and documents are untouched — this closes the tab, not the " +
			"content.",
	}, ctrl.CloseCanvas)

	addTool(srv, &mcp.Tool{
		Name:  "list_canvases",
		Title: "List the canvases",
		Description: "List every canvas where yours are kept — this chat's workspace, the repository of this hive " +
			"session, or the shared space when session is global — most recently updated first: name, title, block count and which chat or session created it. " +
			"Canvases outlive what made them, so this may include artifacts from earlier conversations — read one " +
			"before assuming it is yours to overwrite.",
	}, ctrl.ListCanvases)
}

type canvasSessionInput struct {
	Session string `json:"session" jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which files the canvas in one namespace every such agent shares. Never guess or reuse another value."`
}

type canvasNameInput struct {
	Session string `json:"session" jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which files the canvas in one namespace every such agent shares. Never guess or reuse another value."`
	Canvas  string `json:"canvas"  jsonschema:"The canvas name or path. A bare name like perf files at canvases/perf.canvas.json; a path like reports/2026-10/perf files at reports/2026-10/perf.canvas.json, creating the folders. The last segment is short, lowercase and filename-like (letters, digits, dots, hyphens, underscores; starts and ends alphanumeric). Name or path is the canvas's identity in its space."`
}

type putBlockInput struct {
	Session     string `json:"session"               jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which files the canvas in one namespace every such agent shares. Never guess or reuse another value."`
	Canvas      string `json:"canvas"                jsonschema:"The canvas name or path. A bare name like perf files at canvases/perf.canvas.json; a path like reports/2026-10/perf files at reports/2026-10/perf.canvas.json, creating the folders. The last segment is short, lowercase and filename-like (letters, digits, dots, hyphens, underscores; starts and ends alphanumeric). Name or path is the canvas's identity in its space."`
	CanvasTitle string `json:"canvasTitle,omitempty" jsonschema:"Display title for the whole canvas, shown in the Drive tab's picker. Set it on the canvas's first write; a later non-empty value renames, empty leaves the stored title unchanged."`
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
	Canvas      string            `json:"canvas"                jsonschema:"The canvas name or path. A bare name like perf files at canvases/perf.canvas.json; a path like reports/2026-10/perf files at reports/2026-10/perf.canvas.json, creating the folders. The last segment is short, lowercase and filename-like (letters, digits, dots, hyphens, underscores; starts and ends alphanumeric). Name or path is the canvas's identity in its space."`
	CanvasTitle string            `json:"canvasTitle,omitempty" jsonschema:"Display title for the whole canvas, shown in the Drive tab's picker. Set it on the canvas's first write; a later non-empty value renames, empty leaves the stored title unchanged."`
	Blocks      []batchBlockInput `json:"blocks"                jsonschema:"The blocks to write, applied in order (50 max per call)."`
}

type removeBlockInput struct {
	Session string `json:"session" jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which files the canvas in one namespace every such agent shares. Never guess or reuse another value."`
	Canvas  string `json:"canvas"  jsonschema:"The canvas name or path. A bare name like perf files at canvases/perf.canvas.json; a path like reports/2026-10/perf files at reports/2026-10/perf.canvas.json, creating the folders. The last segment is short, lowercase and filename-like (letters, digits, dots, hyphens, underscores; starts and ends alphanumeric). Name or path is the canvas's identity in its space."`
	ID      string `json:"id"      jsonschema:"The id of the block to remove, as given to put_block."`
}

type setFrontmatterInput struct {
	Session     string         `json:"session"     jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which files the canvas in one namespace every such agent shares. Never guess or reuse another value."`
	Canvas      string         `json:"canvas"      jsonschema:"The existing canvas whose editable front matter to replace, by name or path."`
	Frontmatter map[string]any `json:"frontmatter" jsonschema:"The complete editable front matter. Keys use letters, digits, dots, hyphens, or underscores. Values are strings, finite numbers, booleans, null, or flat lists of those values. created_at and updated_at are reserved and populated by Hive."`
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
// single-block write; read_canvas returns the full surface.
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
	Deleted string `json:"deleted" jsonschema:"The name of the canvas that was deleted."`
}

type openCanvasInput struct {
	Session string `json:"session"          jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which files the canvas in one namespace every such agent shares. Never guess or reuse another value."`
	Canvas  string `json:"canvas,omitempty" jsonschema:"Canvas to show in the Drive tab, by name or path; must exist. Omit to leave the tab on its own pick."`
}

type driveOpenResult struct {
	Requested string `json:"requested" jsonschema:"The request handed to the UI: open or close. Delivery is best-effort — it applies only while the user is viewing this chat or session, and there is no acknowledgment either way."`
}

func (ctrl *DriveController) PutBlock(ctx context.Context, _ *mcp.CallToolRequest, in putBlockInput) (*mcp.CallToolResult, canvasWriteResult, error) {
	c, err := ctrl.core.Drive.PutBlock(ctx, in.Session, in.Canvas, in.CanvasTitle, in.Before, drive.Block{
		ID: in.ID, Kind: in.Kind, Title: in.Title, Body: in.Body, URL: in.URL,
	})
	if err != nil {
		return nil, canvasWriteResult{}, ctrl.toolError(err)
	}
	return nil, canvasWriteResultFrom(c, in.ID), nil
}

func (ctrl *DriveController) PutBlocks(ctx context.Context, _ *mcp.CallToolRequest, in putBlocksInput) (*mcp.CallToolResult, canvasWriteResult, error) {
	blocks := make([]drive.Block, 0, len(in.Blocks))
	for _, b := range in.Blocks {
		blocks = append(blocks, drive.Block{ID: b.ID, Kind: b.Kind, Title: b.Title, Body: b.Body, URL: b.URL})
	}
	c, err := ctrl.core.Drive.PutBlocks(ctx, in.Session, in.Canvas, in.CanvasTitle, blocks)
	if err != nil {
		return nil, canvasWriteResult{}, ctrl.toolError(err)
	}
	return nil, canvasWriteResultFrom(c, ""), nil
}

func (ctrl *DriveController) RemoveBlock(ctx context.Context, _ *mcp.CallToolRequest, in removeBlockInput) (*mcp.CallToolResult, canvasWriteResult, error) {
	c, err := ctrl.core.Drive.RemoveBlock(ctx, in.Session, in.Canvas, in.ID)
	if err != nil {
		return nil, canvasWriteResult{}, ctrl.toolError(err)
	}
	return nil, canvasWriteResultFrom(c, ""), nil
}

func (ctrl *DriveController) SetFrontmatter(ctx context.Context, _ *mcp.CallToolRequest, in setFrontmatterInput) (*mcp.CallToolResult, canvasWriteResult, error) {
	c, err := ctrl.core.Drive.SetFrontmatter(ctx, in.Session, in.Canvas, in.Frontmatter)
	if err != nil {
		return nil, canvasWriteResult{}, ctrl.toolError(err)
	}
	return nil, canvasWriteResultFrom(c, ""), nil
}

func (ctrl *DriveController) ClearCanvas(ctx context.Context, _ *mcp.CallToolRequest, in canvasNameInput) (*mcp.CallToolResult, canvasWriteResult, error) {
	c, err := ctrl.core.Drive.ClearCanvas(ctx, in.Session, in.Canvas)
	if err != nil {
		return nil, canvasWriteResult{}, ctrl.toolError(err)
	}
	return nil, canvasWriteResultFrom(c, ""), nil
}

func (ctrl *DriveController) DeleteCanvas(ctx context.Context, _ *mcp.CallToolRequest, in canvasNameInput) (*mcp.CallToolResult, deleteCanvasResult, error) {
	if err := ctrl.core.Drive.DeleteCanvas(ctx, in.Session, in.Canvas); err != nil {
		return nil, deleteCanvasResult{}, ctrl.toolError(err)
	}
	return nil, deleteCanvasResult{Deleted: in.Canvas}, nil
}

func (ctrl *DriveController) ReadCanvas(ctx context.Context, _ *mcp.CallToolRequest, in canvasNameInput) (*mcp.CallToolResult, canvasResult, error) {
	c, err := ctrl.core.Drive.GetCanvas(ctx, in.Session, in.Canvas)
	if err != nil {
		return nil, canvasResult{}, ctrl.toolError(err)
	}
	return nil, canvasResultFrom(c), nil
}

func (ctrl *DriveController) OpenCanvas(ctx context.Context, _ *mcp.CallToolRequest, in openCanvasInput) (*mcp.CallToolResult, driveOpenResult, error) {
	if err := ctrl.core.Drive.SetDriveOpen(ctx, in.Session, in.Canvas, true); err != nil {
		return nil, driveOpenResult{}, ctrl.toolError(err)
	}
	return nil, driveOpenResult{Requested: "open"}, nil
}

func (ctrl *DriveController) CloseCanvas(ctx context.Context, _ *mcp.CallToolRequest, in canvasSessionInput) (*mcp.CallToolResult, driveOpenResult, error) {
	if err := ctrl.core.Drive.SetDriveOpen(ctx, in.Session, "", false); err != nil {
		return nil, driveOpenResult{}, ctrl.toolError(err)
	}
	return nil, driveOpenResult{Requested: "close"}, nil
}

func (ctrl *DriveController) ListCanvases(ctx context.Context, _ *mcp.CallToolRequest, in canvasSessionInput) (*mcp.CallToolResult, canvasListResult, error) {
	metas, err := ctrl.core.Drive.ListCanvases(ctx, in.Session)
	if err != nil {
		return nil, canvasListResult{}, ctrl.toolError(err)
	}
	out := canvasListResult{Canvases: make([]canvasMetaResult, 0, len(metas))}
	for _, m := range metas {
		out.Canvases = append(out.Canvases, canvasMetaResult{
			Name: drive.CanvasName(m.Path), Title: m.Title, Session: m.Session, HiveSession: m.HiveSession,
			CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt, BlockCount: m.BlockCount,
		})
	}
	return nil, out, nil
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
