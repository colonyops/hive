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
