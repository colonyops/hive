package tui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/rs/zerolog/log"

	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/core/todo"
	"github.com/colonyops/hive/internal/hive"
)

const (
	todoPanelWidthPct  = 65
	todoPanelMinWidth  = 80
	todoPanelMaxHeight = 30
	todoPanelMargin    = 4
	todoPanelChrome    = 6 // title + divider + help + spacing
)

// todoFilter controls which items are visible in the panel.
type todoFilter int

const (
	todoFilterOpen todoFilter = iota
	todoFilterAll
)

var todoFilterLabels = [...]string{
	todoFilterOpen: "Open",
	todoFilterAll:  "All",
}

// TodoPanel displays an interactive list of todo items.
type TodoPanel struct {
	service  *hive.TodoService
	viewport viewport.Model
	allItems []todo.Todo // unfiltered list from store
	items    []todo.Todo // filtered view
	ackErrs  int
	cursor   int
	filter   todoFilter
	width    int
	height   int
}

// NewTodoPanel creates a new interactive todo panel modal.
func NewTodoPanel(service *hive.TodoService, width, height int) *TodoPanel {
	modalWidth := calcTodoPanelWidth(width)
	modalHeight := min(height-todoPanelMargin, todoPanelMaxHeight)
	contentHeight := modalHeight - todoPanelChrome

	vp := viewport.New(
		viewport.WithWidth(modalWidth-4),
		viewport.WithHeight(contentHeight),
	)

	p := &TodoPanel{
		service:  service,
		viewport: vp,
		width:    width,
		height:   height,
	}

	p.loadItems()
	p.acknowledgeAll()
	return p
}

func (p *TodoPanel) loadItems() {
	items, err := p.service.List(context.Background(), todo.ListFilter{})
	if err != nil {
		log.Error().Err(err).Msg("failed to load todo items")
		p.allItems = nil
		p.items = nil
		p.viewport.SetContent(styles.TextErrorStyle.Render(fmt.Sprintf("failed to load todos: %v", err)))
		return
	}

	// Sort: open first, then completed/dismissed
	p.allItems = make([]todo.Todo, 0, len(items))
	closed := make([]todo.Todo, 0, len(items))
	for _, item := range items {
		if item.Status == todo.StatusPending || item.Status == todo.StatusAcknowledged {
			p.allItems = append(p.allItems, item)
		} else {
			closed = append(closed, item)
		}
	}
	p.allItems = append(p.allItems, closed...)

	p.applyFilter()
}

func (p *TodoPanel) applyFilter() {
	switch p.filter {
	case todoFilterOpen:
		filtered := make([]todo.Todo, 0, len(p.allItems))
		for _, item := range p.allItems {
			if item.Status == todo.StatusPending || item.Status == todo.StatusAcknowledged {
				filtered = append(filtered, item)
			}
		}
		p.items = filtered
	default:
		p.items = p.allItems
	}

	if p.cursor >= len(p.items) {
		p.cursor = max(len(p.items)-1, 0)
	}

	p.refreshContent()
}

const linesPerTodoItem = 2

func (p *TodoPanel) refreshContent() {
	if len(p.items) == 0 {
		p.viewport.SetContent(styles.TextMutedStyle.Render("No todo items"))
		return
	}

	modalWidth := calcTodoPanelWidth(p.width)
	contentWidth := modalWidth - 6 // account for modal padding

	var b strings.Builder
	for i, item := range p.items {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(p.formatItem(item, i == p.cursor, contentWidth))
	}

	p.viewport.SetContent(b.String())

	// Ensure cursor is visible (each item is linesPerTodoItem lines)
	cursorLine := p.cursor * linesPerTodoItem
	if cursorLine < p.viewport.YOffset() {
		p.viewport.SetYOffset(cursorLine)
	} else if cursorLine+linesPerTodoItem > p.viewport.YOffset()+p.viewport.VisibleLineCount() {
		p.viewport.SetYOffset(cursorLine + linesPerTodoItem - p.viewport.VisibleLineCount())
	}
}

