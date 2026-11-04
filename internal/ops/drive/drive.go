// Package drive declares the Hive Drive document and canvas operations,
// served on the drive MCP server.
package drive

import "github.com/colonyops/hive/internal/ops"

var Toolset = ops.Toolset{Name: "drive", MCPPath: "/mcp/drive"}

// MaxBlocksPerCall caps one canvas_blocks_put call; a larger layout is
// written in slices.
const MaxBlocksPerCall = 50

// The document_list detail ladder. A space is as large as its owner made it,
// so the default answers one level and the whole subtree is asked for.
const (
	DetailChildren = "children"
	DetailTree     = "tree"
)

var BlockKinds = []string{"markdown", "html", "link"}

func All() []ops.Op {
	return []ops.Op{
		DocumentList, DocumentGet, DocumentPut, DocumentMove, DocumentDelete,
		CanvasList, CanvasGet, CanvasClear, CanvasDelete,
		CanvasBlockPut, CanvasBlocksPut, CanvasBlockDelete, CanvasFrontmatterPut,
	}
}

type DocumentListIn struct {
	Caller string `json:"caller"           jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which uses the one shared space every such agent shares. Never guess or reuse another value."`
	Path   string `json:"path,omitempty"   jsonschema:"Folder to list, forward slashes, relative to the space root; empty is the space root."`
	Detail string `json:"detail,omitempty" jsonschema:"children (default) lists one level; tree lists the whole subtree."`
}

var DocumentList = ops.Spec[DocumentListIn]{
	Name: "document.list", Owner: ops.OwnerDesktop, Effect: ops.EffectRead, Toolset: Toolset, Caller: ops.CallerRuleSessionOrGlobal,
	Summary: "List documents",
	Guidance: "List a folder of Hive Drive, the document tree the user sees in the Drive tab of this chat or Code session. " +
		"Your session decides the space: a chat's workspace, a hive session's repository (the folder a checkout links as .hive), " +
		"or the shared space for global. Each entry has a kind (dir, markdown, html, canvas, or file for anything else), its path " +
		"inside the space, name, title, size and modified time; markdown and canvas entries carry their front matter as front. " +
		"An empty path is the space root. detail defaults to children, one level down; pass tree for the whole subtree. " +
		"A folder that does not exist is not_found; a document path lists that document alone.",
	Args:   []string{"path"},
	Fields: ops.Constraints{"detail": {Enum: []string{DetailChildren, DetailTree}, Default: DetailChildren}},
}

type DocumentGetIn struct {
	Caller string `json:"caller" jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which uses the one shared space every such agent shares. Never guess or reuse another value."`
	Path   string `json:"path"   jsonschema:"Document path inside the space, forward slashes; it must exist."`
}

var DocumentGet = ops.Spec[DocumentGetIn]{
	Name: "document.get", Owner: ops.OwnerDesktop, Effect: ops.EffectRead, Toolset: Toolset, Caller: ops.CallerRuleSessionOrGlobal,
	Summary: "Read a document",
	Guidance: "Read one markdown or html document, or any other UTF-8 text file in the space, as stored, front matter " +
		"included, with its entry. A canvas is blocks rather than a body: read it with canvas_get. A folder is listed " +
		"with document_list, and a binary file is refused. Use this before rewriting a document another chat or the " +
		"user may have changed.",
	Args: []string{"path"},
}

type DocumentPutIn struct {
	Caller string `json:"caller" jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which uses the one shared space every such agent shares. Never guess or reuse another value."`
	Path   string `json:"path"   jsonschema:"Document path inside the space, forward slashes, ending in .md or .html; folders are created as needed."`
	Body   string `json:"body"   jsonschema:"The whole document. Markdown may start with YAML front matter."`
}

var DocumentPut = ops.Spec[DocumentPutIn]{
	Name: "document.put", Owner: ops.OwnerDesktop, Effect: ops.EffectWrite, Toolset: Toolset, Caller: ops.CallerRuleSessionOrGlobal,
	Summary: "Write a document",
	Guidance: "Create or replace a whole markdown (.md) or html (.html) document in Hive Drive. Folders in the path are " +
		"created as needed, so file a plan as plans/auth-rewrite.md or a report as reports/2026-10/perf.md. A markdown " +
		"body may open with YAML front matter between --- lines; title sets the name the user sees, and front matter that " +
		"does not parse is refused rather than saved. An html document renders as a page of its own in a sandboxed frame: " +
		"inline styles and scripts run; other scripts, styles and fetches load only over https; images, fonts and media " +
		"load over https or as data: URLs; forms and frames do not work; a #fragment link scrolls the page and a relative " +
		"link does nothing. It is not sanitized like a canvas html block, " +
		"and the canvas hv- classes do not apply. The body is capped at 2 MiB. A .canvas.json path is refused: write " +
		"canvas blocks with canvas_blocks_put. Answers with the entry as stored. No Drive tab opens by itself; use " +
		"tab_open when the user should look now.",
	Args: []string{"path"},
}

