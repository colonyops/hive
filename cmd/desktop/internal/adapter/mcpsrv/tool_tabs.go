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
			"next to it, and below splits it downward, so the user sees it at once. All leave the keyboard where it is, " +
			"and a tab that is already open stays where it is under any. anchor, a tab reference (drive, diff, tasks, " +
			"terminal, or a window by id or name), is the tab to place relative to instead of the tab in front of the " +
			"focused panel. Call it when you finish something worth looking at now, not on every write: " +
			"a write while a Drive tab is open behind another tab already lights a dot on it. Use beside or below only " +
			"for a result the user asked to see side by side with the terminal. If you are not sure what is open, call " +
			"get_layout first.",
	}, ctrl.OpenTab)

	addTool(srv, &mcp.Tool{
		Name:  "close_tab",
		Title: "Close a tab",
		Description: "Ask Hive to close this chat or Code session's tab for a view (drive, diff or tasks), wherever it " +
			"sits. Like open_tab it applies only while the user is viewing this chat or session. Documents and canvases " +
			"are untouched; this closes the tab, not the content. view terminal needs window (a tmux window of the Code " +
			"session, by id or exact name) and kills that window through tmux: it ends what runs in it, and the tab goes " +
			"with it. Only close a window you started. A chat's terminal is the chat, so close_tab refuses it.",
	}, ctrl.CloseTab)

	addTool(srv, &mcp.Tool{
		Name:  "move_tab",
		Title: "Move a tab",
		Description: "Ask Hive to move a tab within this chat or Code session's panels. tab is a tab reference. placement " +
			"behind (the default) puts it in the anchor's panel right after the anchor, or at the end of the focused panel " +
			"with no anchor. beside and below split the anchor's panel (or the focused one) and move the tab into the new " +
			"panel. It applies only while the user is viewing this chat or session, and takes neither the keyboard nor " +
			"the focus.",
	}, ctrl.MoveTab)

	addTool(srv, &mcp.Tool{
		Name:  "focus_tab",
		Title: "Bring a tab to the front",
		Description: "Ask Hive to bring a tab forward and focus its panel; a terminal window is also selected in tmux. " +
			"This one takes the screen: use it when the user asked to see something, not to show off a result. For a result " +
			"use open_tab, which leaves the focus where it is.",
	}, ctrl.FocusTab)

	addTool(srv, &mcp.Tool{
		Name:  "close_panel",
		Title: "Close a panel",
		Description: "Ask Hive to close the panel that holds a tab. Its tabs join the neighbouring panel and none of them " +
			"close. The last panel stays.",
	}, ctrl.ClosePanel)

	addTool(srv, &mcp.Tool{
		Name:  "resize_split",
		Title: "Resize a split",
		Description: "Ask Hive to resize the nearest split of the panel that holds a tab. share, from 0.2 to 0.8, is the " +
			"share of the split that the side holding the tab gets.",
	}, ctrl.ResizeSplit)

	addTool(srv, &mcp.Tool{
		Name:        "turn_split",
		Title:       "Turn a split",
		Description: "Ask Hive to turn the nearest split of the panel that holds a tab between side by side and stacked.",
	}, ctrl.TurnSplit)

	addTool(srv, &mcp.Tool{
		Name:  "get_layout",
		Title: "Read the panel layout",
		Description: "Read the panel layout of this chat or Code session as the app last reported it: each panel with its " +
			"tabs, which tab is shown, which panel has focus, and the splits that join the panels. Each tab is spelled the " +
			"way the tab tools take it, so you can pass it back. A Code session's terminal tabs carry the tmux window name. " +
			"known is false when the app has not reported this layout yet, for instance just after it started or when the " +
			"session has never been open in Code; then act without it. onScreen says whether the user is looking at this " +
			"session now. Call it first when you are not sure what is open.",
	}, ctrl.GetLayout)
}

type openTabInput struct {
	Session   string `json:"session"             jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which uses the one shared space every such agent shares. Never guess or reuse another value."`
	View      string `json:"view"                jsonschema:"drive, diff, tasks or terminal. Drive is the session's documents and canvases; Diff is the Code session's changes; Tasks is its task board; Terminal is a tmux window of the Code session."`
	Path      string `json:"path,omitempty"      jsonschema:"Drive only: the document to show, by its path in the space, or a canvas by name or path (perf is canvases/perf.canvas.json). It must exist. Omit to leave the tab on its own pick."`
	Placement string `json:"placement,omitempty" jsonschema:"behind (default) adds the tab behind the tab in front, with an unseen dot, and never takes the screen. beside splits the panel in front and opens the tab next to it; below splits it downward. A tab that is already open stays where it is."`
	Anchor    string `json:"anchor,omitempty"    jsonschema:"The tab to place relative to: drive, diff, tasks, terminal (a chat's one terminal), or a window of the Code session by id (@3) or exact name. Omit for the tab in front of the focused panel."`
	Window    string `json:"window,omitempty"    jsonschema:"Terminal only: an existing window of the session, by tmux id (@3) or exact window name. It is placed instead of starting one, so name and command must be empty."`
	Name      string `json:"name,omitempty"      jsonschema:"Terminal only: the name of the new window."`
	Command   string `json:"command,omitempty"   jsonschema:"Terminal only: a shell command the new window runs in the session's checkout. The window stays open as a shell after it exits, so the user can read the output. Use it for something the user will watch, like tests or a log, not for work you can run yourself."`
}

