package mcpsrv

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (ctrl *DriveController) registerTabs(srv *mcp.Server) {
	addTool(srv, &mcp.Tool{
		Name:  "open_tab",
		Title: "Open a tab beside the session",
		Description: "Ask Hive to open a tab beside this chat or Code session. view is drive (the session's documents and " +
			"canvases), diff (the Code session's changes) or tasks (its task board). It applies only while the user is " +
			"viewing this chat or session, and there is no acknowledgment. A chat has only drive; diff and tasks belong to a " +
			"Code session. The shared space (global) sits beside no session, so it has no tabs; the user reads it in the " +
			"Drive page. path, for drive only, shows one document by its path in the space, or a canvas by name or path " +
			"(perf is canvases/perf.canvas.json); it must exist. Omit it to leave the tab on its own pick. placement " +
			"behind (the default) adds the tab behind the tab in front, with an unseen dot, and never takes the screen; " +
			"beside splits the panel in front and opens the tab next to it, so the user sees it at once. Both leave the " +
			"keyboard where it is, and a tab that is already open stays where it is under either. Call it when you finish " +
			"something worth looking at now, not on every write: a write while a Drive tab is open behind another tab " +
			"already lights a dot on it. Use beside only for a result the user asked to see side by side with the terminal.",
	}, ctrl.OpenTab)

	addTool(srv, &mcp.Tool{
		Name:  "close_tab",
		Title: "Close a tab",
		Description: "Ask Hive to close this chat or Code session's tab for a view (drive, diff or tasks), wherever it sits. " +
			"Like open_tab it applies only while the user is viewing this chat or session. Documents and canvases are " +
			"untouched; this closes the tab, not the content.",
	}, ctrl.CloseTab)
}

type openTabInput struct {
	Session   string `json:"session"             jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which uses the one shared space every such agent shares. Never guess or reuse another value."`
	View      string `json:"view"                jsonschema:"drive, diff or tasks. Drive is the session's documents and canvases; Diff is the Code session's changes; Tasks is its task board."`
	Path      string `json:"path,omitempty"      jsonschema:"Drive only: the document to show, by its path in the space, or a canvas by name or path (perf is canvases/perf.canvas.json). It must exist. Omit to leave the tab on its own pick."`
	Placement string `json:"placement,omitempty" jsonschema:"behind (default) adds the tab behind the tab in front, with an unseen dot, and never takes the screen. beside splits the panel in front and opens the tab next to it. A tab that is already open stays where it is."`
}

type closeTabInput struct {
	Session string `json:"session" jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which uses the one shared space every such agent shares. Never guess or reuse another value."`
	View    string `json:"view"    jsonschema:"drive, diff or tasks. The tab for this view closes wherever it sits."`
}

type openTabResult struct {
	Requested string `json:"requested" jsonschema:"The request handed to the UI: open. Delivery is best-effort - it applies only while the user is viewing this chat or session, and there is no acknowledgment either way."`
	View      string `json:"view"      jsonschema:"The view asked for: drive, diff or tasks."`
	Path      string `json:"path"      jsonschema:"The document or canvas path the tab was asked to show, resolved from path; empty when none was given."`
}

type closeTabResult struct {
	Requested string `json:"requested" jsonschema:"The request handed to the UI: close. Delivery is best-effort - it applies only while the user is viewing this chat or session, and there is no acknowledgment either way."`
	View      string `json:"view"      jsonschema:"The view asked to close: drive, diff or tasks."`
}

func (ctrl *DriveController) OpenTab(ctx context.Context, _ *mcp.CallToolRequest, in openTabInput) (*mcp.CallToolResult, openTabResult, error) {
	path, err := ctrl.core.Drive.OpenTab(ctx, in.Session, in.View, in.Path, in.Placement)
	if err != nil {
		return nil, openTabResult{}, ctrl.toolError(err)
	}
	return nil, openTabResult{Requested: "open", View: in.View, Path: path}, nil
}

func (ctrl *DriveController) CloseTab(ctx context.Context, _ *mcp.CallToolRequest, in closeTabInput) (*mcp.CallToolResult, closeTabResult, error) {
	if err := ctrl.core.Drive.CloseTab(ctx, in.Session, in.View); err != nil {
		return nil, closeTabResult{}, ctrl.toolError(err)
	}
	return nil, closeTabResult{Requested: "close", View: in.View}, nil
}
