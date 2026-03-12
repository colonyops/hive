package tasks

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/rs/zerolog/log"

	"github.com/colonyops/hive/internal/core/hc"
	corekv "github.com/colonyops/hive/internal/core/kv"
	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/tui/components"
	"github.com/colonyops/hive/internal/tui/views/shared"
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
	handler     KeyResolver    // resolves configurable keybindings to actions
	kvStore     corekv.KV      // persistent kv store for saving preferences
	splitRatio  int            // panel split percentage (1-80, 0 = default)

	// Rendered content cache — avoids re-running glamour on every cursor change.
	cachedContentKey string // "itemID:commentCount:width"
	cachedContent    string
}

// New creates a new tasks View.
func New(svc *hive.HoneycombService, repoKey string, handler KeyResolver, kvStore corekv.KV, splitRatio int) *View {
	return &View{
		svc:          svc,
		repoKey:      repoKey,
		handler:      handler,
		kvStore:      kvStore,
		splitRatio:   splitRatio,
		comments:     make(map[string][]hc.Comment),
		statusFilter: FilterOpen,
		showPreview:  true,
	}
}

// kvFilterKey is the kv store key for persisting the tasks status filter.
const kvFilterKey = "tui.tasks.filter"

// Init initializes the tasks view.
func (v *View) Init() tea.Cmd {
	v.restoreFilter()
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
			return v.updateViewportContent()
		}
		return nil

	case contentRenderedMsg:
		v.cachedContentKey = msg.key
		v.cachedContent = msg.content
		// Only apply if we're still viewing the same item.
		if selected := v.SelectedItem(); selected != nil {
			currentKey := fmt.Sprintf("%s:%d:%d", selected.ID, len(v.comments[selected.ID]), v.viewport.Width())
			if currentKey == msg.key {
				v.viewport.SetContent(msg.content)
			}
		}
		return nil

	case RefreshTasksMsg:
		v.comments = make(map[string][]hc.Comment)
		v.cachedContentKey = ""
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
	// Content area includes the repo header and filter bar.
	contentHeight := max(v.height-bottomBarLines, 1)
	treeRows := max(contentHeight-3, 1) // tree items between repo header and filter divider+bar

	var body string

	// Choose empty-state message: distinguish "no items at all" from "filtered out".
	var emptyMsg string
	if len(v.items) == 0 {
		emptyMsg = "No tasks for repository"
	}

	if v.showPreview {
		// Two-column layout: configurable tree/detail split.
		// Account for 1 divider column (1 char).
		splitPct := v.splitRatioOrDefault(30)
		availWidth := max(v.width-1, 30)
		treeWidth := max(availWidth*splitPct/100, 25)
		detailWidth := max(availWidth-treeWidth, 10)

		// Tree pane: repo header + items + filter bar
		repoHeader := " " + styles.TextMutedStyle.Render(v.repoKey)
		repoHeader = shared.EnsureExactWidth(repoHeader, treeWidth)
		treeContent := renderTree(v.flatNodes, v.cursor, v.scrollOffset, treeRows, emptyMsg)
		treeContent = shared.EnsureExactHeight(treeContent, treeRows)
		filterLine := " " + renderFilterBar(v.statusFilter)
		filterLine = shared.EnsureExactWidth(filterLine, treeWidth)
		treeContent = shared.EnsureExactWidth(treeContent, treeWidth)
		filterRule := styles.TextMutedStyle.Render(strings.Repeat("─", treeWidth))
		treeContent = repoHeader + "\n" + treeContent + "\n" + filterRule + "\n" + filterLine

		// Selected item for detail
		selected := v.SelectedItem()
		var selectedNode *TreeNode
		if v.cursor >= 0 && v.cursor < len(v.flatNodes) {
			selectedNode = v.flatNodes[v.cursor].Node
		}

		// Detail pane: static header + scrollable viewport
		innerWidth := detailWidth - 2 // 1 char padding each side
		header := renderDetailHeader(selected, selectedNode, innerWidth)
		header = shared.PadLines(header, 1)
		headerRendered := shared.EnsureExactHeight(header, headerLines)
		headerRendered = shared.EnsureExactWidth(headerRendered, detailWidth)

		// Viewport content
		vpView := v.viewport.View()
		vpLines := strings.Split(vpView, "\n")
		for i, line := range vpLines {
			vpLines[i] = " " + line
		}
		vpContent := strings.Join(vpLines, "\n")

		// Compose header + viewport, then pad to exact height
		detailContent := headerRendered + "\n" + vpContent
		detailContent = shared.EnsureExactHeight(detailContent, contentHeight)
		detailContent = shared.EnsureExactWidth(detailContent, detailWidth)

		// Divider — accent color when detail pane has focus
		dividerStyle := styles.TextMutedStyle
		if v.focus == paneDetail {
			dividerStyle = styles.TextPrimaryStyle
		}
		divider := shared.BuildDividerStyled(contentHeight, dividerStyle)

		// Dim tree pane when detail has focus.
		if v.focus == paneDetail {
			treeContent = styles.TextMutedStyle.Render(treeContent)
		}

		// Compose panes
		body = lipgloss.JoinHorizontal(lipgloss.Top, treeContent, divider, detailContent)
	} else {
		// Tree-only layout: full width.
		repoHeader := " " + styles.TextMutedStyle.Render(v.repoKey)
		treeContent := renderTree(v.flatNodes, v.cursor, v.scrollOffset, treeRows, emptyMsg)
		treeContent = shared.EnsureExactHeight(treeContent, treeRows)
		treeContent = shared.EnsureExactWidth(treeContent, v.width)
		filterLine := " " + renderFilterBar(v.statusFilter)
		filterRule := styles.TextMutedStyle.Render(strings.Repeat("─", v.width))
		body = repoHeader + "\n" + treeContent + "\n" + filterRule + "\n" + filterLine
	}

	// Bottom: rule + help bar
	bar := components.StatusBar{Width: v.width}
	var help string
	if v.focus == paneDetail {
		scrollInfo := ""
		if v.viewport.TotalLineCount() > v.viewport.VisibleLineCount() {
			scrollInfo = fmt.Sprintf(" (%.0f%%)", v.viewport.ScrollPercent()*100)
		}
		help = styles.TextMutedStyle.Render(fmt.Sprintf("j/k scroll%s"+components.HelpSep+"h/esc back to tree", scrollInfo))
	} else {
		help = styles.TextMutedStyle.Render(components.HelpNav + components.HelpSep + "space expand" + components.HelpSep + "enter detail" + components.HelpSep + components.HelpHelp)
	}

	return body + "\n" + bar.Rule() + "\n" + bar.Render(help, "")
}

