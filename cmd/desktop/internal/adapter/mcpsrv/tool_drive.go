package mcpsrv

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/internal/domain/drive"
)

// The list_documents detail ladder. A space is as large as its owner made it,
// so the default answers one level and the whole subtree is asked for.
const (
	detailChildren = "children"
	detailTree     = "tree"
)

func (ctrl *DriveController) registerDocuments(srv *mcp.Server) {
	addTool(srv, &mcp.Tool{
		Name:  "list_documents",
		Title: "List documents",
		Description: "List a folder of Hive Drive, the document tree the user sees in the Drive tab of this chat or Code session. " +
			"Your session decides the space: a chat's workspace, a hive session's repository (the folder a checkout links as .hive), " +
			"or the shared space for global. Each entry has a kind (dir, markdown, html, canvas, or file for anything else), its path " +
			"inside the space, name, title, size and modified time; markdown and canvas entries carry their front matter as front. " +
			"An empty path is the space root. detail defaults to children, one level down; pass tree for the whole subtree. " +
			"A folder that does not exist is not_found; a document path lists that document alone.",
	}, ctrl.ListDocuments)

	addTool(srv, &mcp.Tool{
		Name:  "read_document",
		Title: "Read a document",
		Description: "Read one markdown or html document, or any other UTF-8 text file in the space, as stored, front matter " +
			"included, with its entry. A canvas is blocks rather than a body: read it with read_canvas. A folder is listed " +
			"with list_documents, and a binary file is refused. Use this before rewriting a document another chat or the " +
			"user may have changed.",
	}, ctrl.ReadDocument)

	addTool(srv, &mcp.Tool{
		Name:  "write_document",
		Title: "Write a document",
		Description: "Create or replace a whole markdown (.md) or html (.html) document in Hive Drive. Folders in the path are " +
			"created as needed, so file a plan as plans/auth-rewrite.md or a report as reports/2026-10/perf.md. A markdown " +
			"body may open with YAML front matter between --- lines; title sets the name the user sees, and front matter that " +
			"does not parse is refused rather than saved. An html document renders as a page of its own in a sandboxed frame: " +
			"inline styles and scripts run; other scripts, styles and fetches load only over https; images, fonts and media " +
			"load over https or as data: URLs; forms and frames do not work; a #fragment link scrolls the page and a relative " +
			"link does nothing. It is not sanitized like a canvas html block, " +
			"and the canvas hv- classes do not apply. The body is capped at 2 MiB. A .canvas.json path is refused: write " +
			"canvas blocks with put_blocks. Answers with the entry as stored. No Drive tab opens by itself; use " +
			"open_document when the user should look now.",
	}, ctrl.WriteDocument)

	addTool(srv, &mcp.Tool{
		Name:  "move_document",
		Title: "Move a document",
		Description: "Rename or move one document, a canvas included, or a whole folder inside this space, creating the folders " +
			"above the target. The target must not exist, and a document keeps its extension. Answers with the entry at its new " +
			"path. Only move what you wrote or were asked to move; the user may have filed the rest on purpose.",
	}, ctrl.MoveDocument)

	addTool(srv, &mcp.Tool{
		Name:  "delete_document",
		Title: "Delete a document",
		Description: "Delete one document, a canvas included, or an empty folder. A folder with anything in it is refused; there " +
			"is no recursive delete and no trash. Answers with the path it deleted. Only delete what you made obsolete " +
			"yourself; the user may be keeping the rest.",
	}, ctrl.DeleteDocument)

	addTool(srv, &mcp.Tool{
		Name:  "open_document",
		Title: "Open a document in the Drive tab",
		Description: "Ask Hive to show a document in the Drive tab of this chat or Code session when you give its path (it must " +
			"exist). With no Drive tab open, Hive adds one behind the tab in front, with an unseen dot; it never takes the " +
			"screen from what the user is doing. Like open_canvas it applies only while the user is viewing this chat or " +
			"session. Call it when you finish something worth reading now, not on every write. The shared space has no " +
			"Drive tab to open; the user reads it in the Drive page.",
	}, ctrl.OpenDocument)
}

type listDocumentsInput struct {
	Session string `json:"session"          jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which uses the one shared space every such agent shares. Never guess or reuse another value."`
	Path    string `json:"path,omitempty"   jsonschema:"Folder to list, forward slashes, relative to the space root; empty is the space root."`
	Detail  string `json:"detail,omitempty" jsonschema:"children (default) lists one level; tree lists the whole subtree."`
}

type readDocumentInput struct {
	Session string `json:"session" jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which uses the one shared space every such agent shares. Never guess or reuse another value."`
	Path    string `json:"path"    jsonschema:"Document path inside the space, forward slashes; it must exist."`
}

type writeDocumentInput struct {
	Session string `json:"session" jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which uses the one shared space every such agent shares. Never guess or reuse another value."`
	Path    string `json:"path"    jsonschema:"Document path inside the space, forward slashes, ending in .md or .html; folders are created as needed."`
	Body    string `json:"body"    jsonschema:"The whole document. Markdown may start with YAML front matter."`
}

type moveDocumentInput struct {
	Session string `json:"session" jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which uses the one shared space every such agent shares. Never guess or reuse another value."`
	From    string `json:"from"    jsonschema:"The document or folder to move, by its path in the space."`
	To      string `json:"to"      jsonschema:"Its new path. Must not exist; a document keeps its extension."`
}

type deleteDocumentInput struct {
	Session string `json:"session" jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which uses the one shared space every such agent shares. Never guess or reuse another value."`
	Path    string `json:"path"    jsonschema:"The document, or empty folder, to delete."`
}