func (p *TodoPanel) formatItem(item todo.Todo, selected bool, maxWidth int) string {
	// Status indicator
	var statusIcon string
	switch item.Status {
	case todo.StatusPending:
		statusIcon = styles.TextWarningStyle.Render(styles.IconTodoPending)
	case todo.StatusAcknowledged:
		statusIcon = styles.TextPrimaryStyle.Render(styles.IconTodoAcknowledged)
	case todo.StatusCompleted:
		statusIcon = styles.TextSuccessStyle.Render(styles.IconTodoCompleted)
	case todo.StatusDismissed:
		statusIcon = styles.TextMutedStyle.Render(styles.IconTodoDismissed)
	}

	// Cursor
	cursor := "  "
	if selected {
		cursor = styles.TextPrimaryStyle.Render(styles.IconSelector + " ")
	}

	// Title
	title := item.Title
	if selected {
		title = styles.TextPrimaryBoldStyle.Render(title)
	}

	// Source tag right-aligned: [agent], [human], [system]
	tag := styles.TextMutedStyle.Render("[" + string(item.Source) + "]")
	tagWidth := lipgloss.Width(tag)

	// Row 1: cursor + icon + title ... [source]
	prefix := fmt.Sprintf("%s%s %s", cursor, statusIcon, title)
	prefixWidth := lipgloss.Width(prefix)
	gap := max(maxWidth-prefixWidth-tagWidth, 1)
	row1 := prefix + strings.Repeat(" ", gap) + tag

	if lipgloss.Width(row1) > maxWidth {
		// Truncate title portion, preserving the tag
		available := maxWidth - tagWidth - 1
		row1 = ansi.Truncate(prefix, available, "…") + " " + tag
	}

	// Row 2: indented URI in muted style
	indent := "     " // align under title text
	var row2 string
	if !item.URI.IsEmpty() {
		uri := item.URI.String()
		maxURI := maxWidth - len(indent)
		if len(uri) > maxURI {
			uri = uri[:max(maxURI-3, 0)] + "..."
		}
		row2 = indent + styles.TextMutedStyle.Render(uri)
	} else {
		row2 = indent + styles.TextMutedStyle.Render("no uri")
	}

	return row1 + "\n" + row2
}

// MoveUp moves the cursor up.
func (p *TodoPanel) MoveUp() {
	if p.cursor > 0 {
		p.cursor--
		p.refreshContent()
	}
}

// MoveDown moves the cursor down.
func (p *TodoPanel) MoveDown() {
	if p.cursor < len(p.items)-1 {
		p.cursor++
		p.refreshContent()
	}
}

// ScrollUp scrolls the viewport up.
func (p *TodoPanel) ScrollUp() {
	p.viewport.ScrollUp(1)
}

// ScrollDown scrolls the viewport down.
func (p *TodoPanel) ScrollDown() {
	p.viewport.ScrollDown(1)
}

// CompleteCurrent marks the selected todo as completed.
func (p *TodoPanel) CompleteCurrent() error {
	if p.cursor >= len(p.items) {
		return nil
	}
	item := p.items[p.cursor]
	if item.Status == todo.StatusCompleted || item.Status == todo.StatusDismissed {
		return nil
	}
	if _, err := p.service.Complete(context.Background(), item.ID); err != nil {
		return err
	}
	p.loadItems()
	return nil
}

// AcknowledgeCurrent marks the selected todo as acknowledged.
func (p *TodoPanel) AcknowledgeCurrent() error {
	if p.cursor >= len(p.items) {
		return nil
	}
	item := p.items[p.cursor]
	if item.Status != todo.StatusPending {
		return nil
	}
	if _, err := p.service.Acknowledge(context.Background(), item.ID); err != nil {
		return err
	}
	p.loadItems()
	return nil
}

// DismissCurrent marks the selected todo as dismissed.
func (p *TodoPanel) DismissCurrent() error {
	if p.cursor >= len(p.items) {
		return nil
	}
	item := p.items[p.cursor]
	if item.Status == todo.StatusCompleted || item.Status == todo.StatusDismissed {
		return nil
	}
	if _, err := p.service.Dismiss(context.Background(), item.ID); err != nil {
		return err
	}
	p.loadItems()
	return nil
}