type DocumentMoveIn struct {
	Caller string `json:"caller" jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which uses the one shared space every such agent shares. Never guess or reuse another value."`
	From   string `json:"from"   jsonschema:"The document or folder to move, by its path in the space."`
	To     string `json:"to"     jsonschema:"Its new path. Must not exist; a document keeps its extension."`
}

var DocumentMove = ops.Spec[DocumentMoveIn]{
	Name: "document.move", Owner: ops.OwnerDesktop, Effect: ops.EffectWrite, Toolset: Toolset, Caller: ops.CallerRuleSessionOrGlobal,
	Summary: "Move a document",
	Guidance: "Rename or move one document, a canvas included, or a whole folder inside this space, creating the folders " +
		"above the target. The target must not exist, and a document keeps its extension. Answers with the entry at its new " +
		"path. Only move what you wrote or were asked to move; the user may have filed the rest on purpose.",
	Args: []string{"from", "to"},
}

type DocumentDeleteIn struct {
	Caller string `json:"caller" jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which uses the one shared space every such agent shares. Never guess or reuse another value."`
	Path   string `json:"path"   jsonschema:"The document, or empty folder, to delete."`
}

var DocumentDelete = ops.Spec[DocumentDeleteIn]{
	Name: "document.delete", Owner: ops.OwnerDesktop, Effect: ops.EffectWrite, Toolset: Toolset, Caller: ops.CallerRuleSessionOrGlobal,
	Summary: "Delete a document",
	Guidance: "Delete one document, a canvas included, or an empty folder. A folder with anything in it is refused; there " +
		"is no recursive delete and no trash. Answers with the path it deleted, deleted true, and when. Only delete what you made obsolete " +
		"yourself; the user may be keeping the rest.",
	Args: []string{"path"},
}

type CanvasListIn struct {
	Caller string `json:"caller" jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which files the canvas in one namespace every such agent shares. Never guess or reuse another value."`
}

var CanvasList = ops.Spec[CanvasListIn]{
	Name: "canvas.list", Owner: ops.OwnerDesktop, Effect: ops.EffectRead, Toolset: Toolset, Caller: ops.CallerRuleSessionOrGlobal,
	Summary: "List the canvases",
	Guidance: "List every canvas where yours are kept — this chat's workspace, the repository of this hive " +
		"session, or the shared space when caller is global — most recently updated first: name, title, block count and which chat or session created it. " +
		"Canvases outlive what made them, so this may include artifacts from earlier conversations — read one " +
		"before assuming it is yours to overwrite.",
}

// CanvasIn names one canvas. canvas.get, canvas.clear and canvas.delete take it.
type CanvasIn struct {
	Caller string `json:"caller" jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which files the canvas in one namespace every such agent shares. Never guess or reuse another value."`
	Canvas string `json:"canvas" jsonschema:"The canvas name or path. A bare name like perf files at canvases/perf.canvas.json; a path like reports/2026-10/perf files at reports/2026-10/perf.canvas.json, creating the folders. The last segment is short, lowercase and filename-like (letters, digits, dots, hyphens, underscores; starts and ends alphanumeric). Name or path is the canvas's identity in its space."`
}

var CanvasGet = ops.Spec[CanvasIn]{
	Name: "canvas.get", Owner: ops.OwnerDesktop, Effect: ops.EffectRead, Toolset: Toolset, Caller: ops.CallerRuleSessionOrGlobal,
	Summary: "Read a canvas",
	Guidance: "Read one canvas exactly as the user sees it: its editable front matter and every block in order, " +
		"with your ids, kinds and content — an html block comes back as the markup you wrote, before the app sanitizes it for display. A name " +
		"nothing was written under is not_found — use canvas_list to see what exists. Use this to re-orient after " +
		"a long conversation instead of assuming what you last wrote.",
	Args: []string{"canvas"},
}