type closeTabInput struct {
	Session string `json:"session"          jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which uses the one shared space every such agent shares. Never guess or reuse another value."`
	View    string `json:"view"             jsonschema:"drive, diff, tasks or terminal. The tab for drive, diff or tasks closes wherever it sits. terminal kills the tmux window named by window."`
	Window  string `json:"window,omitempty" jsonschema:"Terminal only, and required for it: the window of the Code session to kill, by tmux id (@3) or exact name. It ends what runs in it. A chat's terminal is refused."`
}

type moveTabInput struct {
	Session   string `json:"session"             jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which uses the one shared space every such agent shares. Never guess or reuse another value."`
	Tab       string `json:"tab"                 jsonschema:"The tab to move. drive, diff or tasks name that view. terminal names a chat's one terminal. Anything else is a tmux window of a Code session, by id (@3) or exact name."`
	Placement string `json:"placement,omitempty" jsonschema:"behind (default) puts the tab in the anchor's panel right after the anchor, or at the end of the focused panel. beside and below split the anchor's panel (or the focused one) and move the tab into the new panel."`
	Anchor    string `json:"anchor,omitempty"    jsonschema:"The tab to place relative to, spelled like tab. Omit for the tab in front of the focused panel."`
}

type tabInput struct {
	Session string `json:"session" jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which uses the one shared space every such agent shares. Never guess or reuse another value."`
	Tab     string `json:"tab"     jsonschema:"A tab in the panel to act on. drive, diff or tasks name that view. terminal names a chat's one terminal. Anything else is a tmux window of a Code session, by id (@3) or exact name."`
}

type resizeSplitInput struct {
	Session string  `json:"session" jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which uses the one shared space every such agent shares. Never guess or reuse another value."`
	Tab     string  `json:"tab"     jsonschema:"A tab in the panel whose side of the split to size. drive, diff or tasks name that view. terminal names a chat's one terminal. Anything else is a tmux window of a Code session, by id (@3) or exact name."`
	Share   float64 `json:"share"   jsonschema:"The share of the nearest split that the side holding the tab gets, from 0.2 to 0.8."`
}

type layoutInput struct {
	Session string `json:"session" jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which uses the one shared space every such agent shares. Never guess or reuse another value."`
}

type layoutRequestResult struct {
	Requested string `json:"requested" jsonschema:"The request handed to the UI: the verb. Delivery is best-effort - it applies only while the user is viewing this chat or session, and there is no acknowledgment either way."`
	Tab       string `json:"tab"       jsonschema:"The tab the request acted on, spelled as given."`
}

type layoutResult struct {
	Known        bool          `json:"known"          jsonschema:"False when the app has not reported this layout yet; every other field is then empty."`
	OnScreen     bool          `json:"onScreen"       jsonschema:"Whether the user is looking at this chat or session now."`
	FocusedPanel string        `json:"focusedPanel"   jsonschema:"The id of the panel that has focus."`
	Panels       []layoutPanel `json:"panels"         jsonschema:"The panels, each with its tabs."`
	Splits       []layoutSplit `json:"splits"         jsonschema:"The splits that join the panels."`
	Note         string        `json:"note,omitempty" jsonschema:"Why the layout is not known, when known is false."`
}

type layoutPanel struct {
	ID   string      `json:"id"`
	Tabs []layoutTab `json:"tabs" jsonschema:"In tab order."`
}

type layoutTab struct {
	Tab   string `json:"tab"            jsonschema:"The tab spelled the way the tab tools take it: a window id (@80), or drive, diff, tasks, terminal."`
	Kind  string `json:"kind"           jsonschema:"drive, diff, tasks or terminal."`
	Name  string `json:"name,omitempty" jsonschema:"The tmux window name of a Code session's terminal."`
	Shown bool   `json:"shown"          jsonschema:"Whether this is the tab its panel shows."`
}

type layoutSplit struct {
	ID     string  `json:"id"`
	Axis   string  `json:"axis"   jsonschema:"row for side by side, column for stacked."`
	Ratio  float64 `json:"ratio"  jsonschema:"The share the first side gets."`
	First  string  `json:"first"  jsonschema:"The id of the panel or split on the first side."`
	Second string  `json:"second" jsonschema:"The id of the panel or split on the second side."`
}

type openTabResult struct {
	Requested string `json:"requested" jsonschema:"The request handed to the UI: open. Delivery is best-effort - it applies only while the user is viewing this chat or session, and there is no acknowledgment either way."`
	View      string `json:"view"      jsonschema:"The view asked for: drive, diff, tasks or terminal."`
	Path      string `json:"path"      jsonschema:"The document or canvas path the tab was asked to show, resolved from path; empty when none was given."`
	Window    string `json:"window"    jsonschema:"The tmux window id (@3) the terminal tab was asked to show, new or existing; empty for the other views."`
}

