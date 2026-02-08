package diff

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDiffLines_SimpleDiff(t *testing.T) {
	diff := `--- a/file.go
+++ b/file.go
@@ -1,3 +1,4 @@
 package main
 func main() {
+	fmt.Println("hello")
 }`

	lines, err := ParseDiffLines(diff)
	require.NoError(t, err)
	require.Len(t, lines, 7) // 2 headers + 1 hunk + 4 content lines

	// Check file headers
	assert.Equal(t, LineTypeFileHeader, lines[0].Type)
	assert.Equal(t, "--- a/file.go", lines[0].Content)

	assert.Equal(t, LineTypeFileHeader, lines[1].Type)
	assert.Equal(t, "+++ b/file.go", lines[1].Content)

	// Check hunk header
	assert.Equal(t, LineTypeHunk, lines[2].Type)
	assert.Equal(t, 1, lines[2].OldLineNum)
	assert.Equal(t, 1, lines[2].NewLineNum)

	// Check context lines
	assert.Equal(t, LineTypeContext, lines[3].Type)
	assert.Equal(t, "package main", lines[3].Content)
	assert.Equal(t, 1, lines[3].OldLineNum)
	assert.Equal(t, 1, lines[3].NewLineNum)

	assert.Equal(t, LineTypeContext, lines[4].Type)
	assert.Equal(t, "func main() {", lines[4].Content)
	assert.Equal(t, 2, lines[4].OldLineNum)
	assert.Equal(t, 2, lines[4].NewLineNum)

	// Check addition
	assert.Equal(t, LineTypeAdd, lines[5].Type)
	assert.Equal(t, "\tfmt.Println(\"hello\")", lines[5].Content)
	assert.Equal(t, 0, lines[5].OldLineNum) // Not in old file
	assert.Equal(t, 3, lines[5].NewLineNum)

	// Check closing brace context line
	assert.Equal(t, LineTypeContext, lines[6].Type)
	assert.Equal(t, "}", lines[6].Content)
	assert.Equal(t, 3, lines[6].OldLineNum)
	assert.Equal(t, 4, lines[6].NewLineNum)
}

func TestParseDiffLines_WithDeletions(t *testing.T) {
	diff := `--- a/file.go
+++ b/file.go
@@ -1,3 +1,2 @@
 package main
-func old() {}
 func new() {}`

	lines, err := ParseDiffLines(diff)
	require.NoError(t, err)

	// Find the deletion line
	var deletion *ParsedLine
	for i := range lines {
		if lines[i].Type == LineTypeDelete {
			deletion = &lines[i]
			break
		}
	}
	require.NotNil(t, deletion, "should find deletion line")

	assert.Equal(t, "func old() {}", deletion.Content)
	assert.Equal(t, 2, deletion.OldLineNum)
	assert.Equal(t, 0, deletion.NewLineNum) // Not in new file
}

func TestParseDiffLines_MultipleHunks(t *testing.T) {
	diff := `--- a/file.go
+++ b/file.go
@@ -1,2 +1,3 @@
 line1
+added1
 line2
@@ -10,2 +11,3 @@
 line10
+added2
 line11`

	lines, err := ParseDiffLines(diff)
	require.NoError(t, err)

	// Count hunk headers and find their positions
	var hunkPositions []int
	for i, line := range lines {
		if line.Type == LineTypeHunk {
			hunkPositions = append(hunkPositions, i)
		}
	}
	require.Len(t, hunkPositions, 2, "should have 2 hunks")

	// Verify line numbers reset correctly for second hunk
	secondHunkIdx := hunkPositions[1]

	// Check line after second hunk header
	firstLineAfterSecondHunk := lines[secondHunkIdx+1]
	assert.Equal(t, LineTypeContext, firstLineAfterSecondHunk.Type)
	assert.Equal(t, 10, firstLineAfterSecondHunk.OldLineNum)
	assert.Equal(t, 11, firstLineAfterSecondHunk.NewLineNum)
}

