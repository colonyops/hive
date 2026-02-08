package diff

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bluekeyes/go-gitdiff/gitdiff"
	"github.com/charmbracelet/x/exp/golden"
	"github.com/hay-kot/hive/internal/core/config"
	"github.com/hay-kot/hive/internal/core/styles"
	"github.com/hay-kot/hive/pkg/tuitest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFileTree(t *testing.T) {
	files := []*gitdiff.File{
		{NewName: "file1.go"},
		{NewName: "file2.go"},
	}

	cfg := &config.Config{
		TUI: config.TUIConfig{},
	}

	m := NewFileTree(files, cfg)

	assert.Len(t, m.files, 2)
	assert.Equal(t, 0, m.selected)
	// Default is nerd-fonts when Icons is nil (IconsEnabled() returns true)
	assert.Equal(t, IconStyleNerdFonts, m.iconStyle)
}

func TestNewFileTreeWithNerdFontsEnabled(t *testing.T) {
	files := []*gitdiff.File{
		{NewName: "file1.go"},
	}

	iconsEnabled := true
	cfg := &config.Config{
		TUI: config.TUIConfig{
			Icons: &iconsEnabled,
		},
	}

	m := NewFileTree(files, cfg)

	assert.Equal(t, IconStyleNerdFonts, m.iconStyle)
}

func TestFileTreeNavigationDown(t *testing.T) {
	files := []*gitdiff.File{
		{NewName: "file1.go"},
		{NewName: "file2.go"},
		{NewName: "file3.go"},
	}

	cfg := &config.Config{}
	m := NewFileTree(files, cfg)

	// Test initial state
	assert.Equal(t, 0, m.selected)

	// Test down navigation with 'j'
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'j'}))
	assert.Equal(t, 1, m.selected)

	// Test down navigation with arrow key
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	assert.Equal(t, 2, m.selected)

	// Test boundary (can't go past last file)
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'j'}))
	assert.Equal(t, 2, m.selected)
}

func TestFileTreeNavigationUp(t *testing.T) {
	files := []*gitdiff.File{
		{NewName: "file1.go"},
		{NewName: "file2.go"},
		{NewName: "file3.go"},
	}

	cfg := &config.Config{}
	m := NewFileTree(files, cfg)
	m.selected = 2 // Start at bottom

	// Test up navigation with 'k'
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'k'}))
	assert.Equal(t, 1, m.selected)

	// Test up navigation with arrow key
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	assert.Equal(t, 0, m.selected)

	// Test boundary (can't go above 0)
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'k'}))
	assert.Equal(t, 0, m.selected)
}

func TestFileTreeJumpToTop(t *testing.T) {
	files := []*gitdiff.File{
		{NewName: "file1.go"},
		{NewName: "file2.go"},
		{NewName: "file3.go"},
	}

	cfg := &config.Config{}
	m := NewFileTree(files, cfg)
	m.selected = 2

	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'g'}))
	assert.Equal(t, 0, m.selected)
}

func TestFileTreeJumpToBottom(t *testing.T) {
	files := []*gitdiff.File{
		{NewName: "file1.go"},
		{NewName: "file2.go"},
		{NewName: "file3.go"},
	}

	cfg := &config.Config{}
	m := NewFileTree(files, cfg)

	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'G'}))
	assert.Equal(t, 2, m.selected)
}

func TestFileTreeSelectedFile(t *testing.T) {
	files := []*gitdiff.File{
		{NewName: "file1.go"},
		{NewName: "file2.go"},
	}

	cfg := &config.Config{}
	m := NewFileTree(files, cfg)

	// Test initial selection
	selected := m.SelectedFile()
	require.NotNil(t, selected)
	assert.Equal(t, "file1.go", selected.NewName)

	// Move down and test
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'j'}))
	selected = m.SelectedFile()
	require.NotNil(t, selected)
	assert.Equal(t, "file2.go", selected.NewName)
}

