package components

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/viewport"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/core/todo"
)

const (
	todoPanelWidthPct  = 65
	todoPanelMinWidth  = 80
	todoPanelMaxHeight = 30
	todoPanelMargin    = 4
	todoPanelChrome    = 6 // title + divider + help + spacing
	todoMaxURIDisplay  = 40
)

// TodoService is the minimal interface the todo panel needs.
type TodoService interface {
	List(ctx context.Context, filter todo.ListFilter) ([]todo.Todo, error)
	Acknowledge(ctx context.Context, id string) error
	Complete(ctx context.Context, id string) error
	Dismiss(ctx context.Context, id string) error
}

// todoFilter controls which items the panel shows.
type todoFilter int

const (
	todoFilterOpen todoFilter = iota
	todoFilterAll
)

// TodoPanel is an imperative modal component. The parent dispatches keys
// via HandleKey and renders via Overlay. It does not implement tea.Model.
type TodoPanel struct {
	service  TodoService
	viewport viewport.Model
	items    []todo.Todo
	cursor   int
	filter   todoFilter
	width    int
	height   int
}

// NewTodoPanel creates a todo panel, loads items, and bulk-acknowledges
// all pending items (the "I've seen these" UX).
func NewTodoPanel(service TodoService, width, height int) *TodoPanel {
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
	p.acknowledgeAllPending()

	return p
}

// OpenCount returns the number of open (pending + acknowledged) items.
func (p *TodoPanel) OpenCount() int {
	count := 0
	for _, t := range p.items {
		if isOpen(t.Status) {
			count++
		}
	}
	return count
}

// HandleKey processes a key press. Returns true if the panel should close.
func (p *TodoPanel) HandleKey(keyStr string) (close bool) {
	filtered := p.filteredItems()

	switch keyStr {
	case "esc", "q":
		return true
	case "j", "down":
		if p.cursor < len(filtered)-1 {
			p.cursor++
			p.refreshContent()
		}
	case "k", "up":
		if p.cursor > 0 {
			p.cursor--
			p.refreshContent()
		}
	case "tab":
		if p.filter == todoFilterOpen {
			p.filter = todoFilterAll
		} else {
			p.filter = todoFilterOpen
		}
		p.cursor = 0
		p.refreshContent()
	case "c":
		p.completeSelected(filtered)
	case "d":
		p.dismissSelected(filtered)
	}

	return false
}

// ScrollUp scrolls the viewport up one line.
func (p *TodoPanel) ScrollUp() { p.viewport.ScrollUp(1) }

// ScrollDown scrolls the viewport down one line.
func (p *TodoPanel) ScrollDown() { p.viewport.ScrollDown(1) }

