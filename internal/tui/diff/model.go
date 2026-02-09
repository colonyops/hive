package diff

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/bluekeyes/go-gitdiff/gitdiff"
	"github.com/hay-kot/hive/internal/core/config"
	"github.com/hay-kot/hive/internal/core/styles"
	"github.com/hay-kot/hive/internal/tui/components"
)

// FocusedPanel represents which panel has keyboard focus.
type FocusedPanel int

const (
	FocusFileTree FocusedPanel = iota
	FocusDiffViewer
)

// Model is the main diff viewer model that composes the file tree and diff viewer.
type Model struct {
	fileTree      FileTreeModel
	diffViewer    DiffViewerModel
	focused       FocusedPanel
	width         int
	height        int
	helpDialog    *components.HelpDialog
	showHelp      bool
	reviewContext string // Review title/context to display in header
}

// New creates a new diff viewer model from git diff files.
func New(files []*gitdiff.File, cfg *config.Config) Model {
	return NewWithContext(files, cfg, "Diff Review")
}

// NewWithContext creates a new diff viewer model with a custom review context.
func NewWithContext(files []*gitdiff.File, cfg *config.Config, reviewContext string) Model {
	// Create file tree
	fileTree := NewFileTree(files, cfg)

	// Create diff viewer with first file (if any)
	var diffViewer DiffViewerModel
	if len(files) > 0 {
		diffViewer = NewDiffViewer(files[0])
	} else {
		diffViewer = NewDiffViewer(nil)
	}

	// Create help dialog
	helpDialog := createHelpDialog()

	return Model{
		fileTree:      fileTree,
		diffViewer:    diffViewer,
		focused:       FocusFileTree, // Start with file tree focused
		helpDialog:    helpDialog,
		showHelp:      false,
		reviewContext: reviewContext,
	}
}

// createHelpDialog creates the help dialog with all keyboard shortcuts.
func createHelpDialog() *components.HelpDialog {
	sections := []components.HelpDialogSection{
		{
			Title: "Navigation",
			Entries: []components.HelpEntry{
				{Key: "tab", Desc: "Switch between file tree and diff viewer"},
				{Key: "↑/k", Desc: "Move up"},
				{Key: "↓/j", Desc: "Move down"},
				{Key: "g", Desc: "Jump to top"},
				{Key: "G", Desc: "Jump to bottom"},
				{Key: "ctrl+d", Desc: "Scroll down half page"},
				{Key: "ctrl+u", Desc: "Scroll up half page"},
			},
		},
		{
			Title: "File Tree",
			Entries: []components.HelpEntry{
				{Key: "enter", Desc: "Expand/collapse directory or select file"},
				{Key: "←/h", Desc: "Jump to parent directory"},
			},
		},
		{
			Title: "Visual Selection",
			Entries: []components.HelpEntry{
				{Key: "v", Desc: "Enter/exit visual mode"},
				{Key: "↑/↓", Desc: "Extend selection (in visual mode)"},
				{Key: "esc", Desc: "Cancel selection"},
			},
		},
		{
			Title: "General",
			Entries: []components.HelpEntry{
				{Key: "?", Desc: "Toggle this help"},
				{Key: "q/ctrl+c", Desc: "Quit"},
			},
		},
	}

	return components.NewHelpDialog("Diff Viewer Help", sections, 80, 40)
}

// Init initializes the model (no commands needed).
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles keyboard input and updates the focused panel.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Handle window resize
		m.SetSize(msg.Width, msg.Height)
		return m, nil
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		// Handle help toggle (works from any state)
		if keyMsg.String() == "?" {
			m.showHelp = !m.showHelp
			return m, nil
		}

		// If help is shown, only handle escape to close it
		if m.showHelp {
			if keyMsg.String() == "escape" || keyMsg.String() == "?" {
				m.showHelp = false
			}
			return m, nil
		}

		switch keyMsg.String() {
		case "q", "ctrl+c":
			// Quit the application
			return m, tea.Quit

		case "tab":
			// Switch focus between panels
			if m.focused == FocusFileTree {
				m.focused = FocusDiffViewer
			} else {
				m.focused = FocusFileTree
			}
			return m, nil

		case "enter":
			// If file tree is focused and a file is selected, sync to diff viewer
			if m.focused == FocusFileTree {
				selectedFile := m.fileTree.SelectedFile()
				if selectedFile != nil {
					m.diffViewer.SetFile(selectedFile)
				}
			}
			// Fall through to let file tree handle expand/collapse
		}
	}

	// Route input to focused panel
	var cmd tea.Cmd
	switch m.focused {
	case FocusFileTree:
		m.fileTree, cmd = m.fileTree.Update(msg)

		// Auto-sync diff viewer when selection changes in file tree
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch keyMsg.String() {
			case "j", "k", "up", "down", "g", "G":
				selectedFile := m.fileTree.SelectedFile()
				if selectedFile != nil {
					m.diffViewer.SetFile(selectedFile)
				}
			}
		}

	case FocusDiffViewer:
		m.diffViewer, cmd = m.diffViewer.Update(msg)
	}

	return m, cmd
}