func TestParseDiffLines_EmptyDiff(t *testing.T) {
	lines, err := ParseDiffLines("")
	require.NoError(t, err)
	assert.Empty(t, lines)
}

func TestParseDiffLines_OnlyHeaders(t *testing.T) {
	diff := `--- a/file.go
+++ b/file.go`

	lines, err := ParseDiffLines(diff)
	require.NoError(t, err)
	require.Len(t, lines, 2)

	assert.Equal(t, LineTypeFileHeader, lines[0].Type)
	assert.Equal(t, LineTypeFileHeader, lines[1].Type)
}

func TestParseHunkHeader_Standard(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected HunkHeader
		wantErr  bool
	}{
		{
			name:  "simple range",
			input: "@@ -1,7 +1,8 @@",
			expected: HunkHeader{
				OldStart: 1,
				OldCount: 7,
				NewStart: 1,
				NewCount: 8,
			},
		},
		{
			name:  "single line old",
			input: "@@ -1 +1,2 @@",
			expected: HunkHeader{
				OldStart: 1,
				OldCount: 1,
				NewStart: 1,
				NewCount: 2,
			},
		},
		{
			name:  "with function comment",
			input: "@@ -1,7 +1,8 @@ func main()",
			expected: HunkHeader{
				OldStart: 1,
				OldCount: 7,
				NewStart: 1,
				NewCount: 8,
				Comment:  "func main()",
			},
		},
		{
			name:  "large line numbers",
			input: "@@ -1234,56 +5678,90 @@ MyClass::method()",
			expected: HunkHeader{
				OldStart: 1234,
				OldCount: 56,
				NewStart: 5678,
				NewCount: 90,
				Comment:  "MyClass::method()",
			},
		},
		{
			name:    "missing @@ prefix",
			input:   "not a hunk",
			wantErr: true,
		},
		{
			name:    "missing closing @@",
			input:   "@@ -1,7 +1,8",
			wantErr: true,
		},
		{
			name:    "missing minus prefix",
			input:   "@@ 1,7 +1,8 @@",
			wantErr: true,
		},
		{
			name:    "missing plus prefix",
			input:   "@@ -1,7 1,8 @@",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseHunkHeader(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected.OldStart, result.OldStart)
			assert.Equal(t, tt.expected.OldCount, result.OldCount)
			assert.Equal(t, tt.expected.NewStart, result.NewStart)
			assert.Equal(t, tt.expected.NewCount, result.NewCount)
			assert.Equal(t, tt.expected.Comment, result.Comment)
		})
	}
}

func TestParseRange(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantStart int
		wantCount int
		wantErr   bool
	}{
		{
			name:      "single line",
			input:     "1",
			wantStart: 1,
			wantCount: 1,
		},
		{
			name:      "range",
			input:     "5,10",
			wantStart: 5,
			wantCount: 10,
		},
		{
			name:      "zero start",
			input:     "0,5",
			wantStart: 0,
			wantCount: 5,
		},
		{
			name:    "invalid format",
			input:   "1,2,3",
			wantErr: true,
		},
		{
			name:    "non-numeric start",
			input:   "abc",
			wantErr: true,
		},
		{
			name:    "non-numeric count",
			input:   "1,xyz",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, count, err := parseRange(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantStart, start)
			assert.Equal(t, tt.wantCount, count)
		})
	}
}

func TestFilterByType(t *testing.T) {
	lines := []ParsedLine{
		{Type: LineTypeFileHeader, Content: "--- a/file"},
		{Type: LineTypeHunk, Content: "@@ -1 +1 @@"},
		{Type: LineTypeAdd, Content: "added"},
		{Type: LineTypeDelete, Content: "deleted"},
		{Type: LineTypeContext, Content: "context"},
		{Type: LineTypeAdd, Content: "another add"},
	}

	adds := FilterByType(lines, LineTypeAdd)
	assert.Len(t, adds, 2)
	assert.Equal(t, "added", adds[0].Content)
	assert.Equal(t, "another add", adds[1].Content)

	deletes := FilterByType(lines, LineTypeDelete)
	assert.Len(t, deletes, 1)
	assert.Equal(t, "deleted", deletes[0].Content)

	headers := FilterByType(lines, LineTypeFileHeader)
	assert.Len(t, headers, 1)
}