func TestFileTreeSelectedFileEmpty(t *testing.T) {
	cfg := &config.Config{}
	m := NewFileTree([]*gitdiff.File{}, cfg)

	selected := m.SelectedFile()
	assert.Nil(t, selected)
}

func TestFileTreeSetSize(t *testing.T) {
	cfg := &config.Config{}
	m := NewFileTree([]*gitdiff.File{}, cfg)

	m.SetSize(80, 40)
	assert.Equal(t, 80, m.width)
	assert.Equal(t, 40, m.height)
}

func TestFileTreeSetFiles(t *testing.T) {
	files1 := []*gitdiff.File{
		{NewName: "file1.go"},
		{NewName: "file2.go"},
	}
	files2 := []*gitdiff.File{
		{NewName: "file3.go"},
	}

	cfg := &config.Config{}
	m := NewFileTree(files1, cfg)
	m.selected = 1

	// Replace with fewer files - selection should adjust
	m.SetFiles(files2)
	assert.Len(t, m.files, 1)
	assert.Equal(t, 0, m.selected) // Should reset to valid index
}

func TestGetFileIconNerdFont(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{"Go file", "main.go", styles.IconFileGo},
		{"JavaScript file", "index.js", styles.IconFileJS},
		{"TypeScript file", "app.ts", styles.IconFileTS},
		{"Python file", "script.py", styles.IconFilePython},
		{"Markdown file", "README.md", styles.IconFileMarkdown},
		{"JSON file", "config.json", styles.IconFileJSON},
		{"YAML file", "config.yaml", styles.IconFileYAML},
		{"Dockerfile", "Dockerfile", styles.IconFileDocker},
		{"Makefile", "Makefile", styles.IconFileMakefile},
		{"Unknown extension", "file.xyz", styles.IconFileDefault},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			icon := getFileIconNerdFont(tt.path)
			assert.Equal(t, tt.expected, icon)
		})
	}
}

func TestGetFileIconUnicode(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{"Markdown file", "README.md", "📝"},
		{"JSON file", "config.json", "⚙️"},
		{"Go file", "main.go", "💻"},
		{"HTML file", "index.html", "🌐"},
		{"Unknown extension", "file.xyz", "📄"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			icon := getFileIconUnicode(tt.path)
			assert.Equal(t, tt.expected, icon)
		})
	}
}

func TestFileTreeViewEmpty(t *testing.T) {
	cfg := &config.Config{}
	m := NewFileTree([]*gitdiff.File{}, cfg)
	m.SetSize(80, 20)

	view := m.View()
	assert.Contains(t, view, "No files changed")
}

func TestFileTreeViewWithFiles(t *testing.T) {
	files := []*gitdiff.File{
		{
			NewName: "file1.go",
			TextFragments: []*gitdiff.TextFragment{
				{
					Lines: []gitdiff.Line{
						{Op: gitdiff.OpAdd},
						{Op: gitdiff.OpAdd},
						{Op: gitdiff.OpDelete},
					},
				},
			},
		},
	}

	cfg := &config.Config{}
	m := NewFileTree(files, cfg)
	m.SetSize(80, 20)

	view := m.View()
	assert.Contains(t, view, "file1.go")
	assert.Contains(t, view, "+2 -1") // Stats from diff
}

func TestFileTreeRenderFileWithDeletion(t *testing.T) {
	// File where NewName is empty (deletion)
	file := &gitdiff.File{
		OldName: "deleted.go",
		NewName: "",
		TextFragments: []*gitdiff.TextFragment{
			{
				Lines: []gitdiff.Line{
					{Op: gitdiff.OpDelete},
					{Op: gitdiff.OpDelete},
				},
			},
		},
	}

	cfg := &config.Config{}
	m := NewFileTree([]*gitdiff.File{file}, cfg)
	m.SetSize(80, 20)

	view := m.View()
	assert.Contains(t, view, "deleted.go")
	assert.Contains(t, view, "+0 -2")
}