// Overlay renders the todo panel centered over the background.
func (p *TodoPanel) Overlay(background string, width, height int) string {
	modalWidth := calcTodoPanelWidth(width)
	modalHeight := min(height-todoPanelMargin, todoPanelMaxHeight)

	// Title bar: filter tabs on left, title + scroll on right
	openLabel := "Open"
	allLabel := "All"
	if p.filter == todoFilterOpen {
		openLabel = styles.TextPrimaryBoldStyle.Render(openLabel)
		allLabel = styles.TextMutedStyle.Render(allLabel)
	} else {
		openLabel = styles.TextMutedStyle.Render(openLabel)
		allLabel = styles.TextPrimaryBoldStyle.Render(allLabel)
	}
	filterTabs := openLabel + " | " + allLabel

	scrollInfo := ""
	if p.viewport.TotalLineCount() > p.viewport.VisibleLineCount() {
		scrollInfo = styles.TextMutedStyle.Render(
			fmt.Sprintf(" (%.0f%%)", p.viewport.ScrollPercent()*100),
		)
	}
	title := styles.ModalTitleStyle.Render(styles.IconCheckList + "Todos" + scrollInfo)

	contentWidth := modalWidth - 6
	filtersWidth := lipgloss.Width(filterTabs)
	titleWidth := lipgloss.Width(title)
	spacer := contentWidth - filtersWidth - titleWidth
	if spacer < 1 {
		spacer = 1
	}
	titleBar := filterTabs + strings.Repeat(" ", spacer) + title

	divider := styles.TextSurfaceStyle.Render(strings.Repeat("─", contentWidth))
	helpBar := styles.ModalHelpStyle.Render("[j/k] navigate  [tab] filter  [c] complete  [d] dismiss  [esc] close")

	modalContent := lipgloss.JoinVertical(
		lipgloss.Left,
		titleBar,
		divider,
		p.viewport.View(),
		helpBar,
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

// --- internal ---

func (p *TodoPanel) loadItems() {
	items, err := p.service.List(context.Background(), todo.ListFilter{})
	if err != nil {
		p.items = nil
		p.refreshContent()
		return
	}

	// Sort: open items first (pending, acknowledged), then closed (completed, dismissed).
	// Within each group, most recent first.
	sort.SliceStable(items, func(i, j int) bool {
		iOpen := isOpen(items[i].Status)
		jOpen := isOpen(items[j].Status)
		if iOpen != jOpen {
			return iOpen
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})

	p.items = items
	p.refreshContent()
}

func (p *TodoPanel) acknowledgeAllPending() {
	ctx := context.Background()
	for _, t := range p.items {
		if t.Status == todo.StatusPending {
			_ = p.service.Acknowledge(ctx, t.ID)
		}
	}
}

func (p *TodoPanel) filteredItems() []todo.Todo {
	if p.filter == todoFilterAll {
		return p.items
	}
	var out []todo.Todo
	for _, t := range p.items {
		if isOpen(t.Status) {
			out = append(out, t)
		}
	}
	return out
}

func (p *TodoPanel) refreshContent() {
	filtered := p.filteredItems()
	if len(filtered) == 0 {
		p.viewport.SetContent(styles.TextMutedStyle.Render("No todo items"))
		return
	}

	contentWidth := calcTodoPanelWidth(p.width) - 4

	var b strings.Builder
	for i, t := range filtered {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(formatTodoItem(i, p.cursor, t, contentWidth))
	}

	p.viewport.SetContent(b.String())
}

func (p *TodoPanel) completeSelected(filtered []todo.Todo) {
	if p.cursor >= len(filtered) {
		return
	}
	t := filtered[p.cursor]
	if !isOpen(t.Status) {
		return
	}
	_ = p.service.Complete(context.Background(), t.ID)
	p.loadItems()
	p.clampCursor()
}

func (p *TodoPanel) dismissSelected(filtered []todo.Todo) {
	if p.cursor >= len(filtered) {
		return
	}
	t := filtered[p.cursor]
	if !isOpen(t.Status) {
		return
	}
	_ = p.service.Dismiss(context.Background(), t.ID)
	p.loadItems()
	p.clampCursor()
}

func (p *TodoPanel) clampCursor() {
	if n := len(p.filteredItems()); p.cursor >= n && p.cursor > 0 {
		p.cursor = n - 1
	}
}

func formatTodoItem(idx, cursor int, t todo.Todo, maxWidth int) string {
	// Cursor indicator
	var cur string
	if idx == cursor {
		cur = styles.TextPrimaryStyle.Render(styles.IconTodoCursor)
	} else {
		cur = "  "
	}

	// Status icon
	var icon string
	switch t.Status {
	case todo.StatusPending:
		icon = styles.TextWarningStyle.Render(styles.IconTodoPending)
	case todo.StatusAcknowledged:
		icon = styles.TextPrimaryStyle.Render(styles.IconTodoAcknowledged)
	case todo.StatusCompleted:
		icon = styles.TextSuccessStyle.Render(styles.IconTodoCompleted)
	case todo.StatusDismissed:
		icon = styles.TextMutedStyle.Render(styles.IconTodoDismissed)
	}

	// Scheme tag
	scheme := ""
	if t.URI.Scheme != "" {
		scheme = styles.TextMutedStyle.Render("["+t.URI.Scheme+"]") + " "
	}

	// URI value (truncated)
	uriDisplay := ""
	if !t.URI.IsZero() && t.URI.Value != "" {
		val := t.URI.Value
		if len(val) > todoMaxURIDisplay {
			val = val[:todoMaxURIDisplay-1] + "…"
		}
		uriDisplay = " " + styles.TextMutedStyle.Render(val)
	}

	line := fmt.Sprintf("%s %s %s%s%s", cur, icon, scheme, t.Title, uriDisplay)
	return ansi.Truncate(line, maxWidth, "…")
}

func isOpen(s todo.Status) bool {
	return s == todo.StatusPending || s == todo.StatusAcknowledged
}

func calcTodoPanelWidth(termWidth int) int {
	available := max(termWidth-todoPanelMargin, 1)
	target := termWidth * todoPanelWidthPct / 100
	return min(max(target, todoPanelMinWidth), available)
}
