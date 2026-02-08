package diff

import (
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
	fileTree   FileTreeModel
	diffViewer DiffViewerModel
	focused    FocusedPanel
	width      int
	height     int
	helpDialog *components.HelpDialog
	showHelp   bool
}

// New creates a new diff viewer model from git diff files.
func New(files []*gitdiff.File, cfg *config.Config) Model {
	// Create file tree
	fileTree := NewFileTree(files, cfg)

	// Create diff viewer with first file (if any)
	var diffViewer DiffViewerModel
	if len(files) > 0 {
		diffViewer = NewDiffViewer(files[0])
	} else {
		diffViewer = NewDiffViewer(nil)
	}

	return Model{
		fileTree:   fileTree,
		diffViewer: diffViewer,
		focused:    FocusFileTree, // Start with file tree focused
	}
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

	// Calculate panel dimensions
	// File tree takes 30% of width, diff viewer takes 70%
	treeWidth := m.width * 30 / 100
	diffWidth := m.width - treeWidth - 1 // -1 for separator

	// Both panels use full height minus 1 for status bar
	panelHeight := m.height - 1

	// Render file tree
	fileTreeView := m.fileTree.View()

	// Render diff viewer
	diffViewerView := m.diffViewer.View()

	// Create separator
	separator := lipgloss.NewStyle().
		Width(1).
		Height(panelHeight).
		Render(styles.TextMutedStyle.Render("│"))

	// Apply focus styling
	var treeStyle, diffStyle lipgloss.Style
	if m.focused == FocusFileTree {
		treeStyle = lipgloss.NewStyle().
			Width(treeWidth).
			Height(panelHeight).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(styles.ColorPrimary)
		diffStyle = lipgloss.NewStyle().
			Width(diffWidth).
			Height(panelHeight)
	} else {
		treeStyle = lipgloss.NewStyle().
			Width(treeWidth).
			Height(panelHeight)
		diffStyle = lipgloss.NewStyle().
			Width(diffWidth).
			Height(panelHeight).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(styles.ColorPrimary)
	}

	// Render panels
	leftPanel := treeStyle.Render(fileTreeView)
	rightPanel := diffStyle.Render(diffViewerView)

	// Join panels horizontally
	content := lipgloss.JoinHorizontal(lipgloss.Top,
		leftPanel,
		separator,
		rightPanel,
	)

	// Render status bar
	statusBar := m.renderStatusBar()

	// Join content and status bar vertically
	result := lipgloss.JoinVertical(lipgloss.Left,
		content,
		statusBar,
	)

	// Create and return tea.View
	v := tea.NewView(result)
	v.AltScreen = true // Use alternate screen buffer
	return v
}

// renderStatusBar renders the status bar at the bottom.
func (m Model) renderStatusBar() string {
	// Show current panel and help text
	var leftSection, help string

	switch m.focused {
	case FocusFileTree:
		leftSection = styles.TextPrimaryBoldStyle.Render("File Tree")
		help = "↑/↓ navigate • enter expand/select • tab switch panel • q quit"
	case FocusDiffViewer:
		// Show mode indicator for diff viewer
		var modeIndicator string
		if m.diffViewer.SelectionMode() {
			modeIndicator = styles.ReviewModeVisualStyle.Render("VISUAL")
		} else {
			modeIndicator = styles.ReviewModeNormalStyle.Render("NORMAL")
		}
		leftSection = modeIndicator + " " + styles.TextPrimaryBoldStyle.Render("Diff Viewer")

		if m.diffViewer.SelectionMode() {
			help = "↑/↓ move selection • v exit visual • esc cancel • tab switch panel"
		} else {
			help = "↑/↓ scroll • v visual mode • ctrl+d/u page • g/G top/bottom • tab switch • q quit"
		}
	}

	rightSection := styles.TextMutedStyle.Render(help)

	// Calculate spacing
	leftWidth := lipgloss.Width(leftSection)
	rightWidth := lipgloss.Width(rightSection)
	spacing := m.width - leftWidth - rightWidth - 2 // -2 for padding

	if spacing < 0 {
		spacing = 0
	}

	statusBar := leftSection + strings.Repeat(" ", spacing) + rightSection

	return lipgloss.NewStyle().
		Width(m.width).
		Background(styles.ColorSurface).
		Render(statusBar)
}

// SetSize updates the dimensions and propagates to child components.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height

	// Calculate panel dimensions (same logic as View)
	treeWidth := width * 30 / 100
	diffWidth := width - treeWidth - 1
	panelHeight := height - 1

	// Update child components
	m.fileTree.SetSize(treeWidth, panelHeight)
	m.diffViewer.SetSize(diffWidth, panelHeight)
}