type closeTabResult struct {
	Requested string `json:"requested" jsonschema:"The request handed to the UI: close (a terminal is killed in tmux instead, with no UI request). Delivery is best-effort - it applies only while the user is viewing this chat or session, and there is no acknowledgment either way."`
	View      string `json:"view"      jsonschema:"The view asked to close: drive, diff, tasks or terminal."`
}

func (ctrl *DriveController) OpenTab(ctx context.Context, _ *mcp.CallToolRequest, in openTabInput) (*mcp.CallToolResult, openTabResult, error) {
	res, err := ctrl.core.Drive.OpenTab(ctx, app.OpenTabRequest{
		Session: in.Session, View: in.View, Path: in.Path, Placement: in.Placement, Anchor: in.Anchor,
		Window: in.Window, Name: in.Name, Command: in.Command,
	})
	if err != nil {
		return nil, openTabResult{}, ctrl.toolError(err)
	}
	return nil, openTabResult{Requested: "open", View: in.View, Path: res.Path, Window: res.Window}, nil
}

func (ctrl *DriveController) CloseTab(ctx context.Context, _ *mcp.CallToolRequest, in closeTabInput) (*mcp.CallToolResult, closeTabResult, error) {
	if err := ctrl.core.Drive.CloseTab(ctx, in.Session, in.View, in.Window); err != nil {
		return nil, closeTabResult{}, ctrl.toolError(err)
	}
	return nil, closeTabResult{Requested: "close", View: in.View}, nil
}

func (ctrl *DriveController) MoveTab(ctx context.Context, _ *mcp.CallToolRequest, in moveTabInput) (*mcp.CallToolResult, layoutRequestResult, error) {
	err := ctrl.core.Drive.MoveTab(ctx, app.MoveTabRequest{Session: in.Session, Tab: in.Tab, Placement: in.Placement, Anchor: in.Anchor})
	return layoutRequest(ctrl, "move", in.Tab, err)
}

func (ctrl *DriveController) FocusTab(ctx context.Context, _ *mcp.CallToolRequest, in tabInput) (*mcp.CallToolResult, layoutRequestResult, error) {
	return layoutRequest(ctrl, "focus", in.Tab, ctrl.core.Drive.FocusTab(ctx, in.Session, in.Tab))
}

func (ctrl *DriveController) ClosePanel(ctx context.Context, _ *mcp.CallToolRequest, in tabInput) (*mcp.CallToolResult, layoutRequestResult, error) {
	return layoutRequest(ctrl, "close_panel", in.Tab, ctrl.core.Drive.ClosePanel(ctx, in.Session, in.Tab))
}

func (ctrl *DriveController) ResizeSplit(ctx context.Context, _ *mcp.CallToolRequest, in resizeSplitInput) (*mcp.CallToolResult, layoutRequestResult, error) {
	return layoutRequest(ctrl, "resize", in.Tab, ctrl.core.Drive.ResizeSplit(ctx, in.Session, in.Tab, in.Share))
}

func (ctrl *DriveController) TurnSplit(ctx context.Context, _ *mcp.CallToolRequest, in tabInput) (*mcp.CallToolResult, layoutRequestResult, error) {
	return layoutRequest(ctrl, "turn", in.Tab, ctrl.core.Drive.TurnSplit(ctx, in.Session, in.Tab))
}

func layoutRequest(ctrl *DriveController, verb, tab string, err error) (*mcp.CallToolResult, layoutRequestResult, error) {
	if err != nil {
		return nil, layoutRequestResult{}, ctrl.toolError(err)
	}
	return nil, layoutRequestResult{Requested: verb, Tab: tab}, nil
}

func (ctrl *DriveController) GetLayout(ctx context.Context, _ *mcp.CallToolRequest, in layoutInput) (*mcp.CallToolResult, layoutResult, error) {
	view, err := ctrl.core.Drive.Layout(ctx, in.Session)
	if err != nil {
		return nil, layoutResult{}, ctrl.toolError(err)
	}
	out := layoutResult{
		Known: view.Known, OnScreen: view.OnScreen, FocusedPanel: view.FocusedPanel, Note: view.Note,
		Panels: make([]layoutPanel, 0, len(view.Panels)), Splits: make([]layoutSplit, 0, len(view.Splits)),
	}
	for _, p := range view.Panels {
		panel := layoutPanel{ID: p.ID, Tabs: make([]layoutTab, 0, len(p.Tabs))}
		for _, t := range p.Tabs {
			panel.Tabs = append(panel.Tabs, layoutTab{Tab: t.Tab, Kind: t.Kind, Name: t.Name, Shown: t.Shown})
		}
		out.Panels = append(out.Panels, panel)
	}
	for _, sp := range view.Splits {
		out.Splits = append(out.Splits, layoutSplit{ID: sp.ID, Axis: sp.Axis, Ratio: sp.Ratio, First: sp.First, Second: sp.Second})
	}
	return nil, out, nil
}
