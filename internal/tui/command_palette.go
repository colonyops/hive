package tui

import (
	"sort"
	"strings"

	"charm.land/bubbles/v2/key"
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

// CommandPalette is a vim-style command palette for user commands.
type CommandPalette struct {
	commands     []CommandEntry
	input        textinput.Model
	filteredList []CommandEntry
	selectedIdx  int
	scrollOffset int
	width        int
	height       int
	session      *session.Session
	selected     bool
	cancelled    bool
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
	for i, name := range names {
		entries[i] = CommandEntry{Name: name, Command: cmds[name]}
	}

	// Create text input
	input := textinput.New()
	input.Placeholder = "command [args...]"
	input.Prompt = ":"
	input.Focus()
	inputStyles := textinput.DefaultStyles(true)
	inputStyles.Focused.Prompt = lipgloss.NewStyle().Foreground(colorBlue)
	inputStyles.Cursor.Color = colorBlue
	input.SetWidth(40)
	input.SetStyles(inputStyles)

	p := &CommandPalette{
		commands:     entries,
		input:        input,
		filteredList: entries, // Start with all commands visible
		selectedIdx:  0,
		width:        width,
		height:       height,
		session:      sess,
	}

	return p
}

// Update handles messages for the command palette.
func (p *CommandPalette) Update(msg tea.Msg) (*CommandPalette, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "enter":
			if len(p.filteredList) > 0 && p.selectedIdx < len(p.filteredList) {
				p.selected = true
			}
			return p, nil
		case "esc":
			p.cancelled = true
			return p, nil
		case "tab":
			// Auto-fill with selected command
			if len(p.filteredList) > 0 && p.selectedIdx < len(p.filteredList) {
				selected := p.filteredList[p.selectedIdx]
				parsed := ParseCommandInput(p.input.Value())

				// Reconstruct input: command name + existing args
				newInput := selected.Name
				if len(parsed.Args) > 0 {
					newInput += " " + strings.Join(parsed.Args, " ")
				}

				p.input.SetValue(newInput)
				// Move cursor to end
				p.input.SetCursor(len(newInput))
			}
			return p, nil
		case "up", "ctrl+k":
			if p.selectedIdx > 0 {
				p.selectedIdx--
				p.adjustScroll()
			}
			return p, nil
		case "down", "ctrl+j":
			if p.selectedIdx < len(p.filteredList)-1 {
				p.selectedIdx++
				p.adjustScroll()
			}
			return p, nil
		case "ctrl+p": // Also support vim-style up
			if p.selectedIdx > 0 {
				p.selectedIdx--
				p.adjustScroll()
			}
			return p, nil
		case "ctrl+n": // Also support vim-style down
			if p.selectedIdx < len(p.filteredList)-1 {
				p.selectedIdx++
				p.adjustScroll()
			}
			return p, nil
		}
	}

	// Update the text input
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)

	// Filter commands based on input
	p.updateFilter()

	return p, cmd
}

// updateFilter filters the command list based on the current input.
func (p *CommandPalette) updateFilter() {
	inputVal := p.input.Value()

	// Parse input to get command name (ignore args for filtering)
	parsed := ParseCommandInput(inputVal)

	// If no input, show all commands
	if parsed.Name == "" {
		p.filteredList = p.commands
		p.selectedIdx = 0
		p.scrollOffset = 0
		return
	}

	// Filter commands by prefix match
	filtered := make([]CommandEntry, 0, len(p.commands))
	for _, cmd := range p.commands {
		if strings.HasPrefix(cmd.Name, parsed.Name) {
			filtered = append(filtered, cmd)
		}
	}

	// If no prefix matches, try substring match
	if len(filtered) == 0 {
		for _, cmd := range p.commands {
			if strings.Contains(cmd.Name, parsed.Name) {
				filtered = append(filtered, cmd)
			}
		}
	}

	p.filteredList = filtered

	// Reset selection and scroll when filter changes
	p.selectedIdx = 0
	p.scrollOffset = 0
}

// adjustScroll updates the scroll offset to keep the selected item visible.
func (p *CommandPalette) adjustScroll() {
	maxVisible := 12

	// If selected item is above the visible window, scroll up
	if p.selectedIdx < p.scrollOffset {
		p.scrollOffset = p.selectedIdx
	}

	// If selected item is below the visible window, scroll down
	if p.selectedIdx >= p.scrollOffset+maxVisible {
		p.scrollOffset = p.selectedIdx - maxVisible + 1
	}
}

