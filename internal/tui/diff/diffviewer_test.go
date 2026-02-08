package diff

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bluekeyes/go-gitdiff/gitdiff"
	"github.com/charmbracelet/x/exp/golden"
	"github.com/hay-kot/hive/pkg/tuitest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDiffViewer(t *testing.T) {
	file := &gitdiff.File{
		OldName: "test.go",
		NewName: "test.go",
		TextFragments: []*gitdiff.TextFragment{
			{
				OldPosition: 1,
				OldLines:    3,
				NewPosition: 1,
				NewLines:    3,
				Lines: []gitdiff.Line{
					{Op: gitdiff.OpContext, Line: "package main\n"},
					{Op: gitdiff.OpDelete, Line: "old line\n"},
					{Op: gitdiff.OpAdd, Line: "new line\n"},
				},
			},
		},
	}

	m := NewDiffViewer(file)

	assert.NotNil(t, m.file)
	assert.Equal(t, 0, m.offset)
	assert.NotEmpty(t, m.content)
	assert.NotEmpty(t, m.lines)
}

func TestNewDiffViewerNilFile(t *testing.T) {
	m := NewDiffViewer(nil)

	assert.Nil(t, m.file)
	assert.Equal(t, 0, m.offset)
	assert.Empty(t, m.content)
	assert.Empty(t, m.lines)
}

func TestDiffViewerScrollDown(t *testing.T) {
	file := &gitdiff.File{
		OldName: "test.go",
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
	}

	m := NewDiffViewer(file)
	m.SetSize(80, 8) // Height 8 = 3 header + 5 content (viewport shows lines 0-4)

	// Initial position
	assert.Equal(t, 0, m.offset)
	assert.Equal(t, 0, m.cursorLine)

	// Move cursor down with 'j' - cursor moves but viewport stays since cursor is still visible
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'j'}))
	assert.Equal(t, 1, m.cursorLine)
	assert.Equal(t, 0, m.offset) // viewport doesn't need to scroll yet

	// Move down more to reach viewport bottom (line 4)
	for i := 0; i < 3; i++ {
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'j'}))
	}
	assert.Equal(t, 4, m.cursorLine)
	assert.Equal(t, 0, m.offset) // still fits in viewport (0-4)

	// Move down one more - now viewport must scroll
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'j'}))
	assert.Equal(t, 5, m.cursorLine)
	assert.Equal(t, 1, m.offset) // viewport scrolls to show line 5
}

func TestDiffViewerScrollUp(t *testing.T) {
	file := &gitdiff.File{
		OldName: "test.go",
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
	}

	m := NewDiffViewer(file)
	m.SetSize(80, 5) // viewport shows 5 lines at a time
	m.offset = 5     // Start in the middle (showing lines 5-9)
	m.cursorLine = 5 // Cursor at top of viewport

	// Move up with 'k' - cursor moves up, forcing viewport to scroll
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'k'}))
	assert.Equal(t, 4, m.cursorLine)
	assert.Equal(t, 4, m.offset) // viewport scrolls to keep cursor visible

	// Move up with arrow key
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	assert.Equal(t, 3, m.cursorLine)
	assert.Equal(t, 3, m.offset)

	// Can't scroll above 0
	m.offset = 0
	m.cursorLine = 0
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'k'}))
	assert.Equal(t, 0, m.cursorLine)
	assert.Equal(t, 0, m.offset)
}

func TestDiffViewerPageScroll(t *testing.T) {
	file := &gitdiff.File{
		OldName: "test.go",
		NewName: "test.go",
		TextFragments: []*gitdiff.TextFragment{
			{
				OldPosition: 1,
				OldLines:    20,
				NewPosition: 1,
				NewLines:    20,
				Lines: func() []gitdiff.Line {
					lines := make([]gitdiff.Line, 20)
					for i := range lines {
						lines[i] = gitdiff.Line{Op: gitdiff.OpContext, Line: "line\n"}
					}
					return lines
				}(),
			},
		},
	}

	m := NewDiffViewer(file)
	m.SetSize(80, 13) // Height 13 = 3 header + 10 content

	// Scroll down half page (ctrl+d)
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'd', Mod: tea.ModCtrl}))
	assert.Equal(t, 5, m.offset) // contentHeight 10/2 = 5

	// Scroll up half page (ctrl+u)
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'u', Mod: tea.ModCtrl}))
	assert.Equal(t, 0, m.offset) // 5 - 5 = 0
}

