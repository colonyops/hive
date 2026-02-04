package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// keyMsg creates a KeyMsg for testing.
func keyMsg(s string) tea.Msg {
	if len(s) == 0 {
		return tea.KeyPressMsg{}
	}
	return tea.KeyPressMsg{Text: s, Code: rune(s[0])}
}

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

func TestCommentDeletionWithConfirmation(t *testing.T) {
	doc := ReviewDocument{
		Path:    "/path/to/test.md",
		RelPath: "plans/test.md",
		Type:    DocTypePlan,
		ModTime: time.Now(),
		Content: "Line 1\nLine 2\nLine 3\nLine 4\nLine 5",
	}

	view := NewReviewView([]ReviewDocument{doc}, "", nil)
	view.SetSize(80, 24)
	view.fullScreen = true
	view.selectedDoc = &doc

	// Create a session with comments
	view.activeSession = &ReviewSession{
		ID:      "test-session",
		DocPath: doc.Path,
		Comments: []ReviewComment{
			{
				ID:          "comment-1",
				SessionID:   "test-session",
				StartLine:   2,
				EndLine:     3,
				ContextText: "Line 2\nLine 3",
				CommentText: "Test comment",
				CreatedAt:   time.Now(),
			},
		},
		CreatedAt:  time.Now(),
		ModifiedAt: time.Now(),
	}

	// Set cursor on comment line
	view.cursorLine = 2

	// Press 'd' should show confirmation modal
	view, _ = view.Update(keyMsg("d"))

	if view.confirmModal == nil {
		t.Fatal("expected confirmation modal to be shown")
	}

	if view.pendingDeleteLine != 2 {
		t.Fatalf("expected pendingDeleteLine=2, got %d", view.pendingDeleteLine)
	}

	// Press 'y' to confirm deletion
	view, _ = view.Update(keyMsg("y"))

	if view.confirmModal != nil {
		t.Error("expected confirmation modal to be closed")
	}

	if view.pendingDeleteLine != 0 {
		t.Errorf("expected pendingDeleteLine to be cleared, got %d", view.pendingDeleteLine)
	}

	// Comment should be deleted (session cleared when all comments removed)
	if view.activeSession != nil {
		t.Errorf("expected session to be cleared, got %d comments", len(view.activeSession.Comments))
	}
}

func TestCommentDeletionCancellation(t *testing.T) {
	doc := ReviewDocument{
		Path:    "/path/to/test.md",
		RelPath: "plans/test.md",
		Type:    DocTypePlan,
		ModTime: time.Now(),
		Content: "Line 1\nLine 2\nLine 3\nLine 4\nLine 5",
	}

	view := NewReviewView([]ReviewDocument{doc}, "", nil)
	view.SetSize(80, 24)
	view.fullScreen = true
	view.selectedDoc = &doc

	// Create a session with comments
	view.activeSession = &ReviewSession{
		ID:      "test-session",
		DocPath: doc.Path,
		Comments: []ReviewComment{
			{
				ID:          "comment-1",
				SessionID:   "test-session",
				StartLine:   2,
				EndLine:     3,
				ContextText: "Line 2\nLine 3",
				CommentText: "Test comment",
				CreatedAt:   time.Now(),
			},
		},
		CreatedAt:  time.Now(),
		ModifiedAt: time.Now(),
	}

	// Set cursor on comment line
	view.cursorLine = 2

	// Press 'd' should show confirmation modal
	view, _ = view.Update(keyMsg("d"))

	// Press 'n' to cancel
	view, _ = view.Update(keyMsg("n"))

	if view.confirmModal != nil {
		t.Error("expected confirmation modal to be closed")
	}

	if view.pendingDeleteLine != 0 {
		t.Error("expected pendingDeleteLine to be cleared")
	}

	// Comment should still exist
	if len(view.activeSession.Comments) != 1 {
		t.Errorf("expected 1 comment, got %d", len(view.activeSession.Comments))
	}
}

func TestReviewDiscardWithConfirmation(t *testing.T) {
	doc := ReviewDocument{
		Path:    "/path/to/test.md",
		RelPath: "plans/test.md",
		Type:    DocTypePlan,
		ModTime: time.Now(),
		Content: "Line 1\nLine 2\nLine 3\nLine 4\nLine 5",
	}

	view := NewReviewView([]ReviewDocument{doc}, "", nil)
	view.SetSize(80, 24)
	view.fullScreen = true
	view.selectedDoc = &doc

	// Create a session with multiple comments
	view.activeSession = &ReviewSession{
		ID:      "test-session",
		DocPath: doc.Path,
		Comments: []ReviewComment{
			{
				ID:          "comment-1",
				SessionID:   "test-session",
				StartLine:   2,
				EndLine:     3,
				ContextText: "Line 2\nLine 3",
				CommentText: "First comment",
				CreatedAt:   time.Now(),
			},
			{
				ID:          "comment-2",
				SessionID:   "test-session",
				StartLine:   4,
				EndLine:     5,
				ContextText: "Line 4\nLine 5",
				CommentText: "Second comment",
				CreatedAt:   time.Now(),
			},
		},
		CreatedAt:  time.Now(),
		ModifiedAt: time.Now(),
	}

	// Press 'D' should show confirmation modal
	view, _ = view.Update(keyMsg("D"))

	if view.confirmModal == nil {
		t.Fatal("expected confirmation modal to be shown")
	}

	if !view.pendingDiscard {
		t.Error("expected pendingDiscard to be true")
	}

	// Press 'y' to confirm discard
	view, cmd := view.Update(keyMsg("y"))

	if view.confirmModal != nil {
		t.Error("expected confirmation modal to be closed")
	}

	if view.pendingDiscard {
		t.Error("expected pendingDiscard to be cleared")
	}

	// Execute the discard command
	if cmd != nil {
		msg := cmd()
		if _, ok := msg.(reviewDiscardedMsg); !ok {
			t.Errorf("expected reviewDiscardedMsg, got %T", msg)
		}

		// Process the discard message
		view, _ = view.Update(msg)
	}

	// Session should be cleared
	if view.activeSession != nil {
		t.Error("expected session to be cleared after discard")
	}
}