// wrapText wraps text to fit within maxWidth, returning up to maxLines.
// If text exceeds maxLines, the last line is truncated with "...".
func wrapText(text string, maxWidth, maxLines int) []string {
	if text == "" {
		return nil
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	lines := make([]string, 0, maxLines)
	currentLine := ""

	for _, word := range words {
		testLine := currentLine
		if testLine != "" {
			testLine += " "
		}
		testLine += word

		if len(testLine) <= maxWidth {
			currentLine = testLine
		} else {
			// Current line is full, start new line
			if currentLine != "" {
				if len(lines) < maxLines {
					lines = append(lines, currentLine)
					currentLine = word
				} else {
					// Already at max lines, truncate
					if len(currentLine)+4 > maxWidth {
						// Truncate current line to fit "..."
						currentLine = currentLine[:maxWidth-3] + "..."
					} else {
						currentLine += "..."
					}
					lines[len(lines)-1] = currentLine
					return lines
				}
			} else {
				// Word itself is longer than maxWidth, just use it
				currentLine = word
			}
		}
	}

	// Add remaining text if under line limit
	if currentLine != "" && len(lines) < maxLines {
		lines = append(lines, currentLine)
	} else if currentLine != "" && len(lines) == maxLines {
		// Truncate last line
		lastLine := lines[len(lines)-1]
		if len(lastLine)+4 > maxWidth {
			lines[len(lines)-1] = lastLine[:maxWidth-3] + "..."
		} else {
			lines[len(lines)-1] = lastLine + "..."
		}
	}

	return lines
}

// View renders the command palette.
func (p *CommandPalette) View() string {
	title := modalTitleStyle.Render("Command Palette")

	// Render input
	inputView := p.input.View()

	// Render suggestions (max 12 visible)
	maxVisible := 12
	suggestions := make([]string, 0, min(len(p.filteredList), maxVisible))

	// Set reasonable width for suggestions content (modal will add padding/border)
	contentWidth := 90

	// Calculate visible window based on scroll offset
	endIdx := min(p.scrollOffset+maxVisible, len(p.filteredList))
	visibleCommands := p.filteredList[p.scrollOffset:endIdx]

	for i, entry := range visibleCommands {
		actualIdx := p.scrollOffset + i
		isSelected := actualIdx == p.selectedIdx

		// Style for selected vs unselected
		nameStyle := lipgloss.NewStyle().Foreground(colorWhite)
		helpStyle := lipgloss.NewStyle().Foreground(colorGray)
		cursor := "  "
		if isSelected {
			nameStyle = nameStyle.Foreground(colorBlue).Bold(true)
			helpStyle = helpStyle.Foreground(colorBlue).Faint(true)
			cursor = "> "
		}

		// Build suggestion: name on first line, help text aligned with name
		nameLine := cursor + nameStyle.Render(entry.Name)
		suggestions = append(suggestions, nameLine)

		if entry.Command.Help != "" {
			// Truncate help text to single line
			helpIndent := "  " // 2 spaces to align with command name
			// Account for modal padding, indent, and extra safety margin
			// contentWidth - indent - padding - safety = 90 - 2 - 4 - 1 = 83
			maxHelpWidth := contentWidth - len(helpIndent) - 7
			helpText := entry.Command.Help

			// Truncate with "..." if too long (using runes for proper text measurement)
			// Reserve 3 chars for "..." in the max width calculation
			runes := []rune(helpText)
			if len(runes) > maxHelpWidth {
				helpText = string(runes[:maxHelpWidth-3]) + "..."
			}

			suggestions = append(suggestions, helpIndent+helpStyle.Render(helpText))
		}
	}

	// Show count if more suggestions available beyond visible window
	remaining := len(p.filteredList) - endIdx
	if remaining > 0 {
		moreStyle := lipgloss.NewStyle().Foreground(colorGray).Italic(true)
		suggestions = append(suggestions, moreStyle.Render("  ... and more"))
	}

	// Join all parts with constrained width
	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		inputView,
		"",
		strings.Join(suggestions, "\n"),
	)

	help := modalHelpStyle.Render("↑/k up  ↓/j down  tab fill  enter select  esc cancel")
	content = lipgloss.JoinVertical(lipgloss.Left, content, help)

	return modalStyle.Width(contentWidth).Render(content)
}

// Overlay renders the command palette as a layer over the given background.
func (p *CommandPalette) Overlay(background string, width, height int) string {
	modal := p.View()

	bgLayer := lipgloss.NewLayer(background)
	modalLayer := lipgloss.NewLayer(modal)

	// Position modal: horizontally centered, anchored near top
	modalW := lipgloss.Width(modal)
	centerX := (width - modalW) / 2
	topY := 3 // Anchor near top, below banner
	modalLayer.X(centerX).Y(topY).Z(1)

	compositor := lipgloss.NewCompositor(bgLayer, modalLayer)
	return compositor.Render()
}

// SelectedCommand returns the selected command entry and parsed args, if any.
func (p *CommandPalette) SelectedCommand() (*CommandEntry, []string, bool) {
	if !p.selected || len(p.filteredList) == 0 {
		return nil, nil, false
	}

	if p.selectedIdx >= len(p.filteredList) {
		return nil, nil, false
	}

	entry := p.filteredList[p.selectedIdx]

	// Parse input to extract args
	parsed := ParseCommandInput(p.input.Value())

	return &entry, parsed.Args, true
}

// Cancelled returns true if the user cancelled the palette.
func (p *CommandPalette) Cancelled() bool {
	return p.cancelled
}

// IsFiltering returns whether the palette is currently filtering.
// Always true for the new text input-based palette.
func (p *CommandPalette) IsFiltering() bool {
	return true
}

// KeyMap returns keys that the command palette uses (for help integration).
func (p *CommandPalette) KeyMap() []key.Binding {
	return []key.Binding{
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
