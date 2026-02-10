package review

import (
	"strings"
	"testing"
	"time"

	corereview "github.com/hay-kot/hive/internal/core/review"
	"github.com/hay-kot/hive/internal/tui/testutil"
)

// createTestDocument creates a test document with known content.
func createTestDocument(lines []string) *Document {
	content := strings.Join(lines, "\n")
	return &Document{
		Path:    "/test/path/document.md",
		RelPath: "test/document.md",
		Type:    DocTypePlan,
		ModTime: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		Content: content,
	}
}

func TestDocumentView_RenderEmpty(t *testing.T) {
	doc := createTestDocument([]string{})
	dv := NewDocumentView(doc)
	dv.SetSize(80, 24)

	output := dv.Render()
	output = testutil.StripANSI(output)

	testutil.RequireGolden(t, output)
}

func TestDocumentView_RenderWithLineNumbers(t *testing.T) {
	doc := createTestDocument([]string{
		"# Test Document",
		"",
		"This is line 3.",
		"This is line 4.",
	})
	dv := NewDocumentView(doc)
	dv.SetSize(80, 24)

	output := dv.Render()
	output = testutil.StripANSI(output)

	testutil.RequireGolden(t, output)
}

func TestDocumentView_RenderWithComments(t *testing.T) {
	doc := createTestDocument([]string{
		"# Test Document",
		"",
		"This is line 3.",
		"This is line 4.",
	})
	dv := NewDocumentView(doc)
	dv.SetSize(80, 24)

	comments := []corereview.Comment{
		testutil.CreateTestComment(3, "This is a test comment"),
	}
	output := dv.RenderWithComments(comments)
	output = testutil.StripANSI(output)

	testutil.RequireGolden(t, output)
}

func TestDocumentView_RenderWithLongComment(t *testing.T) {
	doc := createTestDocument([]string{
		"# Test Document",
		"",
		"This is line 3.",
		"This is line 4.",
	})
	dv := NewDocumentView(doc)
	dv.SetSize(80, 24)

	longComment := "This is a very long comment that exceeds 80 characters to test wrapping behavior and formatting."
	comments := []corereview.Comment{
		testutil.CreateTestComment(3, longComment),
	}
	output := dv.RenderWithComments(comments)
	output = testutil.StripANSI(output)

	testutil.RequireGolden(t, output)
}

func TestDocumentView_RenderWithSelection(t *testing.T) {
	doc := createTestDocument([]string{
		"# Test Document",
		"",
		"Line 3",
		"Line 4",
		"Line 5",
		"Line 6",
		"Line 7",
	})
	dv := NewDocumentView(doc)
	dv.SetSize(80, 24)

	// Set selection on lines 4-6
	dv.SetSelection(4, 6)

	output := dv.Render()
	output = testutil.StripANSI(output)

	testutil.RequireGolden(t, output)
}

func TestDocumentView_RenderWithSearchHighlight(t *testing.T) {
	doc := createTestDocument([]string{
		"# Test Document",
		"",
		"This contains search term.",
		"Another line with search.",
		"Last line.",
	})
	dv := NewDocumentView(doc)
	dv.SetSize(80, 24)

	// Highlight matches at lines 3 and 4, with current match at index 0 (line 3)
	dv.HighlightSearchMatches([]int{3, 4}, 0)

	output := dv.Render()
	output = testutil.StripANSI(output)

	testutil.RequireGolden(t, output)
}

func TestDocumentView_MoveCursor(t *testing.T) {
	tests := []struct {
		name        string
		startLine   int
		delta       int
		wantLine    int
		description string
	}{
		{
			name:        "move down within bounds",
			startLine:   1,
			delta:       2,
			wantLine:    3,
			description: "moving down from line 1 by 2 should go to line 3",
		},
		{
			name:        "move down at document end (should clamp)",
			startLine:   4,
			delta:       5,
			wantLine:    5,
			description: "moving down from line 4 by 5 should clamp to max line 5",
		},
		{
			name:        "move up within bounds",
			startLine:   5,
			delta:       -2,
			wantLine:    3,
			description: "moving up from line 5 by 2 should go to line 3",
		},
		{
			name:        "move up at document start (should clamp to 1)",
			startLine:   2,
			delta:       -5,
			wantLine:    1,
			description: "moving up from line 2 by 5 should clamp to line 1",
		},
		{
			name:        "move down by 0 (no change)",
			startLine:   3,
			delta:       0,
			wantLine:    3,
			description: "moving by 0 should not change position",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := createTestDocument([]string{
				"Line 1",
				"Line 2",
				"Line 3",
				"Line 4",
				"Line 5",
			})

			dv := NewDocumentView(doc)
			dv.SetSize(80, 24)

			// Override RenderedLines to bypass glamour rendering for testing
			doc.RenderedLines = []string{
				"Line 1",
				"Line 2",
				"Line 3",
				"Line 4",
				"Line 5",
			}

			dv.cursorLine = tt.startLine

			dv.MoveCursor(tt.delta)

			if dv.cursorLine != tt.wantLine {
				t.Errorf("%s: got cursor at line %d, want %d", tt.description, dv.cursorLine, tt.wantLine)
			}
		})
	}
}

func TestDocumentView_MoveCursor_NilDocument(t *testing.T) {
	dv := NewDocumentView(nil)
	dv.SetSize(80, 24)
	dv.cursorLine = 1

	// Should not panic with nil document
	dv.MoveCursor(5)

	// Cursor should remain at original position
	if dv.cursorLine != 1 {
		t.Errorf("MoveCursor with nil document changed cursor from 1 to %d", dv.cursorLine)
	}
}

