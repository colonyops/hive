package diff

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bluekeyes/go-gitdiff/gitdiff"
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
	m.SetSize(80, 5) // Small height to force scrolling

	// Initial position
	assert.Equal(t, 0, m.offset)

	// Scroll down with 'j'
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'j'}))
	assert.Equal(t, 1, m.offset)

	// Scroll down with arrow key
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	assert.Equal(t, 2, m.offset)
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
	m.SetSize(80, 5)
	m.offset = 5 // Start in the middle

	// Scroll up with 'k'
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'k'}))
	assert.Equal(t, 4, m.offset)

	// Scroll up with arrow key
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	assert.Equal(t, 3, m.offset)

	// Can't scroll above 0
	m.offset = 0
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'k'}))
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
	m.SetSize(80, 10) // Height = 10

	// Scroll down half page (ctrl+d)
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'd', Mod: tea.ModCtrl}))
	assert.Equal(t, 5, m.offset) // 10/2 = 5

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
	m.SetSize(80, 5)
	m.offset = 10

	// Jump to top with 'g'
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'g'}))
	assert.Equal(t, 0, m.offset)

	// Jump to bottom with 'G'
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'G'}))
	maxOffset := len(m.lines) - m.height
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

	assert.Contains(t, view, "No file selected")
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
