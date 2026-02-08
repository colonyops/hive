package diff

import (
	"fmt"
	"image/color"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/bluekeyes/go-gitdiff/gitdiff"
	"github.com/hay-kot/hive/internal/core/config"
	"github.com/hay-kot/hive/internal/core/styles"
)

// IconStyle determines which icon set to use for file tree rendering.
type IconStyle string

const (
	IconStyleNerdFonts IconStyle = "nerd-fonts"
	IconStyleUnicode   IconStyle = "unicode"
	IconStyleASCII     IconStyle = "ascii"
)

// TreeNode represents a node in the file tree (either a directory or file).
type TreeNode struct {
	Name     string        // Directory or file name
	Path     string        // Full path
	IsDir    bool          // True if this is a directory
	File     *gitdiff.File // Associated git file (nil for directories)
	Children []*TreeNode   // Child nodes (for directories)
	Expanded bool          // Whether this directory is expanded
	Depth    int           // Depth in tree (0 = root level)
}

// FileTreeModel displays a hierarchical list of changed files.
// This is a small, focused component that can be tested independently.
type FileTreeModel struct {
	files          []*gitdiff.File // Original flat file list
	root           *TreeNode       // Root of the tree
	visible        []*TreeNode     // Currently visible nodes (flattened view)
	selected       int             // Index in visible list
	offset         int             // Scroll offset (first visible item index)
	width          int
	height         int
	iconStyle      IconStyle
	hierarchical   bool // Display mode: true = tree, false = flat
	selectionStart int  // Start line of selection in diff viewer (0-indexed, -1 if no selection)
	selectionEnd   int  // End line of selection in diff viewer (0-indexed, -1 if no selection)
}

// NewFileTree creates a new file tree from diff files.
func NewFileTree(files []*gitdiff.File, cfg *config.Config) FileTreeModel {
	iconStyle := IconStyleUnicode // Default to unicode
	if cfg.TUI.IconsEnabled() {
		iconStyle = IconStyleNerdFonts
	}

	m := FileTreeModel{
		files:          files,
		selected:       0,
		offset:         0,
		iconStyle:      iconStyle,
		hierarchical:   true, // Default to tree view
		selectionStart: -1,   // No selection initially
		selectionEnd:   -1,
	}

	// Build tree structure
	m.root = buildTree(files)
	m.rebuildVisible()

	return m
}

// buildTree constructs a hierarchical tree from a flat list of files.
func buildTree(files []*gitdiff.File) *TreeNode {
	root := &TreeNode{
		Name:     "",
		Path:     "",
		IsDir:    true,
		Expanded: true,
		Depth:    -1, // Root is at depth -1
	}

	for _, file := range files {
		// Get file path (prefer new name, fall back to old name for deletions)
		path := file.NewName
		if path == "" || path == "/dev/null" {
			path = file.OldName
		}

		// Skip empty paths
		if path == "" {
			continue
		}

		// Split path into components
		parts := strings.Split(path, "/")
		current := root

		// Traverse/create directory nodes
		for i := 0; i < len(parts)-1; i++ {
			dirName := parts[i]
			found := false

			// Look for existing directory node
			for _, child := range current.Children {
				if child.IsDir && child.Name == dirName {
					current = child
					found = true
					break
				}
			}

			// Create new directory node if not found
			if !found {
				newDir := &TreeNode{
					Name:     dirName,
					Path:     strings.Join(parts[:i+1], "/"),
					IsDir:    true,
					Expanded: true, // Expand all directories by default
					Depth:    i,
				}
				current.Children = append(current.Children, newDir)
				current = newDir
			}
		}

		// Add file node
		fileName := parts[len(parts)-1]
		fileNode := &TreeNode{
			Name:  fileName,
			Path:  path,
			IsDir: false,
			File:  file,
			Depth: len(parts) - 1,
		}
		current.Children = append(current.Children, fileNode)
	}

	return root
}

// rebuildVisible rebuilds the visible node list based on expand/collapse state.
func (m *FileTreeModel) rebuildVisible() {
	m.visible = nil
	if m.hierarchical {
		m.collectVisible(m.root)
	} else {
		// Flat mode: show all files
		for _, file := range m.files {
			path := file.NewName
			if path == "" || path == "/dev/null" {
				path = file.OldName
			}
			node := &TreeNode{
				Name:  path,
				Path:  path,
				IsDir: false,
				File:  file,
				Depth: 0,
			}
			m.visible = append(m.visible, node)
		}
	}

	// Ensure selection is valid
	if m.selected >= len(m.visible) {
		m.selected = max(0, len(m.visible)-1)
	}
}