func TestDiffViewerJumpToTopBottom(t *testing.T) {
	file := &gitdiff.File{
		OldName: "test.go",
		NewName: "test.go",
		TextFragments: []*gitdiff.TextFragment{
			{
				OldPosition: 1,
				OldLines:    20,
				NewPosition: 1,
				NewLines:    20,
				Lines: func() []gitdiff.Line {
					lines := make([]gitdiff.Line, 20)
					for i := range lines {
						lines[i] = gitdiff.Line{Op: gitdiff.OpContext, Line: "line\n"}
					}
					return lines
				}(),
			},
		},
	}

	m := NewDiffViewer(file)
	m.SetSize(80, 8) // Height 8 = 3 header + 5 content
	m.offset = 10

	// Jump to top with 'g'
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'g'}))
	assert.Equal(t, 0, m.offset)

	// Jump to bottom with 'G'
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'G'}))
	contentHeight := 5 // 8 - 3 header
	maxOffset := len(m.lines) - contentHeight
	assert.Equal(t, maxOffset, m.offset)
}

func TestDiffViewerSetSize(t *testing.T) {
	m := NewDiffViewer(nil)

	m.SetSize(100, 50)

	assert.Equal(t, 100, m.width)
	assert.Equal(t, 50, m.height)
}

func TestDiffViewerSetFile(t *testing.T) {
	file1 := &gitdiff.File{
		OldName: "file1.go",
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
	}

	file2 := &gitdiff.File{
		OldName: "file2.go",
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
	}

	m := NewDiffViewer(file1)
	m.offset = 5 // Set non-zero offset

	// Change to file2
	m.SetFile(file2)

	assert.Equal(t, "file2.go", m.file.NewName)
	assert.Equal(t, 0, m.offset) // Offset should reset
	assert.NotEmpty(t, m.content)
}

func TestDiffViewerViewEmpty(t *testing.T) {
	m := NewDiffViewer(nil)
	m.SetSize(80, 20)

	view := m.View()

	assert.Contains(t, view, "No File Selected")
}

func TestDiffViewerGenerateContent(t *testing.T) {
	file := &gitdiff.File{
		OldName: "old.go",
		NewName: "new.go",
		TextFragments: []*gitdiff.TextFragment{
			{
				OldPosition: 10,
				OldLines:    3,
				NewPosition: 10,
				NewLines:    4,
				Comment:     "func main()",
				Lines: []gitdiff.Line{
					{Op: gitdiff.OpContext, Line: "package main\n"},
					{Op: gitdiff.OpDelete, Line: "// old comment\n"},
					{Op: gitdiff.OpAdd, Line: "// new comment\n"},
					{Op: gitdiff.OpAdd, Line: "// another line\n"},
				},
			},
		},
	}

	m := NewDiffViewer(file)

	require.NotEmpty(t, m.content)
	require.NotEmpty(t, m.lines)

	// Check unified diff header
	assert.Contains(t, m.content, "--- old.go")
	assert.Contains(t, m.content, "+++ new.go")

	// Check hunk header
	assert.Contains(t, m.content, "@@ -10,3 +10,4 @@ func main()")

	// Check lines (may have ANSI color codes from delta)
	assert.Contains(t, m.content, "package")
	assert.Contains(t, m.content, "main")
	assert.Contains(t, m.content, "old")
	assert.Contains(t, m.content, "comment")
	assert.Contains(t, m.content, "new")
	assert.Contains(t, m.content, "another line")
}

