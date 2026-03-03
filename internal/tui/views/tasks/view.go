package tasks

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/rs/zerolog/log"

	"github.com/colonyops/hive/internal/core/hc"
	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/hive"
)

// focusPane tracks which pane has keyboard focus.
type focusPane int

const (
	paneTree focusPane = iota
	paneDetail
)

// headerLines is the number of fixed lines above the viewport in the detail pane
// (properties bar + divider).
const headerLines = 2

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
	showPreview  bool                    // toggle detail/preview panel visibility

	viewport    viewport.Model // scrollable detail content
	detailWidth int            // cached detail pane width
	focus       focusPane      // which pane has keyboard focus
}

// New creates a new tasks View.
func New(svc *hive.HoneycombService, repoKey string) *View {
	return &View{
		svc:          svc,
		repoKey:      repoKey,
		comments:     make(map[string][]hc.Comment),
		statusFilter: FilterOpen,
		showPreview:  true,
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
		// Refresh viewport if we're still looking at this item
		if selected := v.SelectedItem(); selected != nil && selected.ID == msg.itemID {
			v.updateViewportContent()
		}
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
		help := styles.TextMutedStyle.Render("p preview • f filter • r refresh")
		return filterBar + "\n  " + styles.TextMutedStyle.Render("No tasks match the current filter.") + "\n" + help
	}

	// Reserve 2 lines: filter bar + help bar.
	contentHeight := max(v.height-2, 1)

	var body string

	if v.showPreview {
		// Two-column layout: tree ~30%, detail ~70%.
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

		// Detail pane: static header + scrollable viewport
		innerWidth := detailWidth - 2 // 1 char padding each side
		header := renderDetailHeader(selected, selectedNode, innerWidth)
		header = padLines(header, 1)
		headerRendered := ensureExactHeight(header, headerLines)
		headerRendered = ensureExactWidth(headerRendered, detailWidth)

		// Viewport content
		vpView := v.viewport.View()
		vpLines := strings.Split(vpView, "\n")
		for i, line := range vpLines {
			vpLines[i] = " " + line
		}
		vpContent := strings.Join(vpLines, "\n")

		// Compose header + viewport, then pad to exact height
		detailContent := headerRendered + "\n" + vpContent
		detailContent = ensureExactHeight(detailContent, contentHeight)
		detailContent = ensureExactWidth(detailContent, detailWidth)

		// Divider — accent color when detail pane has focus
		dividerStyle := styles.TextMutedStyle
		if v.focus == paneDetail {
			dividerStyle = styles.TextPrimaryStyle
		}
		divider := buildDividerStyled(contentHeight, dividerStyle)

		// Dim tree pane when detail has focus
		if v.focus == paneDetail {
			treeContent = styles.TextMutedStyle.Render(treeContent)
			treeContent = ensureExactWidth(treeContent, treeWidth)
		}

		// Compose panes
		body = lipgloss.JoinHorizontal(lipgloss.Top, treeContent, divider, detailContent)
	} else {
		// Tree-only layout: full width.
		treeContent := renderTree(v.flatNodes, v.cursor, v.scrollOffset, contentHeight)
		treeContent = ensureExactHeight(treeContent, contentHeight)
		treeContent = ensureExactWidth(treeContent, v.width)
		body = treeContent
	}

	// Help bar
	var help string
	if v.focus == paneDetail {
		scrollInfo := ""
		if v.viewport.TotalLineCount() > v.viewport.VisibleLineCount() {
			scrollInfo = fmt.Sprintf(" (%.0f%%)", v.viewport.ScrollPercent()*100)
		}
		help = styles.TextMutedStyle.Render(fmt.Sprintf("j/k scroll%s • esc back to tree", scrollInfo))
	} else {
		help = styles.TextMutedStyle.Render("j/k navigate • enter expand/collapse • tab detail • p preview • f filter • r refresh")
	}

	return filterBar + "\n" + body + "\n" + help
}