var CanvasClear = ops.Spec[CanvasIn]{
	Name: "canvas.clear", Owner: ops.OwnerDesktop, Effect: ops.EffectWrite, Toolset: Toolset, Caller: ops.CallerRuleSessionOrGlobal,
	Summary: "Clear a canvas",
	Guidance: "Remove every block from one canvas at once and answer with its now-empty metadata. The canvas, its " +
		"name and its title survive — canvas_block_put after a clear starts a fresh layout in the same canvas. Prefer " +
		"canvas_block_put with stable ids for revisions; clear is for abandoning a layout wholesale, and canvas_delete for " +
		"discarding the canvas itself.",
	Args: []string{"canvas"},
}

var CanvasDelete = ops.Spec[CanvasIn]{
	Name: "canvas.delete", Owner: ops.OwnerDesktop, Effect: ops.EffectWrite, Toolset: Toolset, Caller: ops.CallerRuleSessionOrGlobal,
	Summary: "Delete a canvas",
	Guidance: "Remove one canvas entirely — its file is deleted and it leaves the Drive " +
		"tab's picker. A name that does not exist is not_found. Answers with the canvas, deleted true, and when. " +
		"Only delete a canvas you made obsolete yourself; the user may be keeping the others.",
	Args: []string{"canvas"},
}

type CanvasBlockPutIn struct {
	Caller      string `json:"caller"                jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which files the canvas in one namespace every such agent shares. Never guess or reuse another value."`
	Canvas      string `json:"canvas"                jsonschema:"The canvas name or path. A bare name like perf files at canvases/perf.canvas.json; a path like reports/2026-10/perf files at reports/2026-10/perf.canvas.json, creating the folders. The last segment is short, lowercase and filename-like (letters, digits, dots, hyphens, underscores; starts and ends alphanumeric). Name or path is the canvas's identity in its space."`
	CanvasTitle string `json:"canvasTitle,omitempty" jsonschema:"Display title for the whole canvas, shown in the Drive tab's picker. Set it on the canvas's first write; a later non-empty value renames, empty leaves the stored title unchanged."`
	Before      string `json:"before,omitempty"      jsonschema:"An existing block id to place this block ahead of — inserting a new id there, or moving a reused one (its createdAt survives the move). Omit to keep a reused id's position or append a new one."`
	ID          string `json:"id"                    jsonschema:"Your name for the block. Reusing an id updates that block in place; a new id appends."`
	Kind        string `json:"kind"                  jsonschema:"markdown, html or link."`
	Title       string `json:"title,omitempty"       jsonschema:"Heading shown above a markdown or html body (optional); the visible text of a link (required)."`
	Body        string `json:"body,omitempty"        jsonschema:"The markdown source of a markdown block, which may include standalone fenced mermaid diagrams, or the markup of an html block."`
	URL         string `json:"url,omitempty"         jsonschema:"The target of a link block; http, https or mailto only."`
}

var CanvasBlockPut = ops.Spec[CanvasBlockPutIn]{
	Name: "canvas_block.put", Owner: ops.OwnerDesktop, Effect: ops.EffectWrite, Toolset: Toolset, Caller: ops.CallerRuleSessionOrGlobal,
	Summary: "Put a canvas block",
	Guidance: "Create or replace one block on a named canvas. The first canvas_block_put under a new canvas name creates " +
		"that canvas; a workspace keeps as many as you make, so name canvases by artifact (release-notes, " +
		"perf-report), not by conversation. You choose the block id too: reusing an id updates that block in place, " +
		"keeping its position, while a new id appends at the end — stable ids are how you revise a status line " +
		"instead of stacking copies; before places or moves a block ahead of an existing one instead. kind is " +
		"markdown (body required, title optional; rendered as GitHub-flavored markdown with raw HTML escaped, not " +
		"rendered; a standalone fenced mermaid code block renders as a diagram), html (body required; semantic markup " +
		"laid out with the app's hv- classes — read the hive-drive docs first, since an unknown tag, class or attribute is " +
		"refused rather than dropped) or link (title and url " +
		"required; http, https or mailto only). Answers with the canvas metadata and the stored block; canvas_get " +
		"returns the full surface. No Drive tab opens by itself: a write while one " +
		"is open behind another tab lights a dot on it, and with none open it shows nothing until the user opens " +
		"Drive. Use tab_open when the result deserves the user's attention now. Writing a first layout of several blocks? canvas_blocks_put does it in one call.",
	Args:   []string{"canvas", "id"},
	Fields: ops.Constraints{"kind": {Enum: BlockKinds}},
}

