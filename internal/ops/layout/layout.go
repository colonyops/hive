// Package layout declares the tab, panel and split operations of a chat or
// Code session, served on the drive MCP server.
package layout

import "github.com/colonyops/hive/internal/ops"

const (
	PlacementBehind = "behind"
	PlacementBeside = "beside"
	PlacementBelow  = "below"
)

var Placements = []string{PlacementBehind, PlacementBeside, PlacementBelow}

// The UI clamps a split to the same range, so a share outside it is refused
// rather than silently changed.
const (
	MinSplitShare = 0.2
	MaxSplitShare = 0.8
)

var Toolset = ops.Toolset{Name: "layout", MCPPath: "/mcp/drive"}

func All() []ops.Op {
	return []ops.Op{TabOpen, TabClose, TabMove, TabFocus, PanelClose, SplitResize, SplitTurn, LayoutGet}
}

var placement = ops.Constraints{"placement": {Enum: Placements, Default: PlacementBehind}}

type TabOpenIn struct {
	Caller    string `json:"caller"              jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which uses the one shared space every such agent shares. Never guess or reuse another value."`
	View      string `json:"view"                jsonschema:"drive, diff, tasks, terminal or applet:<id>. Drive is the session's documents and canvases; Diff is the Code session's changes; Tasks is its task board; Terminal is a tmux window of the Code session; applet:<id> is an applet that lists surfaces.tab."`
	Path      string `json:"path,omitempty"      jsonschema:"Drive only: the document to show, by its path in the space, or a canvas by name or path (perf is canvases/perf.canvas.json). It must exist. Omit to leave the tab on its own pick."`
	Placement string `json:"placement,omitempty" jsonschema:"behind (default) adds the tab behind the tab in front, with an unseen dot, and never takes the screen. beside splits the panel in front and opens the tab next to it; below splits it downward. A tab that is already open stays where it is."`
	Anchor    string `json:"anchor,omitempty"    jsonschema:"The tab to place relative to: drive, diff, tasks, terminal (a chat's one terminal), applet:<id>, or a window of the Code session by id (@3) or exact name. Omit for the tab in front of the focused panel."`
	Window    string `json:"window,omitempty"    jsonschema:"Terminal only: an existing window of the session, by tmux id (@3) or exact window name. It is placed instead of starting one, so name must be empty."`
	Name      string `json:"name,omitempty"      jsonschema:"Terminal only: the name of the new window. The window is a shell in the session's checkout with nothing running in it."`
}

var TabOpen = ops.Spec[TabOpenIn]{
	Name: "tab.open", Owner: ops.OwnerDesktop, Effect: ops.EffectWrite, Toolset: Toolset, Caller: ops.CallerRuleSession,
	Summary: "Open a tab or terminal beside the session",
	Guidance: "Ask Hive to open a tab beside this chat or Code session. view is drive (the session's documents and " +
		"canvases), diff (the Code session's changes), tasks (its task board), terminal (a tmux window of the Code " +
		"session) or applet:<id> (an applet whose applet.yaml lists surfaces.tab; opening it runs its page). The tab applies only while the user is viewing this chat or session, and there is no " +
		"acknowledgment; a terminal's tmux window is started whether or not they are. A chat has drive and applets; diff, " +
		"tasks and terminal belong to a Code session. The shared space (global) sits beside no session, so it has no " +
		"tabs; the user reads it in the Drive page. path, for drive only, shows one document by its path in the space, " +
		"or a canvas by name or path (perf is canvases/perf.canvas.json); it must exist. Omit it to leave the tab on " +
		"its own pick. terminal starts a new window in the session's checkout, a shell with nothing running in it, or " +
		"places one that exists. It opens detached, so it never takes the keyboard or the screen. It takes no command: " +
		"run your own work yourself, and open a terminal for the user to type in. window, for terminal only, places " +
		"an existing window of the session by tmux id (@3) or exact name instead of starting one, and excludes name. " +
		"name, for terminal only, names the new window. The session must be running; start it in Code first. " +
		"placement behind (the default) adds the tab behind the tab " +
		"in front, with an unseen dot, and never takes the screen; beside splits the panel in front and opens the tab " +
		"next to it, and below splits it downward, so the user sees it at once. All leave the keyboard where it is, " +
		"and a tab that is already open stays where it is under any. anchor, a tab reference (drive, diff, tasks, " +
		"terminal, or a window by id or name), is the tab to place relative to instead of the tab in front of the " +
		"focused panel. Call it when you finish something worth looking at now, not on every write: " +
		"a write while a Drive tab is open behind another tab already lights a dot on it. Use beside or below only " +
		"for a result the user asked to see side by side with the terminal. If you are not sure what is open, call " +
		"layout_get first.",
	Args:   []string{"view"},
	Fields: placement,
}