// collectVisible recursively collects visible nodes (considering expand state).
func (m *FileTreeModel) collectVisible(node *TreeNode) {
	// Don't add root node itself
	if node.Depth >= 0 {
		m.visible = append(m.visible, node)
	}

	// Add children if this is an expanded directory
	if node.IsDir && node.Expanded {
		for _, child := range node.Children {
			m.collectVisible(child)
		}
	}
}

// Update handles key messages for file tree navigation.
func (m FileTreeModel) Update(msg tea.Msg) (FileTreeModel, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "j", "down":
			if m.selected < len(m.visible)-1 {
				m.selected++
				// Adjust viewport to keep selection visible
				if m.height > 0 && m.selected >= m.offset+m.height {
					m.offset = m.selected - m.height + 1
				}
			}
		case "k", "up":
			if m.selected > 0 {
				m.selected--
				// Adjust viewport to keep selection visible
				if m.height > 0 && m.selected < m.offset {
					m.offset = m.selected
				}
			}
		case "g":
			// Jump to top
			m.selected = 0
			m.offset = 0
		case "G":
			// Jump to bottom
			if len(m.visible) > 0 {
				m.selected = len(m.visible) - 1
				// Adjust viewport to show last item
				if m.height > 0 && m.selected >= m.offset+m.height {
					m.offset = m.selected - m.height + 1
				}
			}
		case "enter", "right", " ":
			// Expand/collapse directory or select file
			if m.selected < len(m.visible) {
				node := m.visible[m.selected]
				if node.IsDir {
					node.Expanded = !node.Expanded
					m.rebuildVisible()
				}
			}
		case "left":
			// Collapse current directory or jump to parent
			if m.selected < len(m.visible) {
				node := m.visible[m.selected]
				if node.IsDir && node.Expanded {
					// Collapse if expanded
					node.Expanded = false
					m.rebuildVisible()
				} else if node.Depth > 0 {
					// Jump to parent directory
					m.jumpToParent()
				}
			}
		}
	}
	return m, nil
}

// jumpToParent moves selection to the parent directory of the current node.
func (m *FileTreeModel) jumpToParent() {
	if m.selected >= len(m.visible) {
		return
	}

	currentNode := m.visible[m.selected]
	targetDepth := currentNode.Depth - 1

	// Search backwards for parent
	for i := m.selected - 1; i >= 0; i-- {
		if m.visible[i].IsDir && m.visible[i].Depth == targetDepth {
			m.selected = i
			return
		}
	}
}

// View renders the file tree without header (header is now in unified layout).
func (m FileTreeModel) View() string {
	if len(m.files) == 0 {
		return m.renderEmptyState()
	}

	// Calculate visible range
	start := m.offset
	end := m.offset + m.height
	if end > len(m.visible) {
		end = len(m.visible)
	}

	var lines []string
	for i := start; i < end; i++ {
		node := m.visible[i]
		line := m.renderNode(node, i == m.selected)
		lines = append(lines, line)
	}

	content := strings.Join(lines, "\n")
	return lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Render(content)
}

// renderHeader renders the file tree panel header with branding.
// renderEmptyState renders a centered message when there are no files.
func (m FileTreeModel) renderEmptyState() string {
	title := styles.TextForegroundBoldStyle.Render("No Files Changed")
	hint := styles.TextMutedStyle.Render("No differences found in the working tree")

	content := lipgloss.JoinVertical(lipgloss.Center,
		"",
		title,
		hint,
		"",
	)

	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		content)
}

