package mcpsrv

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/internal/ops/layout"
	"github.com/colonyops/hive/internal/ops/mcpops"
)

func (ctrl *DriveController) registerTabs(srv *mcp.Server) {
	mcpops.Mount(srv, layout.TabOpen, ctrl.OpenTab, ctrl.toolError)
	mcpops.Mount(srv, layout.TabClose, ctrl.CloseTab, ctrl.toolError)
	mcpops.Mount(srv, layout.TabMove, ctrl.MoveTab, ctrl.toolError)
	mcpops.Mount(srv, layout.TabFocus, ctrl.FocusTab, ctrl.toolError)
	mcpops.Mount(srv, layout.PanelClose, ctrl.ClosePanel, ctrl.toolError)
	mcpops.Mount(srv, layout.SplitResize, ctrl.ResizeSplit, ctrl.toolError)
	mcpops.Mount(srv, layout.SplitTurn, ctrl.TurnSplit, ctrl.toolError)
	mcpops.Mount(srv, layout.LayoutGet, ctrl.GetLayout, ctrl.toolError)
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
	Tab   string `json:"tab"            jsonschema:"The tab spelled the way the tab tools take it: a window id (@80), applet:<id>, or drive, diff, tasks, terminal."`
	Kind  string `json:"kind"           jsonschema:"drive, diff, tasks, terminal or applet."`
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
	View      string `json:"view"      jsonschema:"The view asked for: drive, diff, tasks, terminal or applet:<id>."`
	Path      string `json:"path"      jsonschema:"The document or canvas path the tab was asked to show, resolved from path; empty when none was given."`
	Window    string `json:"window"    jsonschema:"The tmux window id (@3) the terminal tab was asked to show, new or existing; empty for the other views."`
}

type closeTabResult struct {
	Requested string `json:"requested" jsonschema:"The request handed to the UI: close (a terminal is killed in tmux instead, with no UI request). Delivery is best-effort - it applies only while the user is viewing this chat or session, and there is no acknowledgment either way."`
	View      string `json:"view"      jsonschema:"The view asked to close: drive, diff, tasks, terminal or applet:<id>."`
}

func (ctrl *DriveController) OpenTab(ctx context.Context, in layout.TabOpenIn) (openTabResult, error) {
	res, err := ctrl.core.Layouts.OpenTab(ctx, app.OpenTabRequest{
		Session: in.Caller, View: in.View, Path: in.Path, Placement: in.Placement, Anchor: in.Anchor,
		Window: in.Window, Name: in.Name,
	})
	if err != nil {
		return openTabResult{}, err
	}
	return openTabResult{Requested: "open", View: in.View, Path: res.Path, Window: res.Window}, nil
}

func (ctrl *DriveController) CloseTab(ctx context.Context, in layout.TabCloseIn) (closeTabResult, error) {
	if err := ctrl.core.Layouts.CloseTab(ctx, in.Caller, in.View, in.Window); err != nil {
		return closeTabResult{}, err
	}
	return closeTabResult{Requested: "close", View: in.View}, nil
}

func (ctrl *DriveController) MoveTab(ctx context.Context, in layout.TabMoveIn) (layoutRequestResult, error) {
	err := ctrl.core.Layouts.MoveTab(ctx, app.MoveTabRequest{Session: in.Caller, Tab: in.Tab, Placement: in.Placement, Anchor: in.Anchor})
	return layoutRequest("move", in.Tab, err)
}

func (ctrl *DriveController) FocusTab(ctx context.Context, in layout.TabIn) (layoutRequestResult, error) {
	return layoutRequest("focus", in.Tab, ctrl.core.Layouts.FocusTab(ctx, in.Caller, in.Tab))
}

func (ctrl *DriveController) ClosePanel(ctx context.Context, in layout.TabIn) (layoutRequestResult, error) {
	return layoutRequest("close_panel", in.Tab, ctrl.core.Layouts.ClosePanel(ctx, in.Caller, in.Tab))
}

func (ctrl *DriveController) ResizeSplit(ctx context.Context, in layout.SplitResizeIn) (layoutRequestResult, error) {
	return layoutRequest("resize", in.Tab, ctrl.core.Layouts.ResizeSplit(ctx, in.Caller, in.Tab, in.Share))
}

func (ctrl *DriveController) TurnSplit(ctx context.Context, in layout.TabIn) (layoutRequestResult, error) {
	return layoutRequest("turn", in.Tab, ctrl.core.Layouts.TurnSplit(ctx, in.Caller, in.Tab))
}

func layoutRequest(verb, tab string, err error) (layoutRequestResult, error) {
	if err != nil {
		return layoutRequestResult{}, err
	}
	return layoutRequestResult{Requested: verb, Tab: tab}, nil
}

func (ctrl *DriveController) GetLayout(ctx context.Context, in layout.LayoutGetIn) (layoutResult, error) {
	view, err := ctrl.core.Layouts.Layout(ctx, in.Caller)
	if err != nil {
		return layoutResult{}, err
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
	return out, nil
}
