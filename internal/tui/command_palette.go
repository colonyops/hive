package tui

import (
	"io"
	"sort"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/hay-kot/hive/internal/core/config"
	"github.com/hay-kot/hive/internal/core/session"
)

// CommandEntry represents an item in the command palette.
type CommandEntry struct {
	Name    string
	Command config.UserCommand
}

// FilterValue implements list.Item.
func (c CommandEntry) FilterValue() string { return c.Name }

// commandItemDelegate renders CommandEntry in the list.
type commandItemDelegate struct{}

func (d commandItemDelegate) Height() int                             { return 1 }
func (d commandItemDelegate) Spacing() int                            { return 0 }
func (d commandItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d commandItemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	entry, ok := listItem.(CommandEntry)
	if !ok {
		return
	}

	isSelected := index == m.Index()

	// Style for selected vs unselected
	nameStyle := lipgloss.NewStyle().Foreground(colorWhite)
	helpStyle := lipgloss.NewStyle().Foreground(colorGray)
	cursor := "  "
	if isSelected {
		nameStyle = nameStyle.Foreground(colorBlue).Bold(true)
		helpStyle = helpStyle.Foreground(colorBlue)
		cursor = "> "
	}

	// Render: cursor + name + help (if present)
	_, _ = io.WriteString(w, cursor)
	_, _ = io.WriteString(w, nameStyle.Render(entry.Name))
	if entry.Command.Help != "" {
		_, _ = io.WriteString(w, " ")
		_, _ = io.WriteString(w, helpStyle.Render("- "+entry.Command.Help))
	}
}

// CommandPalette is a vim-style command palette for user commands.
type CommandPalette struct {
	commands  []CommandEntry
	list      list.Model
	width     int
	height    int
	session   *session.Session
	selected  bool
	cancelled bool
}

// NewCommandPalette creates a new command palette with the given commands.
func NewCommandPalette(cmds map[string]config.UserCommand, sess *session.Session, width, height int) *CommandPalette {
	// Sort command names for consistent ordering
	names := make([]string, 0, len(cmds))
	for name := range cmds {
		names = append(names, name)
	}
	sort.Strings(names)

	// Build entries
	entries := make([]CommandEntry, len(names))
	listItems := make([]list.Item, len(names))
	for i, name := range names {
		entries[i] = CommandEntry{Name: name, Command: cmds[name]}
		listItems[i] = entries[i]
	}

	// Calculate list height (max 10 items or available height)
	listHeight := min(len(entries), 10)

	delegate := commandItemDelegate{}
	l := list.New(listItems, delegate, width-6, listHeight)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.SetShowFilter(false)
	l.SetShowHelp(false)
	l.Styles.TitleBar = lipgloss.NewStyle()

	// Configure filter input styles
	l.FilterInput.Prompt = "/ "
	filterStyles := textinput.DefaultStyles(true)
	filterStyles.Focused.Prompt = lipgloss.NewStyle().Foreground(colorBlue)
	filterStyles.Cursor.Color = colorBlue
	l.FilterInput.SetStyles(filterStyles)

	return &CommandPalette{
		commands: entries,
		list:     l,
		width:    width,
		height:   height,
		session:  sess,
	}
}

// Update handles messages for the command palette.
func (p *CommandPalette) Update(msg tea.Msg) (*CommandPalette, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "enter":
			if p.list.SelectedItem() != nil {
				p.selected = true
			}
			return p, nil
		case "esc":
			if p.list.SettingFilter() {
				// Let the list handle filter cancel
			} else {
				p.cancelled = true
				return p, nil
			}
		}
	}

	var cmd tea.Cmd
	p.list, cmd = p.list.Update(msg)
	return p, cmd
}

// View renders the command palette.
func (p *CommandPalette) View() string {
	title := modalTitleStyle.Render("Command Palette")

	var content string
	if p.list.SettingFilter() {
		content = lipgloss.JoinVertical(lipgloss.Left,
			title,
			"",
			p.list.FilterInput.View(),
			p.list.View(),
		)
	} else {
		content = lipgloss.JoinVertical(lipgloss.Left,
			title,
			"",
			p.list.View(),
		)
	}

	help := modalHelpStyle.Render("↑/k up  ↓/j down  / filter  enter select  esc cancel")
	content = lipgloss.JoinVertical(lipgloss.Left, content, help)

	return modalStyle.Render(content)
}

// Overlay renders the command palette as a layer over the given background.
func (p *CommandPalette) Overlay(background string, width, height int) string {
	modal := p.View()

	bgLayer := lipgloss.NewLayer(background)
	modalLayer := lipgloss.NewLayer(modal)

	// Center the modal
	modalW := lipgloss.Width(modal)
	modalH := lipgloss.Height(modal)
	centerX := (width - modalW) / 2
	centerY := (height - modalH) / 2
	modalLayer.X(centerX).Y(centerY).Z(1)

	compositor := lipgloss.NewCompositor(bgLayer, modalLayer)
	return compositor.Render()
}

// SelectedCommand returns the selected command entry, if any.
func (p *CommandPalette) SelectedCommand() (*CommandEntry, bool) {
	if !p.selected {
		return nil, false
	}
	item := p.list.SelectedItem()
	if item == nil {
		return nil, false
	}
	if entry, ok := item.(CommandEntry); ok {
		return &entry, true
	}
	return nil, false
}

// Cancelled returns true if the user cancelled the palette.
func (p *CommandPalette) Cancelled() bool {
	return p.cancelled
}

// IsFiltering returns whether the palette is currently filtering.
func (p *CommandPalette) IsFiltering() bool {
	return p.list.SettingFilter()
}

// KeyMap returns keys that the command palette uses (for help integration).
func (p *CommandPalette) KeyMap() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	}
}

// Session returns the session context for template rendering.
func (p *CommandPalette) Session() *session.Session {
	return p.session
}