// splitRatioOrDefault returns the configured split ratio, or the given default if unset or invalid.
func (v *View) splitRatioOrDefault(defaultPct int) int {
	if v.splitRatio < 1 || v.splitRatio > 80 {
		return defaultPct
	}
	return v.splitRatio
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

	splitPct := v.splitRatioOrDefault(30)
	availWidth := max(v.width-1, 30)
	treeWidth := max(availWidth*splitPct/100, 25)
	detailWidth := max(availWidth-treeWidth, 10)
	v.detailWidth = detailWidth

	// Content area: total height minus bottom bars.
	contentHeight := max(v.height-bottomBarLines, 1)
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

// SetRepoKey changes the repository scope, clears state, and reloads items.
func (v *View) SetRepoKey(repoKey string) tea.Cmd {
	v.repoKey = repoKey
	v.items = nil
	v.roots = nil
	v.flatNodes = nil
	v.cursor = 0
	v.scrollOffset = 0
	v.comments = make(map[string][]hc.Comment)
	v.lastItemID = ""
	v.cachedContentKey = ""
	v.cachedContent = ""
	if v.svc != nil {
		return v.loadItems()
	}
	return nil
}

// RepoKey returns the current repository scope.
func (v *View) RepoKey() string {
	return v.repoKey
}

// Svc returns the honeycomb service, or nil if not configured.
func (v *View) Svc() *hive.HoneycombService {
	return v.svc
}

// CycleFilter advances the status filter, persists it, and rebuilds the tree.
func (v *View) CycleFilter() tea.Cmd {
	v.statusFilter = (v.statusFilter + 1) % filterCount
	v.saveFilter()
	v.rebuildTree()
	return v.checkCursorChanged()
}

// restoreFilter loads the persisted status filter from the kv store.
func (v *View) restoreFilter() {
	if v.kvStore == nil {
		return
	}
	var saved int
	if err := v.kvStore.Get(context.Background(), kvFilterKey, &saved); err != nil {
		return
	}
	if saved >= 0 && saved < filterCount {
		v.statusFilter = StatusFilter(saved)
	}
}

// saveFilter persists the current status filter to the kv store.
func (v *View) saveFilter() {
	if v.kvStore == nil {
		return
	}
	if err := v.kvStore.Set(context.Background(), kvFilterKey, int(v.statusFilter)); err != nil {
		log.Debug().Err(err).Msg("failed to persist tasks filter")
	}
}

// TogglePreview toggles the preview panel visibility.
func (v *View) TogglePreview() tea.Cmd {
	v.showPreview = !v.showPreview
	if v.showPreview {
		v.sizeViewport()
		return v.updateViewportContent()
	}
	return nil
}

// HasDetailFocus returns true when the detail pane has focus.
func (v *View) HasDetailFocus() bool {
	return v.focus == paneDetail
}

// HelpSections returns view-specific help sections for the help dialog.
func (v *View) HelpSections() []components.HelpDialogSection {
	sections := []components.HelpDialogSection{
		{
			Title: "Navigation",
			Entries: []components.HelpEntry{
				{Key: "↑/k", Desc: "move up"},
				{Key: "↓/j", Desc: "move down"},
				{Key: "g/G", Desc: "go to top/bottom"},
				{Key: "space", Desc: "expand/collapse"},
				{Key: "enter/l", Desc: "detail pane"},
				{Key: "h/esc", Desc: "back to tree"},
				{Key: ":", Desc: "command palette"},
			},
		},
		{
			Title: "Status",
			Entries: []components.HelpEntry{
				{Key: "o", Desc: "mark open"},
				{Key: "i", Desc: "mark in progress"},
				{Key: "d", Desc: "mark done"},
				{Key: "x", Desc: "mark cancelled"},
			},
		},
	}

	if v.handler != nil {
		actionEntries := parseHelpEntries(v.handler.HelpEntries())
		if len(actionEntries) > 0 {
			sections = append(sections, components.HelpDialogSection{
				Title:   "Actions",
				Entries: actionEntries,
			})
		}
	}

	return sections
}

// parseHelpEntries converts "key help" formatted strings to HelpEntry slices.
func parseHelpEntries(raw []string) []components.HelpEntry {
	entries := make([]components.HelpEntry, 0, len(raw))
	for _, e := range raw {
		parts := strings.SplitN(e, " ", 2)
		if len(parts) == 2 {
			entries = append(entries, components.HelpEntry{Key: parts[0], Desc: parts[1]})
		}
	}
	return entries
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
	renderCmd := v.updateViewportContent()

	var cmds []tea.Cmd
	if renderCmd != nil {
		cmds = append(cmds, renderCmd)
	}
	if _, cached := v.comments[selected.ID]; !cached {
		cmds = append(cmds, v.loadComments(selected.ID))
	}

	return tea.Batch(cmds...)
}

// updateViewportContent sets viewport content from cache or kicks off async rendering.
// Returns a tea.Cmd when async rendering is needed, nil otherwise.
func (v *View) updateViewportContent() tea.Cmd {
	selected := v.SelectedItem()
	if selected == nil {
		v.viewport.SetContent("")
		v.cachedContentKey = ""
		return nil
	}

	contentWidth := v.viewport.Width()
	if contentWidth <= 0 {
		contentWidth = 60
	}

	var itemComments []hc.Comment
	if comments, ok := v.comments[selected.ID]; ok {
		itemComments = comments
	}

	key := fmt.Sprintf("%s:%d:%d", selected.ID, len(itemComments), contentWidth)
	if key == v.cachedContentKey {
		v.viewport.SetContent(v.cachedContent)
		v.viewport.SetYOffset(0)
		return nil
	}

	// Show title immediately while markdown renders in the background.
	v.viewport.SetContent(styles.TextForegroundBoldStyle.Render(selected.Title) + "\n")
	v.viewport.SetYOffset(0)

	// Capture values for the goroutine.
	item := *selected
	comments := itemComments
	width := contentWidth

	return func() tea.Msg {
		content := renderDetailContent(&item, comments, width)
		return contentRenderedMsg{key: key, content: content}
	}
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
	keyStr := msg.String()

	switch keyStr {
	// Navigation keys — stay hard-coded
	case "j", "down":
		v.moveCursor(1)
		return v.checkCursorChanged()
	case "k", "up":
		v.moveCursor(-1)
		return v.checkCursorChanged()
	case " ", "space":
		v.toggleExpand()
		return nil
	case "enter", "l":
		if v.showPreview && v.SelectedItem() != nil {
			v.focus = paneDetail
			return v.updateViewportContent()
		}
		return nil
	case "G":
		v.cursor = len(v.flatNodes) - 1
		v.clampScroll()
		return v.checkCursorChanged()
	case "g":
		v.cursor = 0
		v.scrollOffset = 0
		return v.checkCursorChanged()
	case ":":
		return func() tea.Msg { return CommandPaletteRequestMsg{} }
	}

	// Resolve configurable action keys via handler
	if v.handler != nil {
		if a, ok := v.handler.ResolveAction(keyStr); ok {
			return func() tea.Msg { return ActionRequestMsg{Action: a} }
		}
	}

	return nil
}

// handleDetailKey processes keys when the detail pane has focus.
func (v *View) handleDetailKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc", "h":
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
		// Try configurable action keys (e.g. status changes work from detail pane)
		if v.handler != nil {
			if a, ok := v.handler.ResolveAction(msg.String()); ok {
				return func() tea.Msg { return ActionRequestMsg{Action: a} }
			}
		}
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

// bottomBarLines is the number of fixed lines at the bottom (rule + help).
const bottomBarLines = 2

// treeViewHeight returns the number of visible tree rows, accounting for
// the bottom bars, repo header, filter divider, and filter bar.
func (v *View) treeViewHeight() int {
	return max(v.height-bottomBarLines-3, 1)
}

// clampScroll ensures the scroll offset keeps the cursor visible.
func (v *View) clampScroll() {
	if v.height <= 0 {
		return
	}
	visibleRows := v.treeViewHeight()
	// Cursor below visible area
	if v.cursor >= v.scrollOffset+visibleRows {
		v.scrollOffset = v.cursor - visibleRows + 1
	}
	// Cursor above visible area
	if v.cursor < v.scrollOffset {
		v.scrollOffset = v.cursor
	}
	// Don't scroll past the end
	maxOffset := max(len(v.flatNodes)-visibleRows, 0)
	v.scrollOffset = max(min(v.scrollOffset, maxOffset), 0)
}
