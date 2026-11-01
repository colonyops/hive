package mcpsrv

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

func (ctrl *DriveController) registerTabs(srv *mcp.Server) {
	addTool(srv, &mcp.Tool{
		Name:  "open_tab",
		Title: "Open a tab or terminal beside the session",
		Description: "Ask Hive to open a tab beside this chat or Code session. view is drive (the session's documents and " +
			"canvases), diff (the Code session's changes), tasks (its task board) or terminal (a tmux window of the Code " +
			"session). It applies only while the user is viewing this chat or session, and there is no acknowledgment. A " +
			"chat has only drive; diff, tasks and terminal belong to a Code session. The shared space (global) sits beside " +
			"no session, so it has no tabs; the user reads it in the Drive page. path, for drive only, shows one document " +
			"by its path in the space, or a canvas by name or path (perf is canvases/perf.canvas.json); it must exist. " +
			"Omit it to leave the tab on its own pick. terminal starts a new window in the session's checkout, or places " +
			"one that exists. It opens detached, so it never takes the keyboard or the screen. window, for terminal only, " +
			"places an existing window of the session by tmux id (@3) or exact name instead of starting one, and excludes " +
			"name and command. name, for terminal only, names the new window. command, for terminal only, runs in the " +
			"checkout, and the window stays as a shell after it exits so the user can read the output. Use command for " +
			"something the user will watch, like a test watcher or a log tail, not for work you can run yourself. The " +
			"session must be running; start it in Code first. placement behind (the default) adds the tab behind the tab " +
			"in front, with an unseen dot, and never takes the screen; beside splits the panel in front and opens the tab " +
			"next to it, so the user sees it at once. Both leave the keyboard where it is, and a tab that is already open " +
			"stays where it is under either. Call it when you finish something worth looking at now, not on every write: " +
			"a write while a Drive tab is open behind another tab already lights a dot on it. Use beside only for a " +
			"result the user asked to see side by side with the terminal.",
	}, ctrl.OpenTab)

	addTool(srv, &mcp.Tool{
		Name:  "close_tab",
		Title: "Close a tab",
		Description: "Ask Hive to close this chat or Code session's tab for a view (drive, diff or tasks), wherever it " +
			"sits. Like open_tab it applies only while the user is viewing this chat or session. Documents and canvases " +
			"are untouched; this closes the tab, not the content. A terminal is not closable here: it closes from its tab " +
			"or its shell.",
	}, ctrl.CloseTab)
}

type openTabInput struct {
	Session   string `json:"session"             jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which uses the one shared space every such agent shares. Never guess or reuse another value."`
	View      string `json:"view"                jsonschema:"drive, diff, tasks or terminal. Drive is the session's documents and canvases; Diff is the Code session's changes; Tasks is its task board; Terminal is a tmux window of the Code session."`
	Path      string `json:"path,omitempty"      jsonschema:"Drive only: the document to show, by its path in the space, or a canvas by name or path (perf is canvases/perf.canvas.json). It must exist. Omit to leave the tab on its own pick."`
	Placement string `json:"placement,omitempty" jsonschema:"behind (default) adds the tab behind the tab in front, with an unseen dot, and never takes the screen. beside splits the panel in front and opens the tab next to it. A tab that is already open stays where it is."`
	Window    string `json:"window,omitempty"    jsonschema:"Terminal only: an existing window of the session, by tmux id (@3) or exact window name. It is placed instead of starting one, so name and command must be empty."`
	Name      string `json:"name,omitempty"      jsonschema:"Terminal only: the name of the new window."`
	Command   string `json:"command,omitempty"   jsonschema:"Terminal only: a shell command the new window runs in the session's checkout. The window stays open as a shell after it exits, so the user can read the output. Use it for something the user will watch, like tests or a log, not for work you can run yourself."`
}

type closeTabInput struct {
	Session string `json:"session" jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which uses the one shared space every such agent shares. Never guess or reuse another value."`
	View    string `json:"view"    jsonschema:"drive, diff or tasks. The tab for this view closes wherever it sits. terminal is refused: a terminal closes from its tab or its shell."`
}

type openTabResult struct {
	Requested string `json:"requested" jsonschema:"The request handed to the UI: open. Delivery is best-effort - it applies only while the user is viewing this chat or session, and there is no acknowledgment either way."`
	View      string `json:"view"      jsonschema:"The view asked for: drive, diff, tasks or terminal."`
	Path      string `json:"path"      jsonschema:"The document or canvas path the tab was asked to show, resolved from path; empty when none was given."`
	Window    string `json:"window"    jsonschema:"The tmux window id (@3) the terminal tab was asked to show, new or existing; empty for the other views."`
}

type closeTabResult struct {
	Requested string `json:"requested" jsonschema:"The request handed to the UI: close. Delivery is best-effort - it applies only while the user is viewing this chat or session, and there is no acknowledgment either way."`
	View      string `json:"view"      jsonschema:"The view asked to close: drive, diff or tasks."`
}

func (ctrl *DriveController) OpenTab(ctx context.Context, _ *mcp.CallToolRequest, in openTabInput) (*mcp.CallToolResult, openTabResult, error) {
	res, err := ctrl.core.Drive.OpenTab(ctx, app.OpenTabRequest{
		Session: in.Session, View: in.View, Path: in.Path, Placement: in.Placement,
		Window: in.Window, Name: in.Name, Command: in.Command,
	})
	if err != nil {
		return nil, openTabResult{}, ctrl.toolError(err)
	}
	return nil, openTabResult{Requested: "open", View: in.View, Path: res.Path, Window: res.Window}, nil
}

func (ctrl *DriveController) CloseTab(ctx context.Context, _ *mcp.CallToolRequest, in closeTabInput) (*mcp.CallToolResult, closeTabResult, error) {
	if err := ctrl.core.Drive.CloseTab(ctx, in.Session, in.View); err != nil {
		return nil, closeTabResult{}, ctrl.toolError(err)
	}
	return nil, closeTabResult{Requested: "close", View: in.View}, nil
}