func TestFormatRange(t *testing.T) {
	tests := []struct {
		name     string
		pos      int64
		length   int64
		expected string
	}{
		{"single line", 10, 1, "10"},
		{"multiple lines", 10, 5, "10,5"},
		{"zero position", 0, 1, "0"},
		{"large numbers", 1234, 567, "1234,567"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatRange(tt.pos, tt.length)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Golden file tests for selection rendering

func TestDiffViewerView_NormalMode(t *testing.T) {
	file := &gitdiff.File{
		OldName: "test.go",
		NewName: "test.go",
		TextFragments: []*gitdiff.TextFragment{
			{
				OldPosition: 1,
				OldLines:    3,
				NewPosition: 1,
				NewLines:    4,
				Lines: []gitdiff.Line{
					{Op: gitdiff.OpContext, Line: "package main\n"},
					{Op: gitdiff.OpDelete, Line: "old line\n"},
					{Op: gitdiff.OpAdd, Line: "new line\n"},
					{Op: gitdiff.OpContext, Line: "}\n"},
				},
			},
		},
	}

	m := NewDiffViewer(file)
	m.SetSize(80, 10)

	// Strip ANSI for easy inspection - this test verifies content structure only
	output := tuitest.StripANSI(m.View())
	golden.RequireEqual(t, []byte(output))
}

func TestDiffViewerView_CursorHighlight(t *testing.T) {
	file := &gitdiff.File{
		OldName: "test.go",
		NewName: "test.go",
		TextFragments: []*gitdiff.TextFragment{
			{
				OldPosition: 1,
				OldLines:    3,
				NewPosition: 1,
				NewLines:    4,
				Lines: []gitdiff.Line{
					{Op: gitdiff.OpContext, Line: "package main\n"},
					{Op: gitdiff.OpDelete, Line: "old line\n"},
					{Op: gitdiff.OpAdd, Line: "new line\n"},
					{Op: gitdiff.OpContext, Line: "}\n"},
				},
			},
		},
	}

	m := NewDiffViewer(file)
	m.SetSize(80, 10)

	// Move cursor down to highlight a line
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))

	// Keep ANSI codes to verify styling
	output := m.View()
	golden.RequireEqual(t, []byte(output))
}

func TestDiffViewerView_SingleLineSelection(t *testing.T) {
	file := &gitdiff.File{
		OldName: "test.go",
		NewName: "test.go",
		TextFragments: []*gitdiff.TextFragment{
			{
				OldPosition: 1,
				OldLines:    3,
				NewPosition: 1,
				NewLines:    4,
				Lines: []gitdiff.Line{
					{Op: gitdiff.OpContext, Line: "package main\n"},
					{Op: gitdiff.OpDelete, Line: "old line\n"},
					{Op: gitdiff.OpAdd, Line: "new line\n"},
					{Op: gitdiff.OpContext, Line: "}\n"},
				},
			},
		},
	}

	m := NewDiffViewer(file)
	m.SetSize(80, 10)

	// Move cursor and enter visual mode
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'v'}))

	// Keep ANSI codes to verify styling
	output := m.View()
	golden.RequireEqual(t, []byte(output))
}

func TestDiffViewerView_MultiLineSelection(t *testing.T) {
	file := &gitdiff.File{
		OldName: "test.go",
		NewName: "test.go",
		TextFragments: []*gitdiff.TextFragment{
			{
				OldPosition: 1,
				OldLines:    5,
				NewPosition: 1,
				NewLines:    6,
				Lines: []gitdiff.Line{
					{Op: gitdiff.OpContext, Line: "package main\n"},
					{Op: gitdiff.OpContext, Line: "func main() {\n"},
					{Op: gitdiff.OpDelete, Line: "old line 1\n"},
					{Op: gitdiff.OpDelete, Line: "old line 2\n"},
					{Op: gitdiff.OpAdd, Line: "new line 1\n"},
					{Op: gitdiff.OpAdd, Line: "new line 2\n"},
					{Op: gitdiff.OpContext, Line: "}\n"},
				},
			},
		},
	}

	m := NewDiffViewer(file)
	m.SetSize(80, 15)

	// Move cursor, enter visual mode, and extend selection
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'v'})) // Start selection
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))

	// Keep ANSI codes to verify styling
	output := m.View()
	golden.RequireEqual(t, []byte(output))
}

func TestDiffViewerView_SelectionAcrossAdditions(t *testing.T) {
	file := &gitdiff.File{
		OldName: "test.go",
		NewName: "test.go",
		TextFragments: []*gitdiff.TextFragment{
			{
				OldPosition: 1,
				OldLines:    2,
				NewPosition: 1,
				NewLines:    5,
				Lines: []gitdiff.Line{
					{Op: gitdiff.OpContext, Line: "package main\n"},
					{Op: gitdiff.OpAdd, Line: "import \"fmt\"\n"},
					{Op: gitdiff.OpAdd, Line: "import \"os\"\n"},
					{Op: gitdiff.OpAdd, Line: "\n"},
					{Op: gitdiff.OpContext, Line: "func main() {\n"},
				},
			},
		},
	}

	m := NewDiffViewer(file)
	m.SetSize(80, 12)

	// Select multiple addition lines
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'v'})) // Start on first addition
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))

	// Keep ANSI codes to verify styling
	output := m.View()
	golden.RequireEqual(t, []byte(output))
}
