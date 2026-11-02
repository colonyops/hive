package wailsui

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

// LayoutService takes the panel layouts the frontend holds, so agents can read
// them through get_layout.
type LayoutService struct {
	layouts *app.LayoutsService
}

func NewLayoutService(layouts *app.LayoutsService) *LayoutService {
	return &LayoutService{layouts: layouts}
}

// LayoutReport is every layout of one kind. Kind is code or chats, OnScreen
// the key on screen in that kind's area (empty for none), and Layouts is keyed
// by hive session id for code and chat session id for chats.
type LayoutReport struct {
	Kind     string                     `json:"kind"`
	OnScreen string                     `json:"onScreen"`
	Layouts  map[string]PanelLayoutWire `json:"layouts"`
}

type PanelLayoutWire struct {
	Root         PanelNodeWire `json:"root"`
	FocusedPanel string        `json:"focusedPanel"`
}

// PanelNodeWire is a panel or a split, by Kind. A split's two Children are
// its first and second side.
type PanelNodeWire struct {
	Kind      string          `json:"kind"`
	ID        string          `json:"id"`
	Tabs      []PanelTabWire  `json:"tabs"`
	ActiveTab string          `json:"activeTab"`
	Picking   bool            `json:"picking"`
	Axis      string          `json:"axis"`
	Ratio     float64         `json:"ratio"`
	Children  []PanelNodeWire `json:"children"`
}

type PanelTabWire struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	WindowID string `json:"windowId"`
}

// Report replaces everything the mirror knows for the report's kind.
func (s *LayoutService) Report(_ context.Context, report LayoutReport) error {
	layouts := make(map[string]app.PanelLayoutReport, len(report.Layouts))
	for key, l := range report.Layouts {
		layouts[key] = app.PanelLayoutReport{Root: panelNodeOf(l.Root), FocusedPanel: l.FocusedPanel}
	}
	return s.layouts.Report(report.Kind, report.OnScreen, layouts)
}

func panelNodeOf(n PanelNodeWire) app.PanelNodeReport {
	out := app.PanelNodeReport{
		Kind: n.Kind, ID: n.ID, ActiveTab: n.ActiveTab, Picking: n.Picking, Axis: n.Axis, Ratio: n.Ratio,
	}
	for _, t := range n.Tabs {
		out.Tabs = append(out.Tabs, app.PanelTabReport{ID: t.ID, Kind: t.Kind, WindowID: t.WindowID})
	}
	for _, c := range n.Children {
		out.Children = append(out.Children, panelNodeOf(c))
	}
	return out
}
