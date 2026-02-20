package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/core/todo"
)

// TodoPanelAction represents the action taken on a TODO item.
type TodoPanelAction int

const (
	TodoPanelNone     TodoPanelAction = iota
	TodoPanelSelect                   // enter — user wants to open/act on the item
	TodoPanelDismiss                  // d — dismiss the item
	TodoPanelComplete                 // c — complete the item
)

// TodoPanelResult holds the outcome of a user interaction with the panel.
type TodoPanelResult struct {
	Action TodoPanelAction
	Item   todo.Item
}

// TodoActionPanel is a modal overlay listing pending TODO items.
// Items are grouped by repository remote with headers.
type TodoActionPanel struct {
	items     []todo.Item
	cursor    int
	cancelled bool
	result    *TodoPanelResult
	width     int
	height    int
	scroll    int // first visible row index
}

// NewTodoActionPanel creates a new TODO action panel.
func NewTodoActionPanel(items []todo.Item, width, height int) *TodoActionPanel {
	p := &TodoActionPanel{
		items:  items,
		width:  width,
		height: height,
	}
	return p
}

// Update handles key events for the panel.
func (p *TodoActionPanel) Update(msg tea.Msg) (*TodoActionPanel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return p, nil
	}

	switch keyMsg.String() {
	case "esc", "q":
		p.cancelled = true
	case "enter":
		if len(p.items) > 0 {
			p.result = &TodoPanelResult{Action: TodoPanelSelect, Item: p.items[p.cursor]}
		}
	case "d":
		if len(p.items) > 0 {
			p.result = &TodoPanelResult{Action: TodoPanelDismiss, Item: p.items[p.cursor]}
		}
	case "c":
		if len(p.items) > 0 {
			p.result = &TodoPanelResult{Action: TodoPanelComplete, Item: p.items[p.cursor]}
		}
	case "up", "k":
		if p.cursor > 0 {
			p.cursor--
			p.ensureVisible()
		}
	case "down", "j":
		if p.cursor < len(p.items)-1 {
			p.cursor++
			p.ensureVisible()
		}
	}

	return p, nil
}

// View renders the panel content.
func (p *TodoActionPanel) View() string {
	modalWidth := max(int(float64(p.width)*0.8), 40)
	modalHeight := max(int(float64(p.height)*0.8), 10)
	listHeight := max(modalHeight-6, 3)

	title := styles.ModalTitleStyle.Render("TODO Items")

	if len(p.items) == 0 {
		empty := styles.TextMutedStyle.Render("No pending TODO items")
		help := styles.ModalHelpStyle.Render("esc close")
		content := lipgloss.JoinVertical(lipgloss.Left, title, "", empty, "", help)
		return styles.ModalStyle.Width(modalWidth).Render(content)
	}

	// Build grouped rows
	rows := p.buildRows(modalWidth - 6) // account for border + padding

	// Apply scrolling
	visible := rows
	if len(rows) > listHeight {
		end := p.scroll + listHeight
		if end > len(rows) {
			end = len(rows)
		}
		visible = rows[p.scroll:end]
	}

	list := strings.Join(visible, "\n")

	help := styles.ModalHelpStyle.Render("↑/↓ navigate  enter select  d dismiss  c complete  esc close")

	content := lipgloss.JoinVertical(lipgloss.Left, title, "", list, "", help)
	return styles.ModalStyle.Width(modalWidth).Render(content)
}

// Overlay renders the panel centered over the background.
func (p *TodoActionPanel) Overlay(background string, width, height int) string {
	modal := p.View()

	bgLayer := lipgloss.NewLayer(background)
	modalLayer := lipgloss.NewLayer(modal)

	modalW := lipgloss.Width(modal)
	modalH := lipgloss.Height(modal)
	centerX := max((width-modalW)/2, 0)
	centerY := max((height-modalH)/2, 0)
	modalLayer.X(centerX).Y(centerY).Z(1)

	return lipgloss.NewCompositor(bgLayer, modalLayer).Render()
}