type TabCloseIn struct {
	Caller string `json:"caller"           jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which uses the one shared space every such agent shares. Never guess or reuse another value."`
	View   string `json:"view"             jsonschema:"drive, diff, tasks, terminal or applet:<id>. The tab for drive, diff, tasks or an applet closes wherever it sits. terminal kills the tmux window named by window."`
	Window string `json:"window,omitempty" jsonschema:"Terminal only, and required for it: the window of the Code session to kill, by tmux id (@3) or exact name. It ends what runs in it. A chat's terminal is refused."`
}

var TabClose = ops.Spec[TabCloseIn]{
	Name: "tab.close", Owner: ops.OwnerDesktop, Effect: ops.EffectWrite, Toolset: Toolset, Caller: ops.CallerRuleSession,
	Summary: "Close a tab",
	Guidance: "Ask Hive to close this chat or Code session's tab for a view (drive, diff, tasks or applet:<id>), wherever it " +
		"sits. Closing an applet's tab ends its page. Like tab_open, the tab closes only while the user is viewing this chat or session. Documents and " +
		"canvases are untouched; this closes the tab, not the content. view terminal needs window (a tmux window of " +
		"the Code session, by id or exact name) and kills that window through tmux whether or not the session is on " +
		"screen: it ends what runs in it, and the tab goes with it. Only close a window you started. A chat's " +
		"terminal is the chat, so tab_close refuses it.",
	Args: []string{"view"},
}

type TabMoveIn struct {
	Caller    string `json:"caller"              jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which uses the one shared space every such agent shares. Never guess or reuse another value."`
	Tab       string `json:"tab"                 jsonschema:"The tab to move. drive, diff or tasks name that view. terminal names a chat's one terminal. applet:<id> names that applet's tab. Anything else is a tmux window of a Code session, by id (@3) or exact name."`
	Placement string `json:"placement,omitempty" jsonschema:"behind (default) puts the tab in the anchor's panel right after the anchor, or at the end of the focused panel. beside and below split the anchor's panel (or the focused one) and move the tab into the new panel."`
	Anchor    string `json:"anchor,omitempty"    jsonschema:"The tab to place relative to, spelled like tab. Omit for the tab in front of the focused panel."`
}

var TabMove = ops.Spec[TabMoveIn]{
	Name: "tab.move", Owner: ops.OwnerDesktop, Effect: ops.EffectWrite, Toolset: Toolset, Caller: ops.CallerRuleSession,
	Summary: "Move a tab",
	Guidance: "Ask Hive to move a tab within this chat or Code session's panels. tab is a tab reference. placement " +
		"behind (the default) puts it in the anchor's panel right after the anchor, or at the end of the focused panel " +
		"with no anchor. beside and below split the anchor's panel (or the focused one) and move the tab into the new " +
		"panel. It applies only while the user is viewing this chat or session, and takes neither the keyboard nor " +
		"the focus.",
	Args:   []string{"tab"},
	Fields: placement,
}