// CycleFilter advances to the next filter tab.
func (p *TodoPanel) CycleFilter() {
	p.filter = (p.filter + 1) % todoFilter(len(todoFilterLabels))
	p.cursor = 0
	p.applyFilter()
}

// acknowledgeAll marks all pending items as acknowledged.
func (p *TodoPanel) acknowledgeAll() {
	p.ackErrs = 0
	for _, item := range p.allItems {
		if item.Status == todo.StatusPending {
			if _, err := p.service.Acknowledge(context.Background(), item.ID); err != nil {
				p.ackErrs++
				log.Warn().Err(err).Str("id", item.ID).Msg("failed to auto-acknowledge todo")
			}
		}
	}
	p.loadItems()
}

// AcknowledgeErrorCount returns the number of auto-acknowledge failures during panel open.
func (p *TodoPanel) AcknowledgeErrorCount() int {
	return p.ackErrs
}

// PendingCount returns the number of pending items across all items.
func (p *TodoPanel) PendingCount() int {
	count := 0
	for _, item := range p.allItems {
		if item.Status == todo.StatusPending {
			count++
		}
	}
	return count
}

// OpenCount returns the number of open (pending + acknowledged) items across all items.
func (p *TodoPanel) OpenCount() int {
	count := 0
	for _, item := range p.allItems {
		if item.Status == todo.StatusPending || item.Status == todo.StatusAcknowledged {
			count++
		}
	}
	return count
}

// CurrentItem returns the todo item under the cursor, or nil if empty.
func (p *TodoPanel) CurrentItem() *todo.Todo {
	if p.cursor >= len(p.items) || len(p.items) == 0 {
		return nil
	}
	item := p.items[p.cursor]
	return &item
}

// Overlay renders the todo panel centered over the background.
func (p *TodoPanel) Overlay(background string, width, height int) string {
	modalWidth := calcTodoPanelWidth(width)
	modalHeight := min(height-todoPanelMargin, todoPanelMaxHeight)

	scrollInfo := ""
	if p.viewport.TotalLineCount() > p.viewport.VisibleLineCount() {
		scrollInfo = styles.TextMutedStyle.Render(
			fmt.Sprintf(" (%.0f%%)", p.viewport.ScrollPercent()*100),
		)
	}

	// Build filter tabs (left side of title bar)
	var tabs []string
	for i, label := range todoFilterLabels {
		if todoFilter(i) == p.filter {
			tabs = append(tabs, styles.TextPrimaryBoldStyle.Render(label))
		} else {
			tabs = append(tabs, styles.TextMutedStyle.Render(label))
		}
	}
	filterBar := strings.Join(tabs, styles.TextMutedStyle.Render(" | "))

	// Title on right side of title bar
	title := styles.ModalTitleStyle.Render(styles.IconTodo + " Todos" + scrollInfo)
	contentWidth := modalWidth - 6
	titleBarGap := max(contentWidth-lipgloss.Width(filterBar)-lipgloss.Width(title), 1)
	titleBar := filterBar + strings.Repeat(" ", titleBarGap) + title

	divider := styles.TextSurfaceStyle.Render(strings.Repeat("─", max(contentWidth, 1)))
	modalContent := lipgloss.JoinVertical(
		lipgloss.Left,
		titleBar,
		divider,
		p.viewport.View(),
		styles.ModalHelpStyle.Render("[j/k] navigate  [tab] filter  [enter] open  [c] complete  [d] dismiss  [esc] close"),
	)

	modal := styles.ModalStyle.
		Width(modalWidth).
		Height(modalHeight).
		Render(modalContent)

	bgLayer := lipgloss.NewLayer(background)
	modalLayer := lipgloss.NewLayer(modal)

	modalW := lipgloss.Width(modal)
	modalH := lipgloss.Height(modal)
	centerX := max((width-modalW)/2, 0)
	centerY := max((height-modalH)/2, 0)
	modalLayer.X(centerX).Y(centerY).Z(1)

	return lipgloss.NewCompositor(bgLayer, modalLayer).Render()
}

func calcTodoPanelWidth(termWidth int) int {
	available := max(termWidth-todoPanelMargin, 1)
	target := termWidth * todoPanelWidthPct / 100
	return min(max(target, todoPanelMinWidth), available)
}
