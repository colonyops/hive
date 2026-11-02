package app

import (
	"context"
)

// LayoutView is a session's panel layout as the frontend last reported it.
// Known is false, with Note saying why, when nothing was reported for the
// session.
type LayoutView struct {
	Known        bool
	OnScreen     bool
	FocusedPanel string
	Panels       []LayoutPanel
	Splits       []LayoutSplit
	Note         string
}

type LayoutPanel struct {
	ID   string
	Tabs []LayoutTab
}

// LayoutTab names its tab the way the tab tools take it: a window id, or the
// view name. Kind is drive, diff, tasks or terminal.
type LayoutTab struct {
	Tab   string
	Kind  string
	Name  string
	Shown bool
}

// LayoutSplit joins two nodes, each a panel or another split, by id. Axis is
// row for side by side and column for stacked.
type LayoutSplit struct {
	ID     string
	Axis   string
	Ratio  float64
	First  string
	Second string
}

const layoutUnknownNote = "the frontend has not reported this layout: the app may have just started, or the session has never been open in Code"

// Layout reads the panel layout of the calling chat or Code session from the
// mirror. It never reaches the browser, so it answers the last report.
func (s *LayoutsService) Layout(ctx context.Context, session string) (LayoutView, error) {
	caller, err := s.callers.tabCaller(ctx, session)
	if err != nil {
		return LayoutView{}, err
	}
	if err := checkTabSpace(caller); err != nil {
		return LayoutView{}, err
	}
	kind, key := LayoutKindCode, caller.author.HiveSession
	chat := caller.author.Session != ""
	if chat {
		kind, key = LayoutKindChats, caller.author.Session
	}
	report, onScreen, known := s.Reported(kind, key)
	if !known {
		return LayoutView{Panels: []LayoutPanel{}, Splits: []LayoutSplit{}, Note: layoutUnknownNote}, nil
	}
	view := LayoutView{Known: true, OnScreen: onScreen, FocusedPanel: report.FocusedPanel, Panels: []LayoutPanel{}, Splits: []LayoutSplit{}}
	names := s.windowNames(ctx, caller)
	var walk func(PanelNodeReport)
	walk = func(n PanelNodeReport) {
		if n.Kind != "split" {
			view.Panels = append(view.Panels, layoutPanelOf(n, chat, names))
			return
		}
		split := LayoutSplit{ID: n.ID, Axis: n.Axis, Ratio: n.Ratio}
		if len(n.Children) == 2 {
			split.First, split.Second = n.Children[0].ID, n.Children[1].ID
		}
		view.Splits = append(view.Splits, split)
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(report.Root)
	return view, nil
}

func layoutPanelOf(n PanelNodeReport, chat bool, names map[string]string) LayoutPanel {
	panel := LayoutPanel{ID: n.ID, Tabs: make([]LayoutTab, 0, len(n.Tabs))}
	for _, t := range n.Tabs {
		tab := LayoutTab{Shown: t.ID == n.ActiveTab}
		switch t.Kind {
		case "window":
			tab.Kind = string(TabViewTerminal)
			tab.Tab = t.WindowID
			if tab.Tab == "" {
				tab.Tab = t.ID
			}
			if chat {
				tab.Tab = string(TabViewTerminal)
			}
			tab.Name = names[t.WindowID]
		case "docs":
			tab.Kind, tab.Tab = string(TabViewDrive), string(TabViewDrive)
		default:
			tab.Kind, tab.Tab = t.Kind, t.Kind
		}
		panel.Tabs = append(panel.Tabs, tab)
	}
	return panel
}

// windowNames maps a hive session's tmux window ids to their names. A chat's
// tmux session is not known to the drive caller, and a listing that fails
// costs only the names, so both answer an empty map.
func (s *LayoutsService) windowNames(ctx context.Context, caller driveCaller) map[string]string {
	if s.terminals == nil || caller.tmuxSession == "" {
		return nil
	}
	names, err := s.terminals.WindowNames(ctx, caller.tmuxSession)
	if err != nil {
		s.logger.Debug().Err(err).Str("session", caller.tmuxSession).Msg("listing window names for the layout failed")
		return nil
	}
	return names
}