// TabIn names one tab. tab.focus, panel.close and split.turn take it.
type TabIn struct {
	Caller string `json:"caller" jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which uses the one shared space every such agent shares. Never guess or reuse another value."`
	Tab    string `json:"tab"    jsonschema:"A tab in the panel to act on. drive, diff or tasks name that view. terminal names a chat's one terminal. applet:<id> names that applet's tab. Anything else is a tmux window of a Code session, by id (@3) or exact name."`
}

var TabFocus = ops.Spec[TabIn]{
	Name: "tab.focus", Owner: ops.OwnerDesktop, Effect: ops.EffectWrite, Toolset: Toolset, Caller: ops.CallerRuleSession,
	Summary: "Bring a tab to the front",
	Guidance: "Ask Hive to bring a tab forward and focus its panel; a terminal window is also selected in tmux. " +
		"This one takes the screen: use it when the user asked to see something, not to show off a result. For a result " +
		"use tab_open, which leaves the focus where it is.",
	Args: []string{"tab"},
}

var PanelClose = ops.Spec[TabIn]{
	Name: "panel.close", Owner: ops.OwnerDesktop, Effect: ops.EffectWrite, Toolset: Toolset, Caller: ops.CallerRuleSession,
	Summary: "Close a panel",
	Guidance: "Ask Hive to close the panel that holds a tab. Its tabs join the neighbouring panel and none of them " +
		"close. The last panel stays.",
	Args: []string{"tab"},
}

type SplitResizeIn struct {
	Caller string  `json:"caller" jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which uses the one shared space every such agent shares. Never guess or reuse another value."`
	Tab    string  `json:"tab"    jsonschema:"A tab in the panel whose side of the split to size. drive, diff or tasks name that view. terminal names a chat's one terminal. applet:<id> names that applet's tab. Anything else is a tmux window of a Code session, by id (@3) or exact name."`
	Share  float64 `json:"share"  jsonschema:"The share of the nearest split that the side holding the tab gets, from 0.2 to 0.8."`
}

var SplitResize = ops.Spec[SplitResizeIn]{
	Name: "split.resize", Owner: ops.OwnerDesktop, Effect: ops.EffectWrite, Toolset: Toolset, Caller: ops.CallerRuleSession,
	Summary: "Resize a split",
	Guidance: "Ask Hive to resize the nearest split of the panel that holds a tab. share, from 0.2 to 0.8, is the " +
		"share of the split that the side holding the tab gets.",
	Args:   []string{"tab"},
	Fields: ops.Constraints{"share": ops.Range(MinSplitShare, MaxSplitShare)},
}

var SplitTurn = ops.Spec[TabIn]{
	Name: "split.turn", Owner: ops.OwnerDesktop, Effect: ops.EffectWrite, Toolset: Toolset, Caller: ops.CallerRuleSession,
	Summary:  "Turn a split",
	Guidance: "Ask Hive to turn the nearest split of the panel that holds a tab between side by side and stacked.",
	Args:     []string{"tab"},
}

type LayoutGetIn struct {
	Caller string `json:"caller" jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Outside both, it is the word global, which uses the one shared space every such agent shares. Never guess or reuse another value."`
}

var LayoutGet = ops.Spec[LayoutGetIn]{
	Name: "layout.get", Owner: ops.OwnerDesktop, Effect: ops.EffectRead, Toolset: Toolset, Caller: ops.CallerRuleSession,
	Summary: "Read the panel layout",
	Guidance: "Read the panel layout of this chat or Code session as the app last reported it: each panel with its " +
		"tabs, which tab is shown, which panel has focus, and the splits that join the panels. Each tab is spelled the " +
		"way the tab tools take it, so you can pass it back. A Code session's terminal tabs carry the tmux window name. " +
		"known is false when the app has not reported this layout yet, for instance just after it started or when the " +
		"session has never been open in Code; then act without it. onScreen says whether the user is looking at this " +
		"session now. Call it first when you are not sure what is open.",
}
