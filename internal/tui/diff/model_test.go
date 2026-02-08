package diff

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bluekeyes/go-gitdiff/gitdiff"
	"github.com/hay-kot/hive/internal/core/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	files := []*gitdiff.File{
		{NewName: "file1.go"},
		{NewName: "file2.go"},
	}

	cfg := &config.Config{}
	m := New(files, cfg)

	assert.NotNil(t, m.fileTree)
	assert.NotNil(t, m.diffViewer)
	assert.Equal(t, FocusFileTree, m.focused)
}

func TestNewWithNoFiles(t *testing.T) {
	cfg := &config.Config{}
	m := New(nil, cfg)

	assert.NotNil(t, m.fileTree)
	assert.NotNil(t, m.diffViewer)
	assert.Equal(t, FocusFileTree, m.focused)
}

func TestModelInit(t *testing.T) {
	cfg := &config.Config{}
	m := New(nil, cfg)

	cmd := m.Init()
	assert.Nil(t, cmd)
}

func TestModelTabSwitching(t *testing.T) {
	files := []*gitdiff.File{
		{NewName: "test.go"},
	}

	cfg := &config.Config{}
	m := New(files, cfg)

	// Initially focused on file tree
	assert.Equal(t, FocusFileTree, m.focused)

	// Press tab to switch to diff viewer
	result, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: '\t'}))
	m = result.(Model)
	assert.Equal(t, FocusDiffViewer, m.focused)

	// Press tab again to switch back to file tree
	result, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: '\t'}))
	m = result.(Model)
	assert.Equal(t, FocusFileTree, m.focused)
}

func TestModelFileTreeNavigation(t *testing.T) {
	files := []*gitdiff.File{
		{
			NewName: "file1.go",
			TextFragments: []*gitdiff.TextFragment{
				{
					OldPosition: 1,
					OldLines:    1,
					NewPosition: 1,
					NewLines:    1,
					Lines: []gitdiff.Line{
						{Op: gitdiff.OpContext, Line: "line1\n"},
					},
				},
			},
		},
		{
			NewName: "file2.go",
			TextFragments: []*gitdiff.TextFragment{
				{
					OldPosition: 1,
					OldLines:    1,
					NewPosition: 1,
					NewLines:    1,
					Lines: []gitdiff.Line{
						{Op: gitdiff.OpContext, Line: "line2\n"},
					},
				},
			},
		},
	}

	cfg := &config.Config{}
	m := New(files, cfg)
	m.SetSize(100, 40)

	// File tree is focused, first file should be shown
	selectedFile := m.fileTree.SelectedFile()
	require.NotNil(t, selectedFile)
	assert.Equal(t, "file1.go", selectedFile.NewName)

	// Navigate down with 'j'
	result, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'j'}))
	m = result.(Model)

	// Diff viewer should sync to second file
	selectedFile = m.fileTree.SelectedFile()
	require.NotNil(t, selectedFile)
	assert.Equal(t, "file2.go", selectedFile.NewName)
}

func TestModelEnterOnFileTree(t *testing.T) {
	files := []*gitdiff.File{
		{
			NewName: "file1.go",
			TextFragments: []*gitdiff.TextFragment{
				{
					OldPosition: 1,
					OldLines:    1,
					NewPosition: 1,
					NewLines:    1,
					Lines: []gitdiff.Line{
						{Op: gitdiff.OpContext, Line: "content\n"},
					},
				},
			},
		},
	}

	cfg := &config.Config{}
	m := New(files, cfg)
	m.SetSize(100, 40)

	// Press enter on file tree (should sync to diff viewer)
	result, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = result.(Model)

	// Verify diff viewer has the file
	assert.NotNil(t, m.diffViewer.file)
	assert.Equal(t, "file1.go", m.diffViewer.file.NewName)
}

func TestModelDiffViewerScrolling(t *testing.T) {
	files := []*gitdiff.File{
		{
			NewName: "test.go",
			TextFragments: []*gitdiff.TextFragment{
				{
					OldPosition: 1,
					OldLines:    10,
					NewPosition: 1,
					NewLines:    10,
					Lines: func() []gitdiff.Line {
						lines := make([]gitdiff.Line, 10)
						for i := range lines {
							lines[i] = gitdiff.Line{Op: gitdiff.OpContext, Line: "line\n"}
						}
						return lines
					}(),
				},
			},
		},
	}

	cfg := &config.Config{}
	m := New(files, cfg)
	m.SetSize(100, 10)

	// Switch to diff viewer
	result, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: '\t'}))
	m = result.(Model)
	assert.Equal(t, FocusDiffViewer, m.focused)

	// Initial scroll position
	assert.Equal(t, 0, m.diffViewer.offset)
	assert.Equal(t, 0, m.diffViewer.cursorLine)

	// Move cursor down - cursor moves but viewport stays if cursor is still visible
	result, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'j'}))
	m = result.(Model)
	assert.Equal(t, 1, m.diffViewer.cursorLine)
	assert.Equal(t, 0, m.diffViewer.offset) // viewport doesn't scroll yet

	// Move cursor back up
	result, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'k'}))
	m = result.(Model)
	assert.Equal(t, 0, m.diffViewer.cursorLine)
	assert.Equal(t, 0, m.diffViewer.offset)
}

func TestModelSetSize(t *testing.T) {
	cfg := &config.Config{}
	m := New(nil, cfg)

	m.SetSize(120, 50)

	assert.Equal(t, 120, m.width)
	assert.Equal(t, 50, m.height)

	// Check that child components received size updates
	// File tree should get 30% width
	expectedTreeWidth := 120 * 30 / 100
	assert.Equal(t, expectedTreeWidth, m.fileTree.width)

	// Diff viewer should get remaining width (minus separator)
	expectedDiffWidth := 120 - expectedTreeWidth - 1
	assert.Equal(t, expectedDiffWidth, m.diffViewer.width)

	// Both should get height minus status bar
	expectedHeight := 50 - 1
	assert.Equal(t, expectedHeight, m.fileTree.height)
	assert.Equal(t, expectedHeight, m.diffViewer.height)
}

func TestModelView(t *testing.T) {
	files := []*gitdiff.File{
		{NewName: "test.go"},
	}

	cfg := &config.Config{}
	m := New(files, cfg)
	m.SetSize(100, 30)

	view := m.View()

	// Should return non-empty view
	assert.NotEmpty(t, view)
}

func TestModelViewWithZeroSize(t *testing.T) {
	cfg := &config.Config{}
	m := New(nil, cfg)

	// No size set
	view := m.View()
	assert.Empty(t, view)
}