// View renders the two-panel layout with file tree on left and diff viewer on right.
func (m Model) View() tea.View {
	if m.width == 0 || m.height == 0 {
		return tea.NewView("")
	}

	// Render unified header (3 lines: top border, content, bottom border)
	unifiedHeader := m.renderUnifiedHeader()
	headerHeight := 3

	// Render panel sub-header (1 line)
	subHeader := m.renderPanelSubHeader()
	subHeaderHeight := 1

	// Calculate panel dimensions
	// File tree takes 30% of width, gutter is 1 char, diff viewer takes rest
	treeWidth := m.width * 30 / 100
	gutterWidth := 1
	diffWidth := m.width - treeWidth - gutterWidth

	// Panel height = total - headers - status bar
	panelHeight := m.height - headerHeight - subHeaderHeight - 1

	// Sync selection info from diff viewer to file tree
	start, end, active := m.diffViewer.GetSelectionInfo()
	if active {
		m.fileTree.SetSelection(start, end)
	} else {
		m.fileTree.SetSelection(-1, -1)
	}

	// Render file tree
	fileTreeView := m.fileTree.View()

	// Render diff viewer
	diffViewerView := m.diffViewer.View()

	// Create thin gutter divider between panels
	// Build gutter content as multiple lines
	var gutterBuilder strings.Builder
	for i := 0; i < panelHeight; i++ {
		if i > 0 {
			gutterBuilder.WriteString("\n")
		}
		gutterBuilder.WriteString("│")
	}
	gutter := lipgloss.NewStyle().
		Foreground(styles.ColorMuted).
		Render(gutterBuilder.String())

	// Apply consistent styling
	treeStyle := lipgloss.NewStyle().
		Width(treeWidth).
		Height(panelHeight).
		PaddingLeft(1) // Indent tree to align with header

	diffStyle := lipgloss.NewStyle().
		Width(diffWidth).
		Height(panelHeight).
		PaddingLeft(1) // Add padding for alignment

	// Render panels
	leftPanel := treeStyle.Render(fileTreeView)
	rightPanel := diffStyle.Render(diffViewerView)

	// Join panels horizontally with gutter divider
	content := lipgloss.JoinHorizontal(lipgloss.Top,
		leftPanel,
		gutter,
		rightPanel,
	)

	// Render status bar
	statusBar := m.renderStatusBar()

	// Join all components vertically
	result := lipgloss.JoinVertical(lipgloss.Left,
		unifiedHeader,
		subHeader,
		content,
		statusBar,
	)

	// Render help overlay if shown
	if m.showHelp {
		result = m.helpDialog.Overlay(result, m.width, m.height)
	}

	// Create and return tea.View
	v := tea.NewView(result)
	v.AltScreen = true // Use alternate screen buffer
	return v
}

// renderUnifiedHeader renders the top header with review context and branding.
func (m Model) renderUnifiedHeader() string {
	// Top border
	topBorder := strings.Repeat("─", m.width)

	// Content line: review context on left, branded "Hive" on right, with 1 space padding
	contextLeft := styles.TextMutedStyle.Render(m.reviewContext)
	brandingRight := styles.TabBrandingStyle.Render(styles.IconHive + " Hive")

	leftWidth := lipgloss.Width(contextLeft)
	rightWidth := lipgloss.Width(brandingRight)
	// Account for 1 space padding on left and right
	spacing := m.width - leftWidth - rightWidth - 2

	if spacing < 1 {
		spacing = 1
	}

	contentLine := " " + contextLeft + strings.Repeat(" ", spacing) + brandingRight + " "

	// Bottom border
	bottomBorder := strings.Repeat("─", m.width)

	return lipgloss.JoinVertical(lipgloss.Left,
		topBorder,
		contentLine,
		bottomBorder,
	)
}