// Cancelled returns true if the user dismissed the panel.
func (p *TodoActionPanel) Cancelled() bool {
	return p.cancelled
}

// Result returns the action result, or nil if none yet.
func (p *TodoActionPanel) Result() *TodoPanelResult {
	return p.result
}

// buildRows renders TODO items grouped by repo remote.
func (p *TodoActionPanel) buildRows(maxWidth int) []string {
	type group struct {
		remote string
		items  []int // indices into p.items
	}

	// Group items by repo remote, preserving order
	var groups []group
	seen := map[string]int{} // remote -> index in groups

	for i, item := range p.items {
		key := item.RepoRemote
		if key == "" {
			key = "unknown"
		}
		if idx, ok := seen[key]; ok {
			groups[idx].items = append(groups[idx].items, i)
		} else {
			seen[key] = len(groups)
			groups = append(groups, group{remote: key, items: []int{i}})
		}
	}

	var rows []string
	for gi, g := range groups {
		if gi > 0 {
			rows = append(rows, "") // spacer between groups
		}

		// Header
		header := styles.TextPrimaryBoldStyle.Render(shortRemote(g.remote))
		rows = append(rows, header)

		// Items
		for ii, idx := range g.items {
			item := p.items[idx]
			selected := idx == p.cursor

			// Tree branch character
			branch := "├─"
			if ii == len(g.items)-1 {
				branch = "└─"
			}

			// Type indicator
			var typeIcon string
			if item.Type == todo.ItemTypeFileChange {
				typeIcon = "file"
			} else {
				typeIcon = "task"
			}

			// Session label
			sessionLabel := ""
			if item.SessionID != "" {
				sessionLabel = fmt.Sprintf(" [%s]", truncate(item.SessionID, 12))
			}

			// Age
			age := todoFormatAge(item.CreatedAt)

			// Build the row content
			titleStr := truncate(item.Title, max(maxWidth-30, 20))
			meta := styles.TextMutedStyle.Render(fmt.Sprintf(" %s%s %s", typeIcon, sessionLabel, age))
			rowContent := branch + " " + titleStr + meta

			if selected {
				rowContent = styles.TextPrimaryStyle.Render("┃ ") + rowContent
			} else {
				rowContent = "  " + rowContent
			}

			rows = append(rows, rowContent)
		}
	}

	return rows
}

// ensureVisible adjusts scroll so the cursor is visible.
func (p *TodoActionPanel) ensureVisible() {
	modalHeight := max(int(float64(p.height)*0.8), 10)
	listHeight := max(modalHeight-6, 3)

	// Map cursor index to row index (accounting for headers and spacers).
	// This is approximate but sufficient: scroll to keep cursor in view.
	row := p.cursorToRow()

	if row < p.scroll {
		p.scroll = row
	}
	if row >= p.scroll+listHeight {
		p.scroll = row - listHeight + 1
	}
}

// cursorToRow estimates the row index for the current cursor position.
func (p *TodoActionPanel) cursorToRow() int {
	if len(p.items) == 0 {
		return 0
	}

	row := 0
	prevRemote := ""
	for i, item := range p.items {
		remote := item.RepoRemote
		if remote == "" {
			remote = "unknown"
		}
		if remote != prevRemote {
			if prevRemote != "" {
				row++ // spacer
			}
			row++ // header
			prevRemote = remote
		}
		if i == p.cursor {
			return row
		}
		row++ // item row
	}

	return row
}

// shortRemote extracts the owner/repo from a git remote URL.
func shortRemote(remote string) string {
	remote = strings.TrimSuffix(remote, ".git")

	// SSH format: git@github.com:org/repo
	if idx := strings.LastIndex(remote, ":"); idx >= 0 && !strings.Contains(remote, "://") {
		return remote[idx+1:]
	}

	// HTTPS format: https://github.com/org/repo
	parts := strings.Split(remote, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-2] + "/" + parts[len(parts)-1]
	}

	return remote
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

func todoFormatAge(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
