package tui

import (
	"testing"
	"time"
)

func TestBuildReviewTreeItems(t *testing.T) {
	now := time.Now()

	docs := []ReviewDocument{
		{
			Path:    "/path/to/plans/plan1.md",
			RelPath: "plans/plan1.md",
			Type:    DocTypePlan,
			ModTime: now,
		},
		{
			Path:    "/path/to/plans/plan2.md",
			RelPath: "plans/plan2.md",
			Type:    DocTypePlan,
			ModTime: now.Add(-time.Hour),
		},
		{
			Path:    "/path/to/research/research1.md",
			RelPath: "research/research1.md",
			Type:    DocTypeResearch,
			ModTime: now,
		},
		{
			Path:    "/path/to/context/notes.md",
			RelPath: "context/notes.md",
			Type:    DocTypeContext,
			ModTime: now,
		},
	}

	items := BuildReviewTreeItems(docs)

	// Should have: 3 headers (Plan, Research, Context) + 4 documents = 7 items
	expectedCount := 7
	if len(items) != expectedCount {
		t.Fatalf("expected %d items, got %d", expectedCount, len(items))
	}

	// First item should be Plans header
	item0, ok := items[0].(ReviewTreeItem)
	if !ok {
		t.Fatal("item 0 is not a ReviewTreeItem")
	}
	if !item0.IsHeader || item0.HeaderName != "Plan" {
		t.Errorf("expected Plans header, got: IsHeader=%v, HeaderName=%s", item0.IsHeader, item0.HeaderName)
	}

	// Second item should be first plan document
	item1, ok := items[1].(ReviewTreeItem)
	if !ok {
		t.Fatal("item 1 is not a ReviewTreeItem")
	}
	if item1.IsHeader {
		t.Error("expected document, got header")
	}
	if item1.Document.RelPath != "plans/plan1.md" {
		t.Errorf("expected plan1.md, got %s", item1.Document.RelPath)
	}
	if item1.IsLastInType {
		t.Error("expected IsLastInType=false for first plan")
	}

	// Third item should be second plan document (last in type)
	item2, ok := items[2].(ReviewTreeItem)
	if !ok {
		t.Fatal("item 2 is not a ReviewTreeItem")
	}
	if !item2.IsLastInType {
		t.Error("expected IsLastInType=true for second plan")
	}
}

func TestReviewTreeItemFilterValue(t *testing.T) {
	header := ReviewTreeItem{
		IsHeader:   true,
		HeaderName: "Plans",
	}

	if header.FilterValue() != "" {
		t.Errorf("expected empty filter value for header, got %s", header.FilterValue())
	}

	doc := ReviewTreeItem{
		IsHeader: false,
		Document: ReviewDocument{
			RelPath: "plans/implementation.md",
		},
	}

	filterValue := doc.FilterValue()
	if filterValue != "plans/implementation.md" {
		t.Errorf("expected 'plans/implementation.md', got %s", filterValue)
	}
}

func TestNewReviewView(t *testing.T) {
	docs := []ReviewDocument{
		{
			Path:    "/path/to/test.md",
			RelPath: "plans/test.md",
			Type:    DocTypePlan,
			ModTime: time.Now(),
		},
	}

	view := NewReviewView(docs, "", nil)

	// Should not panic and should have a list
	if view.list.Items() == nil {
		t.Error("expected list items to be initialized")
	}

	// Should be able to set size
	view.SetSize(80, 24)
}

func TestDocumentWatcherIntegration(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Create review view with watcher
	view := NewReviewView([]ReviewDocument{}, tmpDir, nil)

	// View should have a watcher
	if view.watcher == nil {
		t.Error("expected watcher to be initialized")
	}

	// Init should return a command
	cmd := view.Init()
	if cmd == nil {
		t.Error("expected Init to return a watch command")
	}
}
