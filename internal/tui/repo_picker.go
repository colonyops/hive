package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/colonyops/hive/internal/core/styles"
)

// RepoPicker is a simple modal for selecting a repository scope.
type RepoPicker struct {
	repos        []string
	filtered     []string
	cursor       int
	scrollOffset int
	query        string
	width        int
	height       int
	cancelled    bool
	selected     string
}

// NewRepoPicker creates a new repository picker modal.
func NewRepoPicker(repos []string, currentRepo string, width, height int) *RepoPicker {
	p := &RepoPicker{
		repos:    repos,
		filtered: repos,
		width:    width,
		height:   height,
	}
	// Pre-select current repo.
	for i, r := range repos {
		if r == currentRepo {
			p.cursor = i
			break
		}
	}
	return p
}

// Update handles key events for the repo picker.
func (p *RepoPicker) Update(msg tea.Msg) (*RepoPicker, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return p, nil
	}

	switch keyMsg.String() {
	case "esc", "q":
		p.cancelled = true
	case "enter":
		if len(p.filtered) > 0 && p.cursor < len(p.filtered) {
			p.selected = p.filtered[p.cursor]
		}
	case "up":
		if p.cursor > 0 {
			p.cursor--
			p.clampScroll()
		}
	case "down":
		if p.cursor < len(p.filtered)-1 {
			p.cursor++
			p.clampScroll()
		}
	case "backspace":
		if len(p.query) > 0 {
			p.query = p.query[:len(p.query)-1]
			p.applyFilter()
		}
	default:
		// Single printable character → append to query.
		if len(keyMsg.String()) == 1 {
			p.query += keyMsg.String()
			p.applyFilter()
		}
	}
	return p, nil
}

func (p *RepoPicker) applyFilter() {
	if p.query == "" {
		p.filtered = p.repos
	} else {
		q := strings.ToLower(p.query)
		filtered := make([]string, 0, len(p.repos))
		for _, r := range p.repos {
			if strings.Contains(strings.ToLower(r), q) {
				filtered = append(filtered, r)
			}
		}
		p.filtered = filtered
	}
	p.cursor = 0
	p.scrollOffset = 0
}

func (p *RepoPicker) visibleCount() int {
	return min(len(p.filtered), max(p.height/3, 5))
}

func (p *RepoPicker) clampScroll() {
	mv := p.visibleCount()
	if p.cursor < p.scrollOffset {
		p.scrollOffset = p.cursor
	} else if p.cursor >= p.scrollOffset+mv {
		p.scrollOffset = p.cursor - mv + 1
	}
	maxOffset := max(len(p.filtered)-mv, 0)
	p.scrollOffset = min(max(p.scrollOffset, 0), maxOffset)
}

// View renders the picker content.
func (p *RepoPicker) View() string {
	modalWidth := max(int(float64(p.width)*0.6), 40)

	title := styles.ModalTitleStyle.Render("Select Repository")

	// Search input
	searchLine := styles.TextMutedStyle.Render("> ") + p.query + styles.TextMutedStyle.Render("█")

	// List items
	maxVisible := p.visibleCount()
	var lines []string
	for i := range maxVisible {
		idx := i + p.scrollOffset
		if idx >= len(p.filtered) {
			break
		}
		repo := p.filtered[idx]
		if idx == p.cursor {
			lines = append(lines, styles.TextPrimaryBoldStyle.Render("▸ "+repo))
		} else {
			lines = append(lines, "  "+repo)
		}
	}
	if len(p.filtered) == 0 {
		lines = append(lines, styles.TextMutedStyle.Render("  no matching repositories"))
	}

	listView := strings.Join(lines, "\n")
	help := styles.ModalHelpStyle.Render("↑/↓ navigate  enter select  esc/q cancel")

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		"",
		searchLine,
		"",
		listView,
		"",
		help,
	)

	return styles.ModalStyle.Width(modalWidth).Render(content)
}

// Overlay renders the picker centered over the background.
func (p *RepoPicker) Overlay(bg string, w, h int) string {
	return centeredOverlay(bg, p.View(), w, h)
}

// Cancelled returns true if the user dismissed the picker.
func (p *RepoPicker) Cancelled() bool {
	return p.cancelled
}

// Selected returns the chosen repo key, or empty string if none selected.
func (p *RepoPicker) Selected() string {
	return p.selected
}
