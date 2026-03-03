package tasks

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/rs/zerolog/log"

	"github.com/colonyops/hive/internal/core/hc"
	"github.com/colonyops/hive/internal/core/styles"
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

	comments     map[string][]hc.Comment // cache: itemID -> comments
	lastItemID   string                  // track cursor changes
	statusFilter StatusFilter            // current status filter (default: FilterOpen)
}

// New creates a new tasks View.
func New(svc *hive.HoneycombService, repoKey string) *View {
	return &View{
		svc:          svc,
		repoKey:      repoKey,
		comments:     make(map[string][]hc.Comment),
		statusFilter: FilterOpen,
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
		return v.checkCursorChanged()

	case commentsLoadedMsg:
		if msg.err != nil {
			log.Debug().Err(msg.err).Str("item_id", msg.itemID).Msg("tasks: failed to load comments")
			return nil
		}
		v.comments[msg.itemID] = msg.comments
		return nil

	case RefreshTasksMsg:
		v.comments = make(map[string][]hc.Comment)
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

	filterBar := renderFilterBar(v.statusFilter)

	if len(v.flatNodes) == 0 {
		help := styles.TextMutedStyle.Render("f filter • r refresh")
		return filterBar + "\n  " + styles.TextMutedStyle.Render("No tasks match the current filter.") + "\n" + help
	}

	// Reserve 2 lines: filter bar + help bar.
	contentHeight := max(v.height-2, 1)

	// Pane widths: tree ~30%, detail ~70%.
	// Account for 1 divider column (1 char).
	availWidth := max(v.width-1, 30)
	treeWidth := max(availWidth*30/100, 25)
	detailWidth := max(availWidth-treeWidth, 10)

	// Tree pane
	treeContent := renderTree(v.flatNodes, v.cursor, v.scrollOffset, contentHeight)
	treeContent = ensureExactHeight(treeContent, contentHeight)
	treeContent = ensureExactWidth(treeContent, treeWidth)

	// Selected item for detail
	selected := v.SelectedItem()
	var selectedNode *TreeNode
	if v.cursor >= 0 && v.cursor < len(v.flatNodes) {
		selectedNode = v.flatNodes[v.cursor].Node
	}

	// Comments for selected item
	var itemComments []hc.Comment
	if selected != nil {
		itemComments = v.comments[selected.ID]
	}

	// Detail pane (includes header with properties)
	detailContent := renderDetail(selected, selectedNode, itemComments, detailWidth-2)
	detailContent = padLines(detailContent, 1)
	detailContent = ensureExactHeight(detailContent, contentHeight)
	detailContent = ensureExactWidth(detailContent, detailWidth)

	// Divider
	divider := buildDivider(contentHeight)

	// Compose panes
	body := lipgloss.JoinHorizontal(lipgloss.Top, treeContent, divider, detailContent)

	// Help bar
	help := styles.TextMutedStyle.Render("j/k navigate • enter expand/collapse • f filter • r refresh")

	return filterBar + "\n" + body + "\n" + help
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

// loadComments fetches comments for an item in a goroutine.
func (v *View) loadComments(itemID string) tea.Cmd {
	svc := v.svc
	return func() tea.Msg {
		comments, err := svc.ListComments(context.Background(), itemID)
		return commentsLoadedMsg{comments: comments, itemID: itemID, err: err}
	}
}

// checkCursorChanged checks if the selected item changed and loads comments if needed.
func (v *View) checkCursorChanged() tea.Cmd {
	selected := v.SelectedItem()
	if selected == nil {
		v.lastItemID = ""
		return nil
	}

	if selected.ID == v.lastItemID {
		return nil
	}

	v.lastItemID = selected.ID

	if _, cached := v.comments[selected.ID]; cached {
		return nil
	}

	return v.loadComments(selected.ID)
}

// rebuildTree rebuilds the tree from the current items, applying the active filter, and clamps the cursor.
func (v *View) rebuildTree() {
	filtered := filterItems(v.items, v.statusFilter)
	v.roots = buildTree(filtered)
	v.flatNodes = flattenTree(v.roots)
	v.clampCursor()
	v.clampScroll()
}

// handleKey processes keyboard input for navigation and actions.
func (v *View) handleKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "j", "down":
		v.moveCursor(1)
		return v.checkCursorChanged()
	case "k", "up":
		v.moveCursor(-1)
		return v.checkCursorChanged()
	case "enter":
		v.toggleExpand()
	case "r":
		v.comments = make(map[string][]hc.Comment)
		return v.loadItems()
	case "f":
		v.statusFilter = (v.statusFilter + 1) % filterCount
		v.rebuildTree()
		return v.checkCursorChanged()
	case "G":
		v.cursor = len(v.flatNodes) - 1
		v.clampScroll()
		return v.checkCursorChanged()
	case "g":
		v.cursor = 0
		v.scrollOffset = 0
		return v.checkCursorChanged()
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

// buildDivider creates a vertical divider string of the given height.
func buildDivider(height int) string {
	lines := make([]string, height)
	for i := range lines {
		lines[i] = styles.TextMutedStyle.Render("│")
	}
	return strings.Join(lines, "\n")
}

// padLines adds left padding to each line of content.
func padLines(content string, padding int) string {
	pad := strings.Repeat(" ", padding)
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = pad + line
	}
	return strings.Join(lines, "\n")
}

// ensureExactHeight ensures content has exactly n lines.
func ensureExactHeight(content string, n int) string {
	if n <= 0 {
		return ""
	}

	lines := strings.Split(content, "\n")

	if len(lines) > n {
		lines = lines[:n]
	} else {
		for len(lines) < n {
			lines = append(lines, "")
		}
	}

	return strings.Join(lines, "\n")
}

// ensureExactWidth pads or truncates each line to exactly the given width.
func ensureExactWidth(content string, width int) string {
	if width <= 0 {
		return content
	}

	lines := strings.Split(content, "\n")
	var b strings.Builder

	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}

		displayWidth := ansi.StringWidth(line)

		switch {
		case displayWidth == width:
			b.WriteString(line)
		case displayWidth < width:
			b.WriteString(line)
			b.WriteString(strings.Repeat(" ", width-displayWidth))
		default:
			b.WriteString(ansi.Truncate(line, width, ""))
		}
	}

	return b.String()
}