type Block struct {
	ID    string `json:"id"              jsonschema:"Your name for the block. Reusing an id updates that block in place; a new id appends."`
	Kind  string `json:"kind"            jsonschema:"markdown, html or link."`
	Title string `json:"title,omitempty" jsonschema:"Heading shown above a markdown or html body (optional); the visible text of a link (required)."`
	Body  string `json:"body,omitempty"  jsonschema:"The markdown source of a markdown block, which may include standalone fenced mermaid diagrams, or the markup of an html block."`
	URL   string `json:"url,omitempty"   jsonschema:"The target of a link block; http, https or mailto only."`
}

type CanvasBlocksPutIn struct {
	Caller      string  `json:"caller"                jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which files the canvas in one namespace every such agent shares. Never guess or reuse another value."`
	Canvas      string  `json:"canvas"                jsonschema:"The canvas name or path. A bare name like perf files at canvases/perf.canvas.json; a path like reports/2026-10/perf files at reports/2026-10/perf.canvas.json, creating the folders. The last segment is short, lowercase and filename-like (letters, digits, dots, hyphens, underscores; starts and ends alphanumeric). Name or path is the canvas's identity in its space."`
	CanvasTitle string  `json:"canvasTitle,omitempty" jsonschema:"Display title for the whole canvas, shown in the Drive tab's picker. Set it on the canvas's first write; a later non-empty value renames, empty leaves the stored title unchanged."`
	Blocks      []Block `json:"blocks"                jsonschema:"The blocks to write, applied in order (50 max per call)."`
}

var CanvasBlocksPut = ops.Spec[CanvasBlocksPutIn]{
	Name: "canvas_blocks.put", Owner: ops.OwnerDesktop, Effect: ops.EffectWrite, Toolset: Toolset, Caller: ops.CallerRuleSessionOrGlobal,
	Summary: "Put a batch of canvas blocks",
	Guidance: "Write several blocks to one canvas in a single atomic call — one write, one render, so an " +
		"initial layout appears whole instead of assembling block by block. Blocks follow canvas_block_put's rules and " +
		"apply in order; every block is validated first, so a rejected batch leaves the canvas untouched. Answers " +
		"with the canvas metadata. For revising one block, prefer canvas_block_put.",
	Args: []string{"canvas"},
}

type CanvasBlockDeleteIn struct {
	Caller string `json:"caller" jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which files the canvas in one namespace every such agent shares. Never guess or reuse another value."`
	Canvas string `json:"canvas" jsonschema:"The canvas name or path. A bare name like perf files at canvases/perf.canvas.json; a path like reports/2026-10/perf files at reports/2026-10/perf.canvas.json, creating the folders. The last segment is short, lowercase and filename-like (letters, digits, dots, hyphens, underscores; starts and ends alphanumeric). Name or path is the canvas's identity in its space."`
	ID     string `json:"id"     jsonschema:"The id of the block to remove, as given to canvas_block_put."`
}

var CanvasBlockDelete = ops.Spec[CanvasBlockDeleteIn]{
	Name: "canvas_block.delete", Owner: ops.OwnerDesktop, Effect: ops.EffectWrite, Toolset: Toolset, Caller: ops.CallerRuleSessionOrGlobal,
	Summary: "Remove a canvas block",
	Guidance: "Remove one block by the id you gave it in canvas_block_put, and answer with the canvas metadata that " +
		"remains. An id that is not on the canvas is not_found — nothing is ever reported removed that was not there.",
	Args: []string{"canvas", "id"},
}

type CanvasFrontmatterPutIn struct {
	Caller      string         `json:"caller"      jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which files the canvas in one namespace every such agent shares. Never guess or reuse another value."`
	Canvas      string         `json:"canvas"      jsonschema:"The existing canvas whose editable front matter to replace, by name or path."`
	Frontmatter map[string]any `json:"frontmatter" jsonschema:"The complete editable front matter. Keys use letters, digits, dots, hyphens, or underscores. Values are strings, finite numbers, booleans, null, or flat lists of those values. created_at and updated_at are reserved and populated by Hive."`
}

var CanvasFrontmatterPut = ops.Spec[CanvasFrontmatterPutIn]{
	Name: "canvas_frontmatter.put", Owner: ops.OwnerDesktop, Effect: ops.EffectWrite, Toolset: Toolset, Caller: ops.CallerRuleSessionOrGlobal,
	Summary: "Set canvas front matter",
	Guidance: "Replace all editable front matter on an existing canvas. Use short keys and scalar values or " +
		"lists of scalars; tags is conventionally a list of strings. Hive owns created_at and updated_at, updates " +
		"them automatically, and refuses either key here. Pass an empty object to remove all editable fields. " +
		"Copy and save include the resulting YAML front matter. Answers with the canvas metadata.",
	Args: []string{"canvas"},
}
