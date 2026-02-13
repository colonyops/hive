package review

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			expected: DocumentTypePlan,
		},
		{
			name:     "research document",
			relPath:  "research/2026-02-01-investigation.md",
			expected: DocumentTypeResearch,
		},
		{
			name:     "context document",
			relPath:  "context/architecture.md",
			expected: DocumentTypeContext,
		},
		{
			name:     "other document",
			relPath:  "notes.md",
			expected: DocumentTypeOther,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := inferDocumentType(tt.relPath)
			assert.Equal(t, tt.expected, result, "inferDocumentType(%q) = %v, want %v", tt.relPath, result, tt.expected)
		})
	}
}

func TestDocumentTypeString(t *testing.T) {
	tests := []struct {
		docType  DocumentType
		expected string
	}{
		{DocumentTypePlan, "Plan"},
		{DocumentTypeResearch, "Research"},
		{DocumentTypeContext, "Context"},
		{DocumentTypeOther, "Other"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.docType.DisplayName()
			assert.Equal(t, tt.expected, result, "DocumentType.String() = %q, want %q", result, tt.expected)
		})
	}
}

func TestSortDocuments(t *testing.T) {
	now := time.Now()
	older := now.Add(-1 * time.Hour)
	newest := now.Add(1 * time.Hour)

	docs := []Document{
		{Type: DocumentTypeOther, ModTime: now},
		{Type: DocumentTypePlan, ModTime: older},
		{Type: DocumentTypeResearch, ModTime: newest},
		{Type: DocumentTypePlan, ModTime: newest},
		{Type: DocumentTypeContext, ModTime: now},
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
		{DocumentTypePlan, newest},
		{DocumentTypePlan, older},
		{DocumentTypeResearch, newest},
		{DocumentTypeContext, now},
		{DocumentTypeOther, now},
	}

	require.Len(t, docs, len(expected), "expected %d documents, got %d", len(expected), len(docs))

	for i, exp := range expected {
		assert.Equal(t, exp.docType, docs[i].Type, "position %d: expected type %v, got %v", i, exp.docType, docs[i].Type)
		assert.True(t, docs[i].ModTime.Equal(exp.modTime), "position %d: expected time %v, got %v", i, exp.modTime, docs[i].ModTime)
	}
}

func TestDiscoverDocuments(t *testing.T) {
	// Create temporary directory structure (simulating context directory)
	tmpDir, err := os.MkdirTemp("", "hive-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create context directory structure
	plansPath := filepath.Join(tmpDir, "plans")
	researchPath := filepath.Join(tmpDir, "research")
	contextPath := filepath.Join(tmpDir, "context")

	for _, dir := range []string{plansPath, researchPath, contextPath} {
		require.NoError(t, os.MkdirAll(dir, 0o755))
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
		require.NoError(t, os.WriteFile(tf.path, []byte(tf.content), 0o644))
	}

	// Discover documents
	docs, err := DiscoverDocuments(tmpDir)
	require.NoError(t, err, "DiscoverDocuments() error")

	// Verify count (should exclude .json file)
	expectedCount := 5
	assert.Len(t, docs, expectedCount, "expected %d documents, got %d", expectedCount, len(docs))

	// Verify all documents have required fields
	for i, doc := range docs {
		assert.NotEmpty(t, doc.Path, "document %d: empty Path", i)
		assert.NotEmpty(t, doc.RelPath, "document %d: empty RelPath", i)
		assert.False(t, doc.ModTime.IsZero(), "document %d: zero ModTime", i)
		// Verify RelPath is relative
		assert.False(t, filepath.IsAbs(doc.RelPath), "document %d: RelPath should be relative, got %q", i, doc.RelPath)
	}

	// Verify document types
	typeCount := make(map[DocumentType]int)
	for _, doc := range docs {
		typeCount[doc.Type]++
	}

	expectedTypes := map[DocumentType]int{
		DocumentTypePlan:     2,
		DocumentTypeResearch: 1,
		DocumentTypeContext:  1,
		DocumentTypeOther:    1,
	}

	for docType, expected := range expectedTypes {
		assert.Equal(t, expected, typeCount[docType], "expected %d %v documents, got %d", expected, docType, typeCount[docType])
	}

	// Verify documents are sorted by type (using priority)
	lastPriority := -1
	for i, doc := range docs {
		if i > 0 {
			assert.GreaterOrEqual(t, doc.Type.priority(), lastPriority, "documents not sorted by type: position %d has type %v (priority %d) after priority %d", i, doc.Type, doc.Type.priority(), lastPriority)
		}
		lastPriority = doc.Type.priority()
	}
}

func TestDiscoverDocuments_NoContextDir(t *testing.T) {
	// Use a non-existent directory
	nonExistentDir := "/tmp/hive-test-nonexistent-" + time.Now().Format("20060102150405")

	docs, err := DiscoverDocuments(nonExistentDir)
	require.NoError(t, err, "DiscoverDocuments() error")
	assert.Empty(t, docs, "expected 0 documents, got %d", len(docs))
}

func TestDiscoverDocuments_EmptyContextDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hive-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Empty context directory
	docs, err := DiscoverDocuments(tmpDir)
	require.NoError(t, err, "DiscoverDocuments() error")
	assert.Empty(t, docs, "expected 0 documents, got %d", len(docs))
}
