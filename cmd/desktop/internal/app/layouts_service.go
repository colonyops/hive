package app

import "sync"

// The layout kinds the frontend reports: Code sessions and chats keep their
// panel layouts apart, and a key is only unique within its kind.
const (
	LayoutKindCode  = "code"
	LayoutKindChats = "chats"
)

// PanelLayoutReport is one session's panel layout as the frontend holds it.
type PanelLayoutReport struct {
	Root         PanelNodeReport
	FocusedPanel string
}

// PanelNodeReport is a panel (Tabs, ActiveTab, Picking) or a split (Axis,
// Ratio, and its two Children). Kind says which.
type PanelNodeReport struct {
	Kind      string
	ID        string
	Tabs      []PanelTabReport
	ActiveTab string
	Picking   bool
	Axis      string
	Ratio     float64
	Children  []PanelNodeReport
}

// PanelTabReport is one tab. Kind is window, docs, diff or tasks; WindowID is
// the tmux window id of a window tab.
type PanelTabReport struct {
	ID       string
	Kind     string
	WindowID string
}

// LayoutsService mirrors the panel layouts the browser holds, so an agent can
// read what is open before it acts. The browser owns the layout and reports
// it whole; nothing here is persisted, and a restart starts empty until the
// frontend reports again. The zero value is ready to use.
type LayoutsService struct {
	mu    sync.Mutex
	kinds map[string]layoutKind
}

type layoutKind struct {
	onScreen string
	layouts  map[string]PanelLayoutReport
}

// Report replaces everything known for kind: a layout missing from layouts
// was closed in the browser. onScreen is the key on screen in that kind's
// area, empty when none is.
func (s *LayoutsService) Report(kind, onScreen string, layouts map[string]PanelLayoutReport) error {
	if kind != LayoutKindCode && kind != LayoutKindChats {
		return Errorf(KindInvalid, "layout kind %q is not one of %s, %s", kind, LayoutKindCode, LayoutKindChats)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.kinds == nil {
		s.kinds = map[string]layoutKind{}
	}
	s.kinds[kind] = layoutKind{onScreen: onScreen, layouts: layouts}
	return nil
}

// Layout answers the reported layout for key, whether that key is the one on
// screen, and whether the frontend has reported it at all.
func (s *LayoutsService) Layout(kind, key string) (layout PanelLayoutReport, onScreen, known bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.kinds[kind]
	if !ok {
		return PanelLayoutReport{}, false, false
	}
	layout, known = state.layouts[key]
	return layout, known && state.onScreen == key, known
}