type openDocumentInput struct {
	Session string `json:"session"        jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which uses the one shared space every such agent shares. Never guess or reuse another value."`
	Path    string `json:"path,omitempty" jsonschema:"Document to show, by its path in the space; must exist. Omit to leave the Drive tab on its own pick."`
}

type documentEntry struct {
	Kind     string         `json:"kind"            jsonschema:"dir, markdown, html, canvas, or file."`
	Path     string         `json:"path"            jsonschema:"Path inside the space, forward slashes; empty for the space root."`
	Name     string         `json:"name"`
	Title    string         `json:"title,omitempty" jsonschema:"A canvas's title, or the title key of a markdown document's front matter."`
	Size     int64          `json:"size"            jsonschema:"Bytes on disk; 0 for a folder."`
	Modified time.Time      `json:"modified"`
	Front    map[string]any `json:"front,omitempty" jsonschema:"Front matter of a markdown or canvas document."`
}

// documentNode is a listed entry with its own children. The result schema
// types children loosely because schema inference refuses a recursive type.
type documentNode struct {
	documentEntry
	Children []documentNode `json:"children,omitempty"`
}

type listDocumentsResult struct {
	Space string `json:"space" jsonschema:"The space listed: owner/repo, workspaces/<name>, or shared."`
	documentEntry
	Children []any  `json:"children" jsonschema:"Entries inside a folder, folders first, each with kind, path, name, title, size, modified and front, and its own children under detail tree."`
	Detail   string `json:"detail"   jsonschema:"The level this answer used: children or tree."`
}

type readDocumentResult struct {
	documentEntry
	Body string `json:"body" jsonschema:"The document as stored, front matter included."`
}

type deleteDocumentResult struct {
	Deleted string `json:"deleted" jsonschema:"The path that was deleted."`
}

func (ctrl *DriveController) ListDocuments(ctx context.Context, _ *mcp.CallToolRequest, in listDocumentsInput) (*mcp.CallToolResult, listDocumentsResult, error) {
	if in.Detail == "" {
		in.Detail = detailChildren
	}
	if in.Detail != detailChildren && in.Detail != detailTree {
		return nil, listDocumentsResult{}, ctrl.toolError(app.Errorf(app.KindInvalid, "detail must be children or tree"))
	}
	space, entry, err := ctrl.core.Drive.ListDocuments(ctx, in.Session, in.Path, in.Detail == detailTree)
	if err != nil {
		return nil, listDocumentsResult{}, ctrl.toolError(err)
	}
	children := make([]any, 0, len(entry.Children))
	for _, child := range entry.Children {
		children = append(children, documentNodeFrom(child))
	}
	return nil, listDocumentsResult{Space: space.Key, documentEntry: documentEntryFrom(entry), Children: children, Detail: in.Detail}, nil
}

func (ctrl *DriveController) ReadDocument(ctx context.Context, _ *mcp.CallToolRequest, in readDocumentInput) (*mcp.CallToolResult, readDocumentResult, error) {
	entry, body, err := ctrl.core.Drive.ReadDocument(ctx, in.Session, in.Path)
	if err != nil {
		return nil, readDocumentResult{}, ctrl.toolError(err)
	}
	return nil, readDocumentResult{documentEntry: documentEntryFrom(entry), Body: string(body)}, nil
}

func (ctrl *DriveController) WriteDocument(ctx context.Context, _ *mcp.CallToolRequest, in writeDocumentInput) (*mcp.CallToolResult, documentEntry, error) {
	entry, err := ctrl.core.Drive.WriteDocument(ctx, in.Session, in.Path, []byte(in.Body))
	if err != nil {
		return nil, documentEntry{}, ctrl.toolError(err)
	}
	return nil, documentEntryFrom(entry), nil
}

func (ctrl *DriveController) MoveDocument(ctx context.Context, _ *mcp.CallToolRequest, in moveDocumentInput) (*mcp.CallToolResult, documentEntry, error) {
	entry, err := ctrl.core.Drive.MoveDocument(ctx, in.Session, in.From, in.To)
	if err != nil {
		return nil, documentEntry{}, ctrl.toolError(err)
	}
	return nil, documentEntryFrom(entry), nil
}

func (ctrl *DriveController) DeleteDocument(ctx context.Context, _ *mcp.CallToolRequest, in deleteDocumentInput) (*mcp.CallToolResult, deleteDocumentResult, error) {
	if err := ctrl.core.Drive.DeleteDocument(ctx, in.Session, in.Path); err != nil {
		return nil, deleteDocumentResult{}, ctrl.toolError(err)
	}
	return nil, deleteDocumentResult{Deleted: in.Path}, nil
}

func (ctrl *DriveController) OpenDocument(ctx context.Context, _ *mcp.CallToolRequest, in openDocumentInput) (*mcp.CallToolResult, driveOpenResult, error) {
	if err := ctrl.core.Drive.OpenDocument(ctx, in.Session, in.Path); err != nil {
		return nil, driveOpenResult{}, ctrl.toolError(err)
	}
	return nil, driveOpenResult{Requested: "open"}, nil
}

func documentEntryFrom(e drive.Entry) documentEntry {
	return documentEntry{
		Kind: e.Kind.String(), Path: e.Path, Name: e.Name, Title: e.Title,
		Size: e.Size, Modified: e.Modified, Front: e.Front,
	}
}

func documentNodeFrom(e drive.Entry) documentNode {
	node := documentNode{documentEntry: documentEntryFrom(e)}
	for _, child := range e.Children {
		node.Children = append(node.Children, documentNodeFrom(child))
	}
	return node
}