func TestReviewDiscardCancellation(t *testing.T) {
	doc := ReviewDocument{
		Path:    "/path/to/test.md",
		RelPath: "plans/test.md",
		Type:    DocTypePlan,
		ModTime: time.Now(),
		Content: "Line 1\nLine 2\nLine 3\nLine 4\nLine 5",
	}

	view := NewReviewView([]ReviewDocument{doc}, "", nil)
	view.SetSize(80, 24)
	view.fullScreen = true
	view.selectedDoc = &doc

	// Create a session with comments
	view.activeSession = &ReviewSession{
		ID:      "test-session",
		DocPath: doc.Path,
		Comments: []ReviewComment{
			{
				ID:          "comment-1",
				SessionID:   "test-session",
				StartLine:   2,
				EndLine:     3,
				ContextText: "Line 2\nLine 3",
				CommentText: "Test comment",
				CreatedAt:   time.Now(),
			},
		},
		CreatedAt:  time.Now(),
		ModifiedAt: time.Now(),
	}

	// Press 'D' should show confirmation modal
	view, _ = view.Update(keyMsg("D"))

	// Press 'n' to cancel
	view, _ = view.Update(keyMsg("n"))

	if view.confirmModal != nil {
		t.Error("expected confirmation modal to be closed")
	}

	if view.pendingDiscard {
		t.Error("expected pendingDiscard to be cleared")
	}

	// Session should still exist
	if view.activeSession == nil {
		t.Error("expected session to still exist after cancellation")
	}

	if len(view.activeSession.Comments) != 1 {
		t.Errorf("expected 1 comment, got %d", len(view.activeSession.Comments))
	}
}

func TestReviewDiscardWithNoComments(t *testing.T) {
	doc := ReviewDocument{
		Path:    "/path/to/test.md",
		RelPath: "plans/test.md",
		Type:    DocTypePlan,
		ModTime: time.Now(),
		Content: "Line 1\nLine 2\nLine 3",
	}

	view := NewReviewView([]ReviewDocument{doc}, "", nil)
	view.SetSize(80, 24)
	view.fullScreen = true
	view.selectedDoc = &doc

	// Create a session with no comments
	view.activeSession = &ReviewSession{
		ID:         "test-session",
		DocPath:    doc.Path,
		Comments:   []ReviewComment{},
		CreatedAt:  time.Now(),
		ModifiedAt: time.Now(),
	}

	// Press 'D' should NOT show confirmation modal (no comments to discard)
	view, _ = view.Update(keyMsg("D"))

	if view.confirmModal != nil {
		t.Error("expected no confirmation modal when there are no comments")
	}

	if view.pendingDiscard {
		t.Error("expected pendingDiscard to remain false")
	}
}

func TestCommentVisualStyling(t *testing.T) {
	doc := ReviewDocument{
		Path:    "/path/to/test.md",
		RelPath: "plans/test.md",
		Type:    DocTypePlan,
		ModTime: time.Now(),
		Content: "Line 1\nLine 2\nLine 3",
	}

	view := NewReviewView([]ReviewDocument{doc}, "", nil)
	view.SetSize(80, 24)
	view.selectedDoc = &doc

	// Create a session with a comment
	view.activeSession = &ReviewSession{
		ID:      "test-session",
		DocPath: doc.Path,
		Comments: []ReviewComment{
			{
				ID:          "comment-1",
				SessionID:   "test-session",
				StartLine:   2,
				EndLine:     2,
				ContextText: "Line 2",
				CommentText: "This is a test comment",
				CreatedAt:   time.Now(),
			},
		},
		CreatedAt:  time.Now(),
		ModifiedAt: time.Now(),
	}

	// Render the document with comments
	content := "Line 1\nLine 2\nLine 3"
	rendered := view.insertCommentsInline(content)

	// Check that the rendered output contains the profile placeholder
	if !strings.Contains(rendered, "<profile>") {
		t.Error("expected rendered output to contain '<profile>' placeholder")
	}

	// Check that the comment text is present
	if !strings.Contains(rendered, "This is a test comment") {
		t.Error("expected rendered output to contain comment text")
	}

	// Check that there's increased indentation (at least 4 spaces before the styled content)
	lines := strings.Split(rendered, "\n")
	var commentLineFound bool
	for _, line := range lines {
		if strings.Contains(line, "<profile>") {
			// Check for leading spaces (indentation)
			if !strings.HasPrefix(line, "    ") {
				t.Error("expected comment line to have increased indentation (at least 4 spaces)")
			}
			commentLineFound = true
			break
		}
	}

	if !commentLineFound {
		t.Error("expected to find a comment line in rendered output")
	}
}