// renderNode renders a tree node (directory or file) with indentation.
func (m FileTreeModel) renderNode(node *TreeNode, selected bool) string {
	// Build indentation
	indent := strings.Repeat("  ", node.Depth)
	indentWidth := len(indent)

	var icon, name string
	var fileColor lipgloss.Style

	if node.IsDir {
		// Directory node
		icon = m.getDirIcon(node.Expanded)
		name = node.Name + "/"
		fileColor = styles.TextForegroundStyle
	} else {
		// File node
		icon = m.getFileIcon(node.Path)
		name = node.Name
		fileColor = m.getFileStatusColor(node.File)
	}

	// Calculate stats for files
	var additions, deletions int
	if !node.IsDir && node.File != nil {
		for _, frag := range node.File.TextFragments {
			for _, line := range frag.Lines {
				switch line.Op {
				case gitdiff.OpAdd:
					additions++
				case gitdiff.OpDelete:
					deletions++
				}
			}
		}
	}

	// Build stats string (compact format without colors for width calculation)
	var statsPlain string
	if additions > 0 || deletions > 0 {
		if additions > 0 && deletions > 0 {
			statsPlain = fmt.Sprintf("+%d -%d", additions, deletions)
		} else if additions > 0 {
			statsPlain = fmt.Sprintf("+%d", additions)
		} else {
			statsPlain = fmt.Sprintf("-%d", deletions)
		}
	}

	// Calculate available width for filename
	// Available = panelWidth - indent - icon - space - stats - padding
	iconWidth := lipgloss.Width(icon) + 1 // icon + space
	statsWidth := 0
	if statsPlain != "" {
		statsWidth = len(statsPlain)
	}
	availableWidth := m.width - indentWidth - iconWidth - statsWidth - 2 // 2 for padding

	// Truncate filename if needed
	if availableWidth < len(name) && availableWidth > 3 {
		name = name[:availableWidth-2] + ".."
	}

	// Render the line
	if selected {
		// Selected: apply background to all components for full-width highlighting
		bgStyle := lipgloss.NewStyle().Background(styles.ColorSurface)

		// Style all parts with background
		indentStyled := bgStyle.Render(indent)
		iconStyled := fileColor.Background(styles.ColorSurface).Render(icon)
		spaceStyled := bgStyle.Render(" ")
		nameStyled := fileColor.Bold(true).Background(styles.ColorSurface).Render(name)

		// Add line range indicator if selection is active
		var lineIndicator string
		if m.selectionStart >= 0 && m.selectionEnd >= 0 {
			// Display 1-indexed line numbers for user clarity
			start := m.selectionStart + 1
			end := m.selectionEnd + 1
			if start == end {
				lineIndicator = fmt.Sprintf(" [L%d]", start)
			} else {
				lineIndicator = fmt.Sprintf(" [L%d-%d]", start, end)
			}
			lineIndicator = styles.TextMutedStyle.Background(styles.ColorSurface).Render(lineIndicator)
		}

		var line string
		if statsPlain != "" {
			statsStyled := m.renderDiffStatsInlineWithBg(additions, deletions, styles.ColorSurface)
			line = indentStyled + iconStyled + spaceStyled + nameStyled + spaceStyled + statsStyled + lineIndicator
		} else {
			line = indentStyled + iconStyled + spaceStyled + nameStyled + lineIndicator
		}

		// Apply background to full line width including padding
		return bgStyle.Width(m.width).Render(line)
	}

	// Normal (unselected) style
	iconStyled := fileColor.Render(icon)
	nameStyled := styles.TextForegroundStyle.Render(name)
	if statsPlain != "" {
		statsStyled := m.renderDiffStatsInline(additions, deletions)
		return fmt.Sprintf("%s%s %s %s", indent, iconStyled, nameStyled, statsStyled)
	}
	return fmt.Sprintf("%s%s %s", indent, iconStyled, nameStyled)
}

// getFileStatusColor returns the color for a file based on its git status.
func (m FileTreeModel) getFileStatusColor(file *gitdiff.File) lipgloss.Style {
	if file == nil {
		return styles.TextForegroundStyle
	}

	if file.IsNew {
		return styles.TextSuccessStyle // Green for new files
	} else if file.IsDelete {
		return styles.TextErrorStyle // Red for deleted files
	}
	return styles.TextWarningStyle // Yellow for modified files
}

// renderDiffStatsInline renders compact inline diff statistics with color.
func (m FileTreeModel) renderDiffStatsInline(additions, deletions int) string {
	return m.renderDiffStatsInlineWithBg(additions, deletions, nil)
}

// renderDiffStatsInlineWithBg renders compact inline diff statistics with color and optional background.
func (m FileTreeModel) renderDiffStatsInlineWithBg(additions, deletions int, bg color.Color) string {
	if additions == 0 && deletions == 0 {
		return ""
	}

	var parts []string

	if additions > 0 {
		addStr := fmt.Sprintf("+%d", additions)
		style := styles.TextSuccessStyle
		if bg != nil {
			style = style.Background(bg)
		}
		parts = append(parts, style.Render(addStr))
	}

	if deletions > 0 {
		delStr := fmt.Sprintf("-%d", deletions)
		style := styles.TextErrorStyle
		if bg != nil {
			style = style.Background(bg)
		}
		parts = append(parts, style.Render(delStr))
	}

	return strings.Join(parts, " ")
}