func TestGetLineAtOffset(t *testing.T) {
	lines := []ParsedLine{
		{Type: LineTypeFileHeader, Content: "header1"},
		{Type: LineTypeFileHeader, Content: "header2"},
		{Type: LineTypeAdd, Content: "add"},
	}

	// Valid offsets
	line := GetLineAtOffset(lines, 0)
	require.NotNil(t, line)
	assert.Equal(t, "header1", line.Content)

	line = GetLineAtOffset(lines, 2)
	require.NotNil(t, line)
	assert.Equal(t, "add", line.Content)

	// Invalid offsets
	assert.Nil(t, GetLineAtOffset(lines, -1))
	assert.Nil(t, GetLineAtOffset(lines, 3))
	assert.Nil(t, GetLineAtOffset(lines, 100))
}

func TestParseDiffLines_RealWorldExample(t *testing.T) {
	// Real-world diff with mixed changes
	diff := `--- a/internal/tui/diff/model.go
+++ b/internal/tui/diff/model.go
@@ -15,7 +15,8 @@ type Model struct {
 	fileTree   filetree.Model
 	diffViewer DiffViewerModel

-	width  int
+	width      int
+	height     int
 	height int
 }

@@ -45,6 +46,7 @@ func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
 		m.width = msg.Width
 		m.height = msg.Height

+		// Update child component sizes
 		treeWidth := m.width / 3
 		diffWidth := m.width - treeWidth`

	lines, err := ParseDiffLines(diff)
	require.NoError(t, err)
	require.NotEmpty(t, lines)

	// Verify we can find both hunks
	hunkCount := 0
	for _, line := range lines {
		if line.Type == LineTypeHunk {
			hunkCount++
		}
	}
	assert.Equal(t, 2, hunkCount, "should have 2 hunks")

	// Verify line numbers are tracked correctly across hunks
	addCount := 0
	deleteCount := 0
	for _, line := range lines {
		if line.Type == LineTypeAdd {
			addCount++
			assert.Greater(t, line.NewLineNum, 0, "add line should have new line number")
			assert.Equal(t, 0, line.OldLineNum, "add line should not have old line number")
		}
		if line.Type == LineTypeDelete {
			deleteCount++
			assert.Greater(t, line.OldLineNum, 0, "delete line should have old line number")
			assert.Equal(t, 0, line.NewLineNum, "delete line should not have new line number")
		}
	}
	assert.Greater(t, addCount, 0, "should have additions")
	assert.Greater(t, deleteCount, 0, "should have deletions")
}

func TestParseDiffLines_PreservesRawLine(t *testing.T) {
	diff := `--- a/file.go
+++ b/file.go
@@ -1,2 +1,3 @@
 context
+addition
-deletion`

	lines, err := ParseDiffLines(diff)
	require.NoError(t, err)

	// Find the context, addition, and deletion lines
	var context, addition, deletion *ParsedLine
	for i := range lines {
		switch lines[i].Type {
		case LineTypeContext:
			if context == nil {
				context = &lines[i]
			}
		case LineTypeAdd:
			addition = &lines[i]
		case LineTypeDelete:
			deletion = &lines[i]
		}
	}

	require.NotNil(t, context)
	require.NotNil(t, addition)
	require.NotNil(t, deletion)

	// RawLine should preserve original prefixes
	assert.Equal(t, " context", context.RawLine)
	assert.Equal(t, "+addition", addition.RawLine)
	assert.Equal(t, "-deletion", deletion.RawLine)

	// Content should strip prefixes
	assert.Equal(t, "context", context.Content)
	assert.Equal(t, "addition", addition.Content)
	assert.Equal(t, "deletion", deletion.Content)
}
