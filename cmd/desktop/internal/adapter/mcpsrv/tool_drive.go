package mcpsrv

import (
	"cmp"
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/colonyops/hive/internal/domain/drive"
	driveops "github.com/colonyops/hive/internal/ops/drive"
	"github.com/colonyops/hive/internal/ops/mcpops"
)

func (ctrl *DriveController) registerDocuments(srv *mcp.Server) {
	mcpops.Mount(srv, driveops.DocumentList, ctrl.ListDocuments, ctrl.toolError)
	mcpops.Mount(srv, driveops.DocumentGet, ctrl.ReadDocument, ctrl.toolError)
	mcpops.Mount(srv, driveops.DocumentPut, ctrl.WriteDocument, ctrl.toolError)
	mcpops.Mount(srv, driveops.DocumentMove, ctrl.MoveDocument, ctrl.toolError)
	mcpops.Mount(srv, driveops.DocumentDelete, ctrl.DeleteDocument, ctrl.toolError)
}

type documentEntry struct {
	Kind     string         `json:"kind"            jsonschema:"dir, markdown, html, or file."`
	Path     string         `json:"path"            jsonschema:"Path inside the space, forward slashes; empty for the space root."`
	Name     string         `json:"name"`
	Title    string         `json:"title,omitempty" jsonschema:"The title key of a markdown document's front matter."`
	Size     int64          `json:"size"            jsonschema:"Bytes on disk; 0 for a folder."`
	Modified time.Time      `json:"modified"`
	Front    map[string]any `json:"front,omitempty" jsonschema:"Front matter of a markdown document."`
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
	Path    string    `json:"path"    jsonschema:"The path that was deleted."`
	Deleted bool      `json:"deleted" jsonschema:"Always true: a delete that did not happen is an error."`
	At      time.Time `json:"at"      jsonschema:"When it was deleted."`
}

func (ctrl *DriveController) ListDocuments(ctx context.Context, in driveops.DocumentListIn) (listDocumentsResult, error) {
	detail := cmp.Or(in.Detail, driveops.DetailChildren)
	space, entry, err := ctrl.core.Drive.ListDocuments(ctx, in.Caller, in.Path, detail == driveops.DetailTree)
	if err != nil {
		return listDocumentsResult{}, err
	}
	children := make([]any, 0, len(entry.Children))
	for _, child := range entry.Children {
		children = append(children, documentNodeFrom(child))
	}
	return listDocumentsResult{Space: space.Key, documentEntry: documentEntryFrom(entry), Children: children, Detail: detail}, nil
}

func (ctrl *DriveController) ReadDocument(ctx context.Context, in driveops.DocumentGetIn) (readDocumentResult, error) {
	entry, body, err := ctrl.core.Drive.ReadDocument(ctx, in.Caller, in.Path)
	if err != nil {
		return readDocumentResult{}, err
	}
	return readDocumentResult{documentEntry: documentEntryFrom(entry), Body: string(body)}, nil
}

func (ctrl *DriveController) WriteDocument(ctx context.Context, in driveops.DocumentPutIn) (documentEntry, error) {
	entry, err := ctrl.core.Drive.WriteDocument(ctx, in.Caller, in.Path, []byte(in.Body))
	if err != nil {
		return documentEntry{}, err
	}
	return documentEntryFrom(entry), nil
}

func (ctrl *DriveController) MoveDocument(ctx context.Context, in driveops.DocumentMoveIn) (documentEntry, error) {
	entry, err := ctrl.core.Drive.MoveDocument(ctx, in.Caller, in.From, in.To)
	if err != nil {
		return documentEntry{}, err
	}
	return documentEntryFrom(entry), nil
}

func (ctrl *DriveController) DeleteDocument(ctx context.Context, in driveops.DocumentDeleteIn) (deleteDocumentResult, error) {
	if err := ctrl.core.Drive.DeleteDocument(ctx, in.Caller, in.Path); err != nil {
		return deleteDocumentResult{}, err
	}
	return deleteDocumentResult{Path: in.Path, Deleted: true, At: time.Now()}, nil
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