func TestDocumentView_JumpToLine(t *testing.T) {
	tests := []struct {
		name     string
		jumpTo   int
		wantLine int
	}{
		{
			name:     "jump to valid line",
			jumpTo:   3,
			wantLine: 3,
		},
		{
			name:     "jump to line 1",
			jumpTo:   1,
			wantLine: 1,
		},
		{
			name:     "jump to last line",
			jumpTo:   5,
			wantLine: 5,
		},
		{
			name:     "jump to line 0 (should clamp to 1)",
			jumpTo:   0,
			wantLine: 1,
		},
		{
			name:     "jump to negative line (should clamp to 1)",
			jumpTo:   -5,
			wantLine: 1,
		},
		{
			name:     "jump beyond document end (should clamp to max)",
			jumpTo:   100,
			wantLine: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := createTestDocument([]string{
				"Line 1",
				"Line 2",
				"Line 3",
				"Line 4",
				"Line 5",
			})

			dv := NewDocumentView(doc)
			dv.SetSize(80, 24)

			// Override RenderedLines to bypass glamour rendering for testing
			doc.RenderedLines = []string{
				"Line 1",
				"Line 2",
				"Line 3",
				"Line 4",
				"Line 5",
			}

			dv.JumpToLine(tt.jumpTo)

			if dv.cursorLine != tt.wantLine {
				t.Errorf("JumpToLine(%d) set cursor to line %d, want %d", tt.jumpTo, dv.cursorLine, tt.wantLine)
			}
		})
	}
}

func TestDocumentView_JumpToLine_NilDocument(t *testing.T) {
	dv := NewDocumentView(nil)
	dv.SetSize(80, 24)
	dv.cursorLine = 1

	// Should not panic with nil document
	dv.JumpToLine(5)

	// Cursor should remain at original position
	if dv.cursorLine != 1 {
		t.Errorf("JumpToLine with nil document changed cursor from 1 to %d", dv.cursorLine)
	}
}

func TestDocumentView_CoordinateMapping(t *testing.T) {
	dv := NewDocumentView(nil) // don't need actual document for these tests

	t.Run("mapDocToDisplay with nil mapping", func(t *testing.T) {
		// With nil mapping, document and display coordinates are the same
		got := dv.mapDocToDisplay(5, nil)
		if got != 5 {
			t.Errorf("mapDocToDisplay(5, nil) = %d, want 5", got)
		}
	})

	t.Run("mapDocToDisplay with comments inserted", func(t *testing.T) {
		// Simulate comments inserted after lines 2 and 5
		// Line 1 → Display 1
		// Line 2 → Display 2
		// [comment lines 3, 4]
		// Line 3 → Display 5
		// Line 4 → Display 6
		// Line 5 → Display 7
		// [comment lines 8, 9]
		// Line 6 → Display 10
		lineMapping := map[int]int{
			1: 1,
			2: 2,
			3: 5,
			4: 6,
			5: 7,
			6: 10,
		}

		tests := []struct {
			docLine     int
			wantDisplay int
		}{
			{1, 1},
			{2, 2},
			{3, 5}, // After comment insertion
			{5, 7},
			{6, 10},
		}

		for _, tt := range tests {
			got := dv.mapDocToDisplay(tt.docLine, lineMapping)
			if got != tt.wantDisplay {
				t.Errorf("mapDocToDisplay(%d) = %d, want %d", tt.docLine, got, tt.wantDisplay)
			}
		}
	})

	t.Run("mapDocToDisplay unmapped line fallback", func(t *testing.T) {
		lineMapping := map[int]int{
			1: 1,
			2: 2,
		}
		// Line 99 not in mapping should return itself as fallback
		got := dv.mapDocToDisplay(99, lineMapping)
		if got != 99 {
			t.Errorf("mapDocToDisplay(99) with unmapped line = %d, want 99 (fallback)", got)
		}
	})

	t.Run("mapDisplayToDoc with nil mapping", func(t *testing.T) {
		// With nil mapping, document and display coordinates are the same
		got := dv.mapDisplayToDoc(5, nil)
		if got != 5 {
			t.Errorf("mapDisplayToDoc(5, nil) = %d, want 5", got)
		}
	})

	t.Run("mapDisplayToDoc with comments inserted", func(t *testing.T) {
		lineMapping := map[int]int{
			1: 1,
			2: 2,
			3: 5,
			4: 6,
			5: 7,
		}

		tests := []struct {
			displayLine int
			wantDocLine int
		}{
			{1, 1},  // Display line 1 → Doc line 1
			{2, 2},  // Display line 2 → Doc line 2
			{5, 3},  // Display line 5 → Doc line 3
			{6, 4},  // Display line 6 → Doc line 4
			{3, 0},  // Display line 3 is a comment (not mapped) → 0
			{4, 0},  // Display line 4 is a comment (not mapped) → 0
			{99, 0}, // Display line 99 not in mapping → 0
		}

		for _, tt := range tests {
			got := dv.mapDisplayToDoc(tt.displayLine, lineMapping)
			if got != tt.wantDocLine {
				t.Errorf("mapDisplayToDoc(%d) = %d, want %d", tt.displayLine, got, tt.wantDocLine)
			}
		}
	})

	t.Run("coordinate round-trip", func(t *testing.T) {
		lineMapping := map[int]int{
			1: 1,
			2: 2,
			3: 5,
		}

		// Doc line 3 → Display line 5 → Doc line 3
		displayLine := dv.mapDocToDisplay(3, lineMapping)
		docLine := dv.mapDisplayToDoc(displayLine, lineMapping)

		if docLine != 3 {
			t.Errorf("Round-trip: doc 3 → display %d → doc %d, want 3", displayLine, docLine)
		}
	})
}
