package tui

import (
	"testing"

	"github.com/hay-kot/hive/internal/core/config"
)

func TestHiveDocReviewCmd_Execute(t *testing.T) {
	// Create a mock model with review view
	docs := []ReviewDocument{
		{
			Path:    "/test/doc1.md",
			RelPath: ".hive/plans/doc1.md",
			Type:    DocTypePlan,
		},
		{
			Path:    "/test/doc2.md",
			RelPath: ".hive/research/doc2.md",
			Type:    DocTypeResearch,
		},
	}
	reviewView := NewReviewView(docs, "/test", nil)
	reviewView.SetSize(100, 40)

	// Create a minimal handler for testing
	handler := NewKeybindingResolver(nil, map[string]config.UserCommand{})

	m := &Model{
		activeView: ViewSessions,
		reviewView: &reviewView,
		handler:    handler,
	}

	// Execute command without argument
	cmd := HiveDocReviewCmd{Arg: ""}
	_ = cmd.Execute(m)

	// Check that view switched to review
	if m.activeView != ViewReview {
		t.Errorf("Expected active view to be ViewReview, got %v", m.activeView)
	}

	// Check that picker modal is shown
	if reviewView.pickerModal == nil {
		t.Error("Expected picker modal to be created")
	}
}

func TestOpenDocument(t *testing.T) {
	docs := []ReviewDocument{
		{
			Path:    "/test/doc1.md",
			RelPath: ".hive/plans/doc1.md",
			Type:    DocTypePlan,
		},
		{
			Path:    "/test/doc2.md",
			RelPath: ".hive/research/doc2.md",
			Type:    DocTypeResearch,
		},
	}
	reviewView := NewReviewView(docs, "/test", nil)
	reviewView.SetSize(100, 40)

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "find by full path",
			path:    "/test/doc1.md",
			wantErr: false,
		},
		{
			name:    "find by relative path",
			path:    ".hive/plans/doc1.md",
			wantErr: false,
		},
		{
			name:    "find by basename",
			path:    "doc2.md",
			wantErr: false,
		},
		{
			name:    "not found",
			path:    "nonexistent.md",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := reviewView.OpenDocument(tt.path)
			msg := cmd()

			if openMsg, ok := msg.(openDocumentMsg); ok {
				if tt.wantErr && openMsg.err == nil {
					t.Error("Expected error but got none")
				}
				if !tt.wantErr && openMsg.err != nil {
					t.Errorf("Expected no error but got: %v", openMsg.err)
				}
			} else {
				t.Errorf("Expected openDocumentMsg, got %T", msg)
			}
		})
	}
}

func TestDocumentPickerModal_FuzzyMatch(t *testing.T) {
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
			if got != tt.want {
				t.Errorf("fuzzyMatch(%q, %q) = %v, want %v", tt.target, tt.query, got, tt.want)
			}
		})
	}
}

func TestDocumentPickerModal_UpdateFilter(t *testing.T) {
	docs := []ReviewDocument{
		{RelPath: ".hive/plans/feature-a.md", Type: DocTypePlan},
		{RelPath: ".hive/plans/feature-b.md", Type: DocTypePlan},
		{RelPath: ".hive/research/api-design.md", Type: DocTypeResearch},
	}

	modal := NewDocumentPickerModal(docs, 100, 40)

	// Initially all documents should be shown
	if len(modal.filteredDocs) != 3 {
		t.Errorf("Expected 3 documents initially, got %d", len(modal.filteredDocs))
	}

	// Set filter query
	modal.searchInput.SetValue("feat")
	modal.filterQuery = "feat"
	modal.updateFilter()

	// Should match 2 documents (feature-a and feature-b)
	if len(modal.filteredDocs) != 2 {
		t.Errorf("Expected 2 documents after filtering 'feat', got %d", len(modal.filteredDocs))
	}

	// Clear filter
	modal.searchInput.SetValue("")
	modal.filterQuery = ""
	modal.updateFilter()

	// Should show all documents again
	if len(modal.filteredDocs) != 3 {
		t.Errorf("Expected 3 documents after clearing filter, got %d", len(modal.filteredDocs))
	}
}

func TestGetAllDocuments(t *testing.T) {
	docs := []ReviewDocument{
		{RelPath: "doc1.md", Type: DocTypePlan},
		{RelPath: "doc2.md", Type: DocTypeResearch},
	}

	reviewView := NewReviewView(docs, "/test", nil)

	allDocs := reviewView.getAllDocuments()

	// Should return all non-header documents
	if len(allDocs) != 2 {
		t.Errorf("Expected 2 documents, got %d", len(allDocs))
	}
}

func TestReviewView_WithPickerModal(t *testing.T) {
	docs := []ReviewDocument{
		{RelPath: "doc1.md", Type: DocTypePlan},
	}

	reviewView := NewReviewView(docs, "/test", nil)
	reviewView.SetSize(100, 40)

	// Show picker
	_ = reviewView.ShowDocumentPicker()

	if reviewView.pickerModal == nil {
		t.Error("Expected picker modal to be created")
	}

	// Verify picker has documents
	if len(reviewView.pickerModal.documents) != 1 {
		t.Errorf("Expected 1 document in picker, got %d", len(reviewView.pickerModal.documents))
	}
}

// Ensure ReviewView has pickerModal field
func TestReviewView_HasPickerModalField(t *testing.T) {
	reviewView := NewReviewView(nil, "", nil)

	// Access the field to ensure it exists
	_ = reviewView.pickerModal

	// This test passes if it compiles
}