// Golden file tests for View rendering

func TestFileTreeView_Empty(t *testing.T) {
	cfg := &config.Config{}
	m := NewFileTree([]*gitdiff.File{}, cfg)
	m.SetSize(80, 20)

	output := tuitest.StripANSI(m.View())
	golden.RequireEqual(t, []byte(output))
}

func TestFileTreeView_SingleFile(t *testing.T) {
	files := []*gitdiff.File{
		{
			NewName: "main.go",
			TextFragments: []*gitdiff.TextFragment{
				{
					Lines: []gitdiff.Line{
						{Op: gitdiff.OpAdd},
						{Op: gitdiff.OpAdd},
						{Op: gitdiff.OpDelete},
					},
				},
			},
		},
	}

	cfg := &config.Config{}
	m := NewFileTree(files, cfg)
	m.SetSize(80, 20)

	output := tuitest.StripANSI(m.View())
	golden.RequireEqual(t, []byte(output))
}

func TestFileTreeView_MultipleFiles(t *testing.T) {
	files := []*gitdiff.File{
		{
			NewName: "main.go",
			TextFragments: []*gitdiff.TextFragment{
				{
					Lines: []gitdiff.Line{
						{Op: gitdiff.OpAdd},
						{Op: gitdiff.OpAdd},
						{Op: gitdiff.OpDelete},
					},
				},
			},
		},
		{
			NewName: "utils.go",
			TextFragments: []*gitdiff.TextFragment{
				{
					Lines: []gitdiff.Line{
						{Op: gitdiff.OpAdd},
						{Op: gitdiff.OpDelete},
						{Op: gitdiff.OpDelete},
					},
				},
			},
		},
		{
			NewName: "config.json",
			TextFragments: []*gitdiff.TextFragment{
				{
					Lines: []gitdiff.Line{
						{Op: gitdiff.OpAdd},
					},
				},
			},
		},
	}

	cfg := &config.Config{}
	m := NewFileTree(files, cfg)
	m.SetSize(80, 20)

	output := tuitest.StripANSI(m.View())
	golden.RequireEqual(t, []byte(output))
}

func TestFileTreeView_WithSelection(t *testing.T) {
	files := []*gitdiff.File{
		{NewName: "file1.go"},
		{NewName: "file2.go"},
		{NewName: "file3.go"},
	}

	cfg := &config.Config{}
	m := NewFileTree(files, cfg)
	m.selected = 1 // Select middle file
	m.SetSize(80, 20)

	output := tuitest.StripANSI(m.View())
	golden.RequireEqual(t, []byte(output))
}

func TestFileTreeView_DeletedFile(t *testing.T) {
	files := []*gitdiff.File{
		{
			OldName: "deleted.go",
			NewName: "",
			TextFragments: []*gitdiff.TextFragment{
				{
					Lines: []gitdiff.Line{
						{Op: gitdiff.OpDelete},
						{Op: gitdiff.OpDelete},
						{Op: gitdiff.OpDelete},
					},
				},
			},
		},
	}

	cfg := &config.Config{}
	m := NewFileTree(files, cfg)
	m.SetSize(80, 20)

	output := tuitest.StripANSI(m.View())
	golden.RequireEqual(t, []byte(output))
}

func TestFileTreeView_NerdFonts(t *testing.T) {
	files := []*gitdiff.File{
		{NewName: "main.go"},
		{NewName: "script.py"},
		{NewName: "README.md"},
		{NewName: "Dockerfile"},
	}

	iconsEnabled := true
	cfg := &config.Config{
		TUI: config.TUIConfig{
			Icons: &iconsEnabled,
		},
	}
	m := NewFileTree(files, cfg)
	m.SetSize(80, 20)

	output := tuitest.StripANSI(m.View())
	golden.RequireEqual(t, []byte(output))
}

