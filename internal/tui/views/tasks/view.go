package tasks

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/colonyops/hive/internal/core/hc"
	"github.com/colonyops/hive/internal/hive"
)

// View is the Bubble Tea sub-model for the tasks tab.
type View struct {
	svc    *hive.HoneycombService
	width  int
	height int
	active bool

	items        []hc.Item
	roots        []*TreeNode
	flatNodes    []FlatNode
	cursor       int
	scrollOffset int
	repoKey      string
}

// New creates a new tasks View.
func New(svc *hive.HoneycombService, repoKey string) *View {
	return &View{
		svc:     svc,
		repoKey: repoKey,
	}
}

// Init initializes the tasks view.
func (v *View) Init() tea.Cmd {
	if v.svc != nil {
		return v.loadItems()
	}
	return nil
}

// Update handles messages for the tasks view.
func (v *View) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case itemsLoadedMsg:
		if msg.err != nil {
			return nil
		}
		v.items = msg.items
		v.rebuildTree()
		return nil

	case RefreshTasksMsg:
		return v.loadItems()

	case tea.KeyMsg:
		if !v.active {
			return nil
		}
		return v.handleKey(msg)
	}

	return nil
}

// View renders the tasks view.
func (v *View) View() string {
	if v.svc == nil {
		return "Tasks not configured"
	}
	if len(v.flatNodes) == 0 && len(v.items) == 0 {
		return "  No tasks loaded"
	}
	return renderTree(v.flatNodes, v.cursor, v.scrollOffset, v.height)
}

// SetSize updates the view dimensions.
func (v *View) SetSize(w, h int) {
	v.width = w
	v.height = h
	v.clampScroll()
}

// SetActive sets whether this view is the currently active tab.
func (v *View) SetActive(active bool) {
	v.active = active
}

// SelectedItem returns the currently selected item, or nil if none.
func (v *View) SelectedItem() *hc.Item {
	if v.cursor < 0 || v.cursor >= len(v.flatNodes) {
		return nil
	}
	return &v.flatNodes[v.cursor].Node.Item
}

// loadItems fetches items from the store in a goroutine.
func (v *View) loadItems() tea.Cmd {
	svc := v.svc
	repoKey := v.repoKey
	return func() tea.Msg {
		items, err := svc.ListItems(context.Background(), hc.ListFilter{
			RepoKey: repoKey,
		})
		return itemsLoadedMsg{items: items, err: err}
	}
}

// rebuildTree rebuilds the tree from the current items and clamps the cursor.
func (v *View) rebuildTree() {
	v.roots = buildTree(v.items)
	v.flatNodes = flattenTree(v.roots)
	v.clampCursor()
	v.clampScroll()
}

// handleKey processes keyboard input for navigation and actions.
func (v *View) handleKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "j", "down":
		v.moveCursor(1)
	case "k", "up":
		v.moveCursor(-1)
	case "enter":
		v.toggleExpand()
	case "r":
		return v.loadItems()
	case "G":
		v.cursor = len(v.flatNodes) - 1
		v.clampScroll()
	case "g":
		v.cursor = 0
		v.scrollOffset = 0
	}
	return nil
}

// moveCursor moves the cursor by delta and adjusts scroll.
func (v *View) moveCursor(delta int) {
	v.cursor += delta
	v.clampCursor()
	v.clampScroll()
}

// toggleExpand toggles expand/collapse on the selected node if it has children.
func (v *View) toggleExpand() {
	if v.cursor < 0 || v.cursor >= len(v.flatNodes) {
		return
	}
	node := v.flatNodes[v.cursor].Node
	if len(node.Children) > 0 {
		node.Expanded = !node.Expanded
		v.flatNodes = flattenTree(v.roots)
		v.clampCursor()
		v.clampScroll()
	}
}

// clampCursor ensures the cursor is within bounds.
func (v *View) clampCursor() {
	if len(v.flatNodes) == 0 {
		v.cursor = 0
		return
	}
	if v.cursor < 0 {
		v.cursor = 0
	}
	if v.cursor >= len(v.flatNodes) {
		v.cursor = len(v.flatNodes) - 1
	}
}

// clampScroll ensures the scroll offset keeps the cursor visible.
func (v *View) clampScroll() {
	if v.height <= 0 {
		return
	}
	// Cursor below visible area
	if v.cursor >= v.scrollOffset+v.height {
		v.scrollOffset = v.cursor - v.height + 1
	}
	// Cursor above visible area
	if v.cursor < v.scrollOffset {
		v.scrollOffset = v.cursor
	}
	// Don't scroll past the end
	maxOffset := max(len(v.flatNodes)-v.height, 0)
	v.scrollOffset = max(min(v.scrollOffset, maxOffset), 0)
}
