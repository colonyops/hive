package review

import (
	"strings"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/tui/testutil"
)

func TestView_ListMode(t *testing.T) {
	// Create view with 2 documents in list mode
	docs := []Document{
		{
			Path:    "/test/.hive/plans/feature-a.md",
			RelPath: ".hive/plans/feature-a.md",
			Type:    DocTypePlan,
			ModTime: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
			Content: "# Feature A\n\nImplementation plan for feature A.",
		},
		{
			Path:    "/test/.hive/research/api-design.md",
			RelPath: ".hive/research/api-design.md",
			Type:    DocTypeResearch,
			ModTime: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
			Content: "# API Design\n\nResearch notes for API design.",
		},
	}

	view := New(docs, "", nil, 0)
	view.SetSize(80, 24)

	output := view.View()
	output = testutil.StripANSI(output)

	testutil.RequireGolden(t, output)
}

func TestView_DocumentMode(t *testing.T) {
	// Create view with document loaded in fullscreen mode
	testContent := strings.Join([]string{
		"# Test Document",
		"",
		"This is a test document with multiple lines.",
		"Line 4 has some content.",
		"Line 5 has more content.",
		"",
		"## Section 2",
		"",
		"Additional content in section 2.",
		"Last line of the document.",
	}, "\n")

	doc := Document{
		Path:    "/test/document.md",
		RelPath: "test/document.md",
		Type:    DocTypePlan,
		ModTime: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		Content: testContent,
	}

	view := New([]Document{doc}, "", nil, 0)
	view.SetSize(80, 24)

	// Load the document to enter fullscreen mode
	view.loadDocument(&doc)

	output := view.View()
	output = testutil.StripANSI(output)

	testutil.RequireGolden(t, output)
}