// SetSize updates the view dimensions.
func (v *View) SetSize(w, h int) {
	v.width = w
	v.height = h
	v.clampScroll()
	v.sizeViewport()
}

// sizeViewport recalculates the viewport dimensions based on the current view size.
func (v *View) sizeViewport() {
	if !v.showPreview {
		return
	}

	availWidth := max(v.width-1, 30)
	treeWidth := max(availWidth*30/100, 25)
	detailWidth := max(availWidth-treeWidth, 10)
	v.detailWidth = detailWidth

	// Content area: total height minus filter bar (1) and help bar (1)
	contentHeight := max(v.height-2, 1)
	// Viewport height: content height minus header lines (properties + divider) minus 1 for the newline separator
	vpHeight := max(contentHeight-headerLines-1, 1)
	innerWidth := max(detailWidth-2, 10) // 1 char padding each side

	v.viewport = viewport.New(
		viewport.WithWidth(innerWidth),
		viewport.WithHeight(vpHeight),
	)

	v.lastItemID = "" // force content re-render
}

// SetActive sets whether this view is the currently active tab.
func (v *View) SetActive(active bool) {
	v.active = active
	if !active {
		v.focus = paneTree
	}
}

// HasDetailFocus returns true when the detail pane has focus.
func (v *View) HasDetailFocus() bool {
	return v.focus == paneDetail
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
		v.viewport.SetContent("")
		return nil
	}

	if selected.ID == v.lastItemID {
		return nil
	}

	v.lastItemID = selected.ID
	v.updateViewportContent()

	if _, cached := v.comments[selected.ID]; cached {
		return nil
	}

	return v.loadComments(selected.ID)
}

// updateViewportContent re-renders the detail content into the viewport.
func (v *View) updateViewportContent() {
	selected := v.SelectedItem()
	if selected == nil {
		v.viewport.SetContent("")
		return
	}

	contentWidth := v.viewport.Width()
	if contentWidth <= 0 {
		contentWidth = 60
	}

	var itemComments []hc.Comment
	if comments, ok := v.comments[selected.ID]; ok {
		itemComments = comments
	}

	content := renderDetailContent(selected, itemComments, contentWidth)
	v.viewport.SetContent(content)
	v.viewport.SetYOffset(0)
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
	// Detail pane keys take priority when focused
	if v.focus == paneDetail {
		return v.handleDetailKey(msg)
	}

	return v.handleTreeKey(msg)
}

// handleTreeKey processes keys when the tree pane has focus.
func (v *View) handleTreeKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "j", "down":
		v.moveCursor(1)
		return v.checkCursorChanged()
	case "k", "up":
		v.moveCursor(-1)
		return v.checkCursorChanged()
	case "enter":
		v.toggleExpand()
	case "tab":
		if v.showPreview && v.SelectedItem() != nil {
			v.focus = paneDetail
			v.updateViewportContent()
		}
	case "r":
		v.comments = make(map[string][]hc.Comment)
		return v.loadItems()
	case "p":
		v.showPreview = !v.showPreview
		if v.showPreview {
			v.sizeViewport()
			v.updateViewportContent()
		}
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

// handleDetailKey processes keys when the detail pane has focus.
func (v *View) handleDetailKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc", "tab":
		v.focus = paneTree
	case "j", "down":
		v.viewport.ScrollDown(1)
	case "k", "up":
		v.viewport.ScrollUp(1)
	case "pgdown", "ctrl+d":
		v.viewport.ScrollDown(v.viewport.VisibleLineCount())
	case "pgup", "ctrl+u":
		v.viewport.ScrollUp(v.viewport.VisibleLineCount())
	case "g":
		v.viewport.SetYOffset(0)
	case "G":
		v.viewport.SetYOffset(v.viewport.TotalLineCount())
	default:
		v.viewport, _ = v.viewport.Update(msg)
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

// buildDividerStyled creates a vertical divider string of the given height with the given style.
func buildDividerStyled(height int, style lipgloss.Style) string {
	lines := make([]string, height)
	for i := range lines {
		lines[i] = style.Render("│")
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
