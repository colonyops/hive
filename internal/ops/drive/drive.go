// Package drive declares the Hive Drive document operations,
// served on the drive MCP server.
package drive

import "github.com/colonyops/hive/internal/ops"

var Toolset = ops.Toolset{Name: "drive", MCPPath: "/mcp/drive"}

// The document_list detail ladder. A space is as large as its owner made it,
// so the default answers one level and the whole subtree is asked for.
const (
	DetailChildren = "children"
	DetailTree     = "tree"
)

func All() []ops.Op {
	return []ops.Op{
		DocumentList, DocumentGet, DocumentPut, DocumentMove, DocumentDelete,
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
		"or the shared space for global. Each entry has a kind (dir, markdown, html, or file for anything else), its path " +
		"inside the space, name, title, size and modified time; markdown entries carry their front matter as front. " +
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
		"included, with its entry. A folder is listed with document_list, and a binary file is refused. Use this before rewriting a document another chat or the " +
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
		"link does nothing. The body is capped at 2 MiB. Answers with the entry as stored. No Drive tab opens by itself; use " +
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
	Guidance: "Rename or move one document or a whole folder inside this space, creating the folders " +
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
	Guidance: "Delete one document or an empty folder. A folder with anything in it is refused; there " +
		"is no recursive delete and no trash. Answers with the path it deleted, deleted true, and when. Only delete what you made obsolete " +
		"yourself; the user may be keeping the rest.",
	Args: []string{"path"},
}