func TestFileTreeView_ASCII(t *testing.T) {
	files := []*gitdiff.File{
		{NewName: "main.go"},
		{NewName: "script.py"},
		{NewName: "README.md"},
	}

	cfg := &config.Config{}
	m := NewFileTree(files, cfg)
	m.iconStyle = IconStyleASCII
	m.SetSize(80, 20)

	output := tuitest.StripANSI(m.View())
	golden.RequireEqual(t, []byte(output))
}

func TestFileTreeView_Hierarchical(t *testing.T) {
	files := []*gitdiff.File{
		{
			NewName: "internal/tui/diff/filetree.go",
			TextFragments: []*gitdiff.TextFragment{
				{
					Lines: []gitdiff.Line{
						{Op: gitdiff.OpAdd},
						{Op: gitdiff.OpAdd},
						{Op: gitdiff.OpAdd},
					},
				},
			},
		},
		{
			NewName: "internal/tui/diff/filetree_test.go",
			TextFragments: []*gitdiff.TextFragment{
				{
					Lines: []gitdiff.Line{
						{Op: gitdiff.OpAdd},
						{Op: gitdiff.OpDelete},
					},
				},
			},
		},
		{
			NewName: "internal/core/config/config.go",
			TextFragments: []*gitdiff.TextFragment{
				{
					Lines: []gitdiff.Line{
						{Op: gitdiff.OpAdd},
					},
				},
			},
		},
		{
			NewName: "README.md",
			TextFragments: []*gitdiff.TextFragment{
				{
					Lines: []gitdiff.Line{
						{Op: gitdiff.OpAdd},
						{Op: gitdiff.OpAdd},
					},
				},
			},
		},
	}

	cfg := &config.Config{}
	m := NewFileTree(files, cfg)
	m.iconStyle = IconStyleASCII
	m.SetSize(80, 20)

	output := tuitest.StripANSI(m.View())
	golden.RequireEqual(t, []byte(output))
}

func TestFileTreeView_HierarchicalCollapsed(t *testing.T) {
	files := []*gitdiff.File{
		{NewName: "internal/tui/diff/filetree.go"},
		{NewName: "internal/tui/diff/filetree_test.go"},
		{NewName: "internal/core/config/config.go"},
	}

	cfg := &config.Config{}
	m := NewFileTree(files, cfg)
	m.iconStyle = IconStyleASCII
	m.SetSize(80, 20)

	// Collapse the "internal" directory
	m.visible[0].Expanded = false
	m.rebuildVisible()

	output := tuitest.StripANSI(m.View())
	golden.RequireEqual(t, []byte(output))
}

func TestFileTreeExpandCollapseWithKeys(t *testing.T) {
	files := []*gitdiff.File{
		{NewName: "src/main.go"},
		{NewName: "src/utils.go"},
	}

	cfg := &config.Config{}
	m := NewFileTree(files, cfg)

	// Initially, src directory should be expanded
	assert.True(t, m.visible[0].Expanded)
	assert.Equal(t, 3, len(m.visible)) // src/, main.go, utils.go

	// Press enter to collapse the directory
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.False(t, m.visible[0].Expanded)
	assert.Equal(t, 1, len(m.visible)) // Just src/

	// Press enter again to expand
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.True(t, m.visible[0].Expanded)
	assert.Equal(t, 3, len(m.visible))
}

func TestFileTreeJumpToParent(t *testing.T) {
	files := []*gitdiff.File{
		{NewName: "src/components/Button.tsx"},
		{NewName: "src/utils/helpers.ts"},
	}

	cfg := &config.Config{}
	m := NewFileTree(files, cfg)

	// Navigate to Button.tsx (should be at index 2: src/, components/, Button.tsx)
	m.selected = 2

	// Press left to jump to parent (components/)
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft}))
	assert.Equal(t, 1, m.selected) // Should be on components/
	assert.True(t, m.visible[1].IsDir)
	assert.Equal(t, "components", m.visible[1].Name)
}
