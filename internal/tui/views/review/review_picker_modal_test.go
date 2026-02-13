package review

import (
	"testing"
	"time"

	"github.com/hay-kot/hive/internal/tui/testutil"
	"github.com/stretchr/testify/assert"
)

func TestFuzzyMatch(t *testing.T) {
	tests := []struct {
		target string
		query  string
		want   bool
	}{
		{"hello.md", "helo", true},
		{"hello.md", "h.md", true},
		{"hello.md", "md", true},
		{"hello.md", "xyz", false},
		{".hive/plans/2026-01-01-feature.md", "26feat", true},
		{".hive/research/api-design.md", "api", true},
		{".hive/research/api-design.md", "design", true},
	}

	for _, tt := range tests {
		t.Run(tt.target+"_"+tt.query, func(t *testing.T) {
			got := fuzzyMatch(tt.target, tt.query)
			assert.Equal(t, tt.want, got, "fuzzyMatch(%q, %q) = %v, want %v", tt.target, tt.query, got, tt.want)
		})
	}
}

func TestDocumentPickerModal_UpdateFilter(t *testing.T) {
	docs := []Document{
		{RelPath: ".hive/plans/feature-a.md", Type: DocumentTypePlan},
		{RelPath: ".hive/plans/feature-b.md", Type: DocumentTypePlan},
		{RelPath: ".hive/research/api-design.md", Type: DocumentTypeResearch},
	}

	modal := NewDocumentPickerModal(docs, 100, 40, nil)

	// Initially all documents should be shown
	assert.Len(t, modal.filteredDocs, 3, "Expected 3 documents initially, got %d", len(modal.filteredDocs))

	// Set filter query
	modal.searchInput.SetValue("feat")
	modal.filterQuery = "feat"
	modal.updateFilter()

	// Should match 2 documents (feature-a and feature-b)
	assert.Len(t, modal.filteredDocs, 2, "Expected 2 documents after filtering 'feat', got %d", len(modal.filteredDocs))

	// Clear filter
	modal.searchInput.SetValue("")
	modal.filterQuery = ""
	modal.updateFilter()

	// Should show all documents again
	assert.Len(t, modal.filteredDocs, 3, "Expected 3 documents after clearing filter, got %d", len(modal.filteredDocs))
}

func TestDocumentPickerModal_View(t *testing.T) {
	// Create picker with 3 documents (plan, research, context)
	docs := []Document{
		{
			Path:    "/test/.hive/plans/feature-a.md",
			RelPath: ".hive/plans/feature-a.md",
			Type:    DocumentTypePlan,
			ModTime: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		},
		{
			Path:    "/test/.hive/research/api-design.md",
			RelPath: ".hive/research/api-design.md",
			Type:    DocumentTypeResearch,
			ModTime: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		},
		{
			Path:    "/test/.hive/context/overview.md",
			RelPath: ".hive/context/overview.md",
			Type:    DocumentTypeContext,
			ModTime: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		},
	}

	modal := NewDocumentPickerModal(docs, 80, 24, nil)
	output := modal.View()
	output = testutil.StripANSI(output)

	testutil.RequireGolden(t, output)
}

func TestDocumentPickerModal_ViewWithFilter(t *testing.T) {
	// Create picker with documents that can be filtered
	docs := []Document{
		{
			Path:    "/test/.hive/plans/feature-a.md",
			RelPath: ".hive/plans/feature-a.md",
			Type:    DocumentTypePlan,
			ModTime: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		},
		{
			Path:    "/test/.hive/plans/feature-a-spec.md",
			RelPath: ".hive/plans/feature-a-spec.md",
			Type:    DocumentTypePlan,
			ModTime: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		},
		{
			Path:    "/test/.hive/plans/feature-b.md",
			RelPath: ".hive/plans/feature-b.md",
			Type:    DocumentTypePlan,
			ModTime: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		},
		{
			Path:    "/test/.hive/research/api-design.md",
			RelPath: ".hive/research/api-design.md",
			Type:    DocumentTypeResearch,
			ModTime: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		},
	}

	modal := NewDocumentPickerModal(docs, 80, 24, nil)
	// Apply filter to show only 'feature-a' documents
	modal.searchInput.SetValue("feature-a")
	modal.filterQuery = "feature-a"
	modal.updateFilter()

	output := modal.View()
	output = testutil.StripANSI(output)

	testutil.RequireGolden(t, output)
}
