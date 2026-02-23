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

	// Ensure cursor is visible
	if p.cursor < p.viewport.YOffset() {
		p.viewport.SetYOffset(p.cursor)
	} else if p.cursor >= p.viewport.YOffset()+p.viewport.VisibleLineCount() {
		p.viewport.SetYOffset(p.cursor - p.viewport.VisibleLineCount() + 1)
	}
}

func (p *TodoPanel) formatItem(item todo.Todo, selected bool, maxWidth int) string {
	// Status indicator
	var statusIcon string
	switch item.Status {
	case todo.StatusPending:
		statusIcon = styles.TextWarningStyle.Render("○")
	case todo.StatusAcknowledged:
		statusIcon = styles.TextPrimaryStyle.Render("◐")
	case todo.StatusCompleted:
		statusIcon = styles.TextSuccessStyle.Render("●")
	case todo.StatusDismissed:
		statusIcon = styles.TextMutedStyle.Render("⊘")
	}

	// Scheme tag
	scheme := ""
	if !item.URI.IsEmpty() {
		scheme = styles.TextMutedStyle.Render("[" + item.URI.Scheme + "]")
	}

	// Title
	title := item.Title
	if selected {
		title = styles.TextPrimaryBoldStyle.Render(title)
	}

	// Cursor
	cursor := "  "
	if selected {
		cursor = styles.TextPrimaryStyle.Render("▸ ")
	}

	line := fmt.Sprintf("%s%s %s %s", cursor, statusIcon, scheme, title)

	// Add URI value if present (truncated)
	if !item.URI.IsEmpty() && item.URI.Value != "" {
		val := item.URI.Value
		if len(val) > 40 {
			val = val[:37] + "..."
		}
		line += " " + styles.TextMutedStyle.Render(val)
	}

	// Truncate to max width using ANSI-aware truncation
	if lipgloss.Width(line) > maxWidth {
		line = ansi.Truncate(line, maxWidth, "")
	}

	return line
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
	if err := p.service.Complete(context.Background(), item.ID); err != nil {
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
	if err := p.service.Acknowledge(context.Background(), item.ID); err != nil {
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
	if err := p.service.Dismiss(context.Background(), item.ID); err != nil {
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
	for _, item := range p.allItems {
		if item.Status == todo.StatusPending {
			if err := p.service.Acknowledge(context.Background(), item.ID); err != nil {
				log.Error().Err(err).Str("id", item.ID).Msg("failed to auto-acknowledge todo")
			}
		}
	}
	p.loadItems()
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