// getDirIcon returns the icon for a directory (expanded or collapsed).
func (m FileTreeModel) getDirIcon(expanded bool) string {
	switch m.iconStyle {
	case IconStyleNerdFonts:
		if expanded {
			return styles.IconFolderOpen
		}
		return styles.IconFolderClosed
	case IconStyleUnicode:
		if expanded {
			return "📂"
		}
		return "📁"
	case IconStyleASCII:
		if expanded {
			return "▼"
		}
		return "▶"
	default:
		if expanded {
			return "▾"
		}
		return "▸"
	}
}

// getFileIcon returns the appropriate icon for a file path.
func (m FileTreeModel) getFileIcon(path string) string {
	switch m.iconStyle {
	case IconStyleNerdFonts:
		return getFileIconNerdFont(path)
	case IconStyleUnicode:
		return getFileIconUnicode(path)
	case IconStyleASCII:
		return "*"
	default:
		return "•"
	}
}

// getFileIconNerdFont returns nerd font icon for file extension.
func getFileIconNerdFont(path string) string {
	ext := filepath.Ext(path)
	switch ext {
	case ".go":
		return styles.IconFileGo
	case ".js":
		return styles.IconFileJS
	case ".ts":
		return styles.IconFileTS
	case ".tsx":
		return styles.IconFileTSX
	case ".jsx":
		return styles.IconFileJSX
	case ".py":
		return styles.IconFilePython
	case ".md":
		return styles.IconFileMarkdown
	case ".json":
		return styles.IconFileJSON
	case ".yaml", ".yml":
		return styles.IconFileYAML
	case ".toml":
		return styles.IconFileTOML
	case ".xml":
		return styles.IconFileXML
	case ".html":
		return styles.IconFileHTML
	case ".css":
		return styles.IconFileCSS
	case ".rs":
		return styles.IconFileRust
	case ".c", ".h":
		return styles.IconFileC
	case ".cpp", ".cc", ".cxx", ".hpp":
		return styles.IconFileCPP
	case ".java":
		return styles.IconFileJava
	case ".rb":
		return styles.IconFileRuby
	case ".php":
		return styles.IconFilePHP
	case ".sh":
		return styles.IconFileShell
	case ".sql":
		return styles.IconFileSQL
	case ".vim":
		return styles.IconFileVim
	case ".lua":
		return styles.IconFileLua
	default:
		// Check for common filenames
		base := filepath.Base(path)
		switch base {
		case "Dockerfile":
			return styles.IconFileDocker
		case "Makefile":
			return styles.IconFileMakefile
		case "README.md", "README":
			return styles.IconFileReadme
		default:
			return styles.IconFileDefault
		}
	}
}

// getFileIconUnicode returns unicode icon for file type.
func getFileIconUnicode(path string) string {
	ext := filepath.Ext(path)
	switch ext {
	case ".md":
		return "📝"
	case ".json", ".yaml", ".yml", ".toml":
		return "⚙️"
	case ".go", ".js", ".ts", ".py", ".rs", ".c", ".cpp", ".java":
		return "💻"
	case ".html", ".css":
		return "🌐"
	default:
		return "📄"
	}
}

// SelectedFile returns the currently selected file, or nil if selection is a directory or invalid.
func (m FileTreeModel) SelectedFile() *gitdiff.File {
	if m.selected < 0 || m.selected >= len(m.visible) {
		return nil
	}
	node := m.visible[m.selected]
	if node.IsDir {
		return nil // Directories don't have an associated file
	}
	return node.File
}

// SelectedIndex returns the currently selected index.
func (m FileTreeModel) SelectedIndex() int {
	return m.selected
}

// SetSize updates the dimensions of the file tree.
func (m *FileTreeModel) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// SetSelection updates the selection range from the diff viewer.
// Pass -1, -1 to clear the selection.
func (m *FileTreeModel) SetSelection(start, end int) {
	m.selectionStart = start
	m.selectionEnd = end
}

// FileCount returns the total number of files in the tree.
func (m FileTreeModel) FileCount() int {
	return len(m.files)
}

// SetFiles updates the files list and rebuilds the tree.
func (m *FileTreeModel) SetFiles(files []*gitdiff.File) {
	m.files = files
	m.root = buildTree(files)
	m.rebuildVisible()
	// Reset selection if out of bounds
	if m.selected >= len(m.visible) {
		m.selected = max(0, len(m.visible)-1)
	}
}