// renderPanelSubHeader renders the panel title bar with focus indicators.
func (m Model) renderPanelSubHeader() string {
	// Calculate widths for layout (matching panel widths)
	treeWidth := m.width * 30 / 100
	gutterWidth := 1
	diffWidth := m.width - treeWidth - gutterWidth

	// Render left panel title based on focus
	var filesTitle, diffTitle string
	if m.focused == FocusFileTree {
		filesTitle = styles.TextPrimaryBoldStyle.Render("Files Changed")
		diffTitle = styles.TextMutedStyle.Render("Diff View")
	} else {
		filesTitle = styles.TextMutedStyle.Render("Files Changed")
		diffTitle = styles.TextPrimaryBoldStyle.Render("Diff View")
	}

	// Left side (left-aligned with padding)
	leftSide := " " + filesTitle
	leftSide = lipgloss.NewStyle().Width(treeWidth).Render(leftSide)

	// Gutter divider
	gutter := styles.TextMutedStyle.Render("│")

	// Right side (left-aligned with padding)
	rightSide := " " + diffTitle
	rightSide = lipgloss.NewStyle().Width(diffWidth).Render(rightSide)

	return leftSide + gutter + rightSide
}

// renderStatusBar renders the status bar at the bottom.
func (m Model) renderStatusBar() string {
	// Left section: panel name with mode indicator and context
	var leftSection, rightSection string

	switch m.focused {
	case FocusFileTree:
		// File tree: show file count
		fileCount := m.fileTree.FileCount()
		panelName := styles.TextPrimaryBoldStyle.Render("Files")
		count := styles.TextMutedStyle.Render(fmt.Sprintf("(%d)", fileCount))
		leftSection = panelName + " " + count

		// Help text
		rightSection = styles.TextMutedStyle.Render("↑↓:nav  enter:select  tab:switch  ?:help  q:quit")

	case FocusDiffViewer:
		// Diff viewer: show mode indicator and line position
		var modeIndicator string
		if m.diffViewer.SelectionMode() {
			modeIndicator = styles.ReviewModeVisualStyle.Render(" VISUAL ")
		} else {
			modeIndicator = styles.ReviewModeNormalStyle.Render(" NORMAL ")
		}

		// Position info (line X/Y)
		currentLine := m.diffViewer.CurrentLine()
		totalLines := m.diffViewer.TotalLines()
		position := styles.TextMutedStyle.Render(fmt.Sprintf(" %d/%d", currentLine, totalLines))

		leftSection = modeIndicator + position

		// Help text
		if m.diffViewer.SelectionMode() {
			rightSection = styles.TextMutedStyle.Render("↑↓:move  v:exit  esc:cancel  ?:help")
		} else {
			rightSection = styles.TextMutedStyle.Render("↑↓:scroll  v:visual  ctrl+d/u:page  g/G:jump  ?:help  q:quit")
		}
	}

	// Calculate spacing for layout: [left] ... [right]
	leftWidth := lipgloss.Width(leftSection)
	rightWidth := lipgloss.Width(rightSection)
	spacing := m.width - leftWidth - rightWidth

	if spacing < 1 {
		spacing = 1
	}

	statusBar := leftSection + strings.Repeat(" ", spacing) + rightSection

	// Status bar with distinct background (like vim's status bar)
	return lipgloss.NewStyle().
		Width(m.width).
		Background(styles.ColorPrimary).
		Foreground(styles.ColorBackground).
		Bold(true).
		Padding(0, 1).
		Render(statusBar)
}

// SetSize updates the dimensions and propagates to child components.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height

	// Calculate panel dimensions (same logic as View)
	headerHeight := 3    // unified header
	subHeaderHeight := 1 // panel titles
	statusBarHeight := 1 // status bar

	treeWidth := width * 30 / 100
	gutterWidth := 1
	diffWidth := width - treeWidth - gutterWidth
	panelHeight := height - headerHeight - subHeaderHeight - statusBarHeight

	// Update child components
	m.fileTree.SetSize(treeWidth, panelHeight)
	m.diffViewer.SetSize(diffWidth, panelHeight)
}
