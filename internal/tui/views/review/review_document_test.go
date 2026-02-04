package review

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInferDocumentType(t *testing.T) {
	tests := []struct {
		name     string
		relPath  string
		expected DocumentType
	}{
		{
			name:     "plan document",
			relPath:  "plans/2026-02-01-feature.md",
			expected: DocTypePlan,
		},
		{
			name:     "research document",
			relPath:  "research/2026-02-01-investigation.md",
			expected: DocTypeResearch,
		},
		{
			name:     "context document",
			relPath:  "context/architecture.md",
			expected: DocTypeContext,
		},
		{
			name:     "other document",
			relPath:  "notes.md",
			expected: DocTypeOther,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := inferDocumentType(tt.relPath)
			if result != tt.expected {
				t.Errorf("inferDocumentType(%q) = %v, want %v", tt.relPath, result, tt.expected)
			}
		})
	}
}

func TestDocumentTypeString(t *testing.T) {
	tests := []struct {
		docType  DocumentType
		expected string
	}{
		{DocTypePlan, "Plan"},
		{DocTypeResearch, "Research"},
		{DocTypeContext, "Context"},
		{DocTypeOther, "Other"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.docType.String()
			if result != tt.expected {
				t.Errorf("DocumentType.String() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestSortDocuments(t *testing.T) {
	now := time.Now()
	older := now.Add(-1 * time.Hour)
	newest := now.Add(1 * time.Hour)

	docs := []Document{
		{Type: DocTypeOther, ModTime: now},
		{Type: DocTypePlan, ModTime: older},
		{Type: DocTypeResearch, ModTime: newest},
		{Type: DocTypePlan, ModTime: newest},
		{Type: DocTypeContext, ModTime: now},
	}

	sortDocuments(docs)

	// Expected order:
	// 1. Plans (newest first)
	// 2. Research (newest first)
	// 3. Context (newest first)
	// 4. Other (newest first)
	expected := []struct {
		docType DocumentType
		modTime time.Time
	}{
		{DocTypePlan, newest},
		{DocTypePlan, older},
		{DocTypeResearch, newest},
		{DocTypeContext, now},
		{DocTypeOther, now},
	}

	if len(docs) != len(expected) {
		t.Fatalf("expected %d documents, got %d", len(expected), len(docs))
	}

	for i, exp := range expected {
		if docs[i].Type != exp.docType {
			t.Errorf("position %d: expected type %v, got %v", i, exp.docType, docs[i].Type)
		}
		if !docs[i].ModTime.Equal(exp.modTime) {
			t.Errorf("position %d: expected time %v, got %v", i, exp.modTime, docs[i].ModTime)
		}
	}
}

func TestDiscoverDocuments(t *testing.T) {
	// Create temporary directory structure (simulating context directory)
	tmpDir, err := os.MkdirTemp("", "hive-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create context directory structure
	plansPath := filepath.Join(tmpDir, "plans")
	researchPath := filepath.Join(tmpDir, "research")
	contextPath := filepath.Join(tmpDir, "context")

	for _, dir := range []string{plansPath, researchPath, contextPath} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// Create test files
	testFiles := []struct {
		path    string
		content string
	}{
		{filepath.Join(plansPath, "plan1.md"), "# Plan 1"},
		{filepath.Join(plansPath, "plan2.txt"), "Plan 2"},
		{filepath.Join(researchPath, "research1.md"), "# Research 1"},
		{filepath.Join(contextPath, "context1.md"), "# Context 1"},
		{filepath.Join(tmpDir, "other.md"), "# Other"},
		{filepath.Join(tmpDir, "ignored.json"), "{}"}, // Should be ignored
	}

	for _, tf := range testFiles {
		if err := os.WriteFile(tf.path, []byte(tf.content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Discover documents
	docs, err := DiscoverDocuments(tmpDir)
	if err != nil {
		t.Fatalf("DiscoverDocuments() error = %v", err)
	}

	// Verify count (should exclude .json file)
	expectedCount := 5
	if len(docs) != expectedCount {
		t.Errorf("expected %d documents, got %d", expectedCount, len(docs))
	}

	// Verify all documents have required fields
	for i, doc := range docs {
		if doc.Path == "" {
			t.Errorf("document %d: empty Path", i)
		}
		if doc.RelPath == "" {
			t.Errorf("document %d: empty RelPath", i)
		}
		if doc.ModTime.IsZero() {
			t.Errorf("document %d: zero ModTime", i)
		}
		// Verify RelPath is relative
		if filepath.IsAbs(doc.RelPath) {
			t.Errorf("document %d: RelPath should be relative, got %q", i, doc.RelPath)
		}
	}

	// Verify document types
	typeCount := make(map[DocumentType]int)
	for _, doc := range docs {
		typeCount[doc.Type]++
	}

	expectedTypes := map[DocumentType]int{
		DocTypePlan:     2,
		DocTypeResearch: 1,
		DocTypeContext:  1,
		DocTypeOther:    1,
	}

	for docType, expected := range expectedTypes {
		if typeCount[docType] != expected {
			t.Errorf("expected %d %v documents, got %d", expected, docType, typeCount[docType])
		}
	}

	// Verify documents are sorted by type
	var lastType DocumentType
	for i, doc := range docs {
		if i > 0 && doc.Type < lastType {
			t.Errorf("documents not sorted by type: position %d has type %v after %v", i, doc.Type, lastType)
		}
		lastType = doc.Type
	}
}

func TestDiscoverDocuments_NoContextDir(t *testing.T) {
	// Use a non-existent directory
	nonExistentDir := "/tmp/hive-test-nonexistent-" + time.Now().Format("20060102150405")

	docs, err := DiscoverDocuments(nonExistentDir)
	if err != nil {
		t.Fatalf("DiscoverDocuments() error = %v", err)
	}

	if len(docs) != 0 {
		t.Errorf("expected 0 documents, got %d", len(docs))
	}
}

func TestDiscoverDocuments_EmptyContextDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hive-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Empty context directory
	docs, err := DiscoverDocuments(tmpDir)
	if err != nil {
		t.Fatalf("DiscoverDocuments() error = %v", err)
	}

	if len(docs) != 0 {
		t.Errorf("expected 0 documents, got %d", len(docs))
	}
}
