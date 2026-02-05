package review

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/hay-kot/hive/internal/data/db"
	"github.com/hay-kot/hive/internal/stores"
	"github.com/hay-kot/hive/internal/styles"
)

// keyMsg creates a KeyMsg for testing.
func keyMsg(s string) tea.Msg {
	if len(s) == 0 {
		return tea.KeyPressMsg{}
	}
	return tea.KeyPressMsg{Text: s, Code: rune(s[0])}
}

func TestBuildTreeItems(t *testing.T) {
	now := time.Now()

	docs := []Document{
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

	items := BuildTreeItems(docs)

	// Should have: 3 headers (Plan, Research, Context) + 4 documents = 7 items
	expectedCount := 7
	if len(items) != expectedCount {
		t.Fatalf("expected %d items, got %d", expectedCount, len(items))
	}

	// First item should be Plans header
	item0, ok := items[0].(TreeItem)
	if !ok {
		t.Fatal("item 0 is not a TreeItem")
	}
	if !item0.IsHeader || item0.HeaderName != "Plan" {
		t.Errorf("expected Plans header, got: IsHeader=%v, HeaderName=%s", item0.IsHeader, item0.HeaderName)
	}

	// Second item should be first plan document
	item1, ok := items[1].(TreeItem)
	if !ok {
		t.Fatal("item 1 is not a TreeItem")
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
	item2, ok := items[2].(TreeItem)
	if !ok {
		t.Fatal("item 2 is not a TreeItem")
	}
	if !item2.IsLastInType {
		t.Error("expected IsLastInType=true for second plan")
	}
}

func TestTreeItemFilterValue(t *testing.T) {
	header := TreeItem{
		IsHeader:   true,
		HeaderName: "Plans",
	}

	if header.FilterValue() != "" {
		t.Errorf("expected empty filter value for header, got %s", header.FilterValue())
	}

	doc := TreeItem{
		IsHeader: false,
		Document: Document{
			RelPath: "plans/implementation.md",
		},
	}

	filterValue := doc.FilterValue()
	if filterValue != "plans/implementation.md" {
		t.Errorf("expected 'plans/implementation.md', got %s", filterValue)
	}
}

func TestNew(t *testing.T) {
	docs := []Document{
		{
			Path:    "/path/to/test.md",
			RelPath: "plans/test.md",
			Type:    DocTypePlan,
			ModTime: time.Now(),
		},
	}

	view := New(docs, "", nil)

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
	view := New([]Document{}, tmpDir, nil)

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
	doc := Document{
		Path:    "/path/to/test.md",
		RelPath: "plans/test.md",
		Type:    DocTypePlan,
		ModTime: time.Now(),
		Content: "Line 1\nLine 2\nLine 3\nLine 4\nLine 5",
	}

	view := New([]Document{doc}, "", nil)
	view.SetSize(80, 24)
	view.fullScreen = true
	view.selectedDoc = &doc

	// Create a session with comments
	view.activeSession = &Session{
		ID:      "test-session",
		DocPath: doc.Path,
		Comments: []Comment{
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
	doc := Document{
		Path:    "/path/to/test.md",
		RelPath: "plans/test.md",
		Type:    DocTypePlan,
		ModTime: time.Now(),
		Content: "Line 1\nLine 2\nLine 3\nLine 4\nLine 5",
	}

	view := New([]Document{doc}, "", nil)
	view.SetSize(80, 24)
	view.fullScreen = true
	view.selectedDoc = &doc

	// Create a session with comments
	view.activeSession = &Session{
		ID:      "test-session",
		DocPath: doc.Path,
		Comments: []Comment{
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
	doc := Document{
		Path:    "/path/to/test.md",
		RelPath: "plans/test.md",
		Type:    DocTypePlan,
		ModTime: time.Now(),
		Content: "Line 1\nLine 2\nLine 3\nLine 4\nLine 5",
	}

	view := New([]Document{doc}, "", nil)
	view.SetSize(80, 24)
	view.fullScreen = true
	view.selectedDoc = &doc

	// Create a session with multiple comments
	view.activeSession = &Session{
		ID:      "test-session",
		DocPath: doc.Path,
		Comments: []Comment{
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
	doc := Document{
		Path:    "/path/to/test.md",
		RelPath: "plans/test.md",
		Type:    DocTypePlan,
		ModTime: time.Now(),
		Content: "Line 1\nLine 2\nLine 3\nLine 4\nLine 5",
	}

	view := New([]Document{doc}, "", nil)
	view.SetSize(80, 24)
	view.fullScreen = true
	view.selectedDoc = &doc

	// Create a session with comments
	view.activeSession = &Session{
		ID:      "test-session",
		DocPath: doc.Path,
		Comments: []Comment{
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
	doc := Document{
		Path:    "/path/to/test.md",
		RelPath: "plans/test.md",
		Type:    DocTypePlan,
		ModTime: time.Now(),
		Content: "Line 1\nLine 2\nLine 3",
	}

	view := New([]Document{doc}, "", nil)
	view.SetSize(80, 24)
	view.fullScreen = true
	view.selectedDoc = &doc

	// Create a session with no comments
	view.activeSession = &Session{
		ID:         "test-session",
		DocPath:    doc.Path,
		Comments:   []Comment{},
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
	doc := Document{
		Path:    "/path/to/test.md",
		RelPath: "plans/test.md",
		Type:    DocTypePlan,
		ModTime: time.Now(),
		Content: "Line 1\nLine 2\nLine 3",
	}

	view := New([]Document{doc}, "", nil)
	view.SetSize(80, 24)
	view.selectedDoc = &doc

	// Create a session with a comment
	view.activeSession = &Session{
		ID:      "test-session",
		DocPath: doc.Path,
		Comments: []Comment{
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
	rendered, _ := view.insertCommentsInline(content)

	// Check that the rendered output contains the profile placeholder
	if !strings.Contains(rendered, styles.IconProfile) {
		t.Errorf("expected rendered output to contain '%s' placeholder", styles.IconProfile)
	}

	// Check that the comment text is present
	if !strings.Contains(rendered, "This is a test comment") {
		t.Error("expected rendered output to contain comment text")
	}

	// Check that there's increased indentation (at least 4 spaces before the styled content)
	lines := strings.Split(rendered, "\n")
	var commentLineFound bool
	for _, line := range lines {
		if strings.Contains(line, styles.IconProfile) {
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

func TestLineMappingWithComments(t *testing.T) {
	doc := Document{
		Path:    "/path/to/test.md",
		RelPath: "plans/test.md",
		Type:    DocTypePlan,
		ModTime: time.Now(),
		Content: "Line 1\nLine 2\nLine 3\nLine 4\nLine 5",
	}

	view := New([]Document{doc}, "", nil)
	view.selectedDoc = &doc

	// Create a session with comments on lines 2 and 4
	view.activeSession = &Session{
		ID:      "test-session",
		DocPath: doc.Path,
		Comments: []Comment{
			{
				ID:          "comment-1",
				SessionID:   "test-session",
				StartLine:   2,
				EndLine:     2,
				ContextText: "Line 2",
				CommentText: "Comment on line 2",
				CreatedAt:   time.Now(),
			},
			{
				ID:          "comment-2",
				SessionID:   "test-session",
				StartLine:   4,
				EndLine:     4,
				ContextText: "Line 4",
				CommentText: "Comment on line 4",
				CreatedAt:   time.Now(),
			},
		},
		CreatedAt:  time.Now(),
		ModifiedAt: time.Now(),
	}

	// Insert comments and get line mapping
	content := "Line 1\nLine 2\nLine 3\nLine 4\nLine 5"
	_, lineMapping := view.insertCommentsInline(content)

	// Verify line mapping
	// Expected mapping:
	// Doc line 1 -> Display line 1
	// Doc line 2 -> Display line 2
	// [comment after line 2]
	// Doc line 3 -> Display line 4
	// Doc line 4 -> Display line 5
	// [comment after line 4]
	// Doc line 5 -> Display line 7

	tests := []struct {
		docLine     int
		displayLine int
	}{
		{1, 1}, // Line 1 - no comments before
		{2, 2}, // Line 2 - no comments before
		{3, 4}, // Line 3 - 1 comment inserted before (after line 2)
		{4, 5}, // Line 4 - 1 comment inserted before
		{5, 7}, // Line 5 - 2 comments inserted before
	}

	for _, tt := range tests {
		got := lineMapping[tt.docLine]
		if got != tt.displayLine {
			t.Errorf("lineMapping[%d] = %d, want %d", tt.docLine, got, tt.displayLine)
		}
	}

	// Test mapDocToDisplay helper
	for _, tt := range tests {
		got := view.mapDocToDisplay(tt.docLine, lineMapping)
		if got != tt.displayLine {
			t.Errorf("mapDocToDisplay(%d) = %d, want %d", tt.docLine, got, tt.displayLine)
		}
	}

	// Test mapDisplayToDoc helper (reverse mapping)
	for _, tt := range tests {
		got := view.mapDisplayToDoc(tt.displayLine, lineMapping)
		if got != tt.docLine {
			t.Errorf("mapDisplayToDoc(%d) = %d, want %d", tt.displayLine, got, tt.docLine)
		}
	}

	// Test that comment lines map back to 0 (not a document line)
	commentDisplayLines := []int{3, 6} // Lines where comments are inserted
	for _, displayLine := range commentDisplayLines {
		got := view.mapDisplayToDoc(displayLine, lineMapping)
		if got != 0 {
			t.Errorf("mapDisplayToDoc(%d) = %d, want 0 (comment line, not a doc line)", displayLine, got)
		}
	}
}

func TestView_WithPickerModal(t *testing.T) {
	docs := []Document{
		{RelPath: "doc1.md", Type: DocTypePlan},
	}

	reviewView := New(docs, "/test", nil)
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

func TestView_HasPickerModalField(t *testing.T) {
	reviewView := New(nil, "", nil)

	// Access the field to ensure it exists
	_ = reviewView.pickerModal

	// This test passes if it compiles
}

// TestScrollVisibilityWithComments verifies that ensureCursorVisible uses display coordinates
// when comments are inserted inline, ensuring the cursor scrolls to the correct position.
func TestScrollVisibilityWithComments(t *testing.T) {
	// Create a document with many lines
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = strings.Repeat("x", 40) // 40 chars per line
	}
	content := strings.Join(lines, "\n")

	doc := Document{
		Path:          "/path/to/test.md",
		RelPath:       "test.md",
		Type:          DocTypePlan,
		ModTime:       time.Now(),
		Content:       content,
		RenderedLines: lines,
	}

	view := New([]Document{doc}, "", nil)
	view.SetSize(80, 10) // Small height to force scrolling
	view.fullScreen = true
	view.selectedDoc = &doc

	// Insert comments before line 5 (will push line 5 down in display)
	view.activeSession = &Session{
		ID:      "test-session",
		DocPath: doc.Path,
		Comments: []Comment{
			{
				ID:          "c1",
				SessionID:   "test-session",
				StartLine:   3,
				EndLine:     3,
				ContextText: lines[2],
				CommentText: "Comment 1",
				CreatedAt:   time.Now(),
			},
			{
				ID:          "c2",
				SessionID:   "test-session",
				StartLine:   4,
				EndLine:     4,
				ContextText: lines[3],
				CommentText: "Comment 2",
				CreatedAt:   time.Now(),
			},
		},
		CreatedAt:  time.Now(),
		ModifiedAt: time.Now(),
	}

	// Build line mapping by rendering with comments
	rendered := content
	rendered, lineMapping := view.insertCommentsInline(rendered)
	view.lineMapping = lineMapping
	view.viewport.SetContent(rendered)

	// Set cursor to doc line 5 (which should be at display line 7 due to 2 inserted comments)
	view.cursorLine = 5
	displayLine := view.mapDocToDisplay(5, lineMapping)
	if displayLine != 7 {
		t.Fatalf("expected doc line 5 to map to display line 7, got %d", displayLine)
	}

	// Call ensureCursorVisible
	view.ensureCursorVisible()

	// Verify viewport scrolled to show the display line, not the document line
	offset := view.viewport.YOffset()
	visibleHeight := view.viewport.VisibleLineCount() - 1 // -1 for status bar

	// Display line 7 should be visible within the viewport
	if displayLine < offset+1 || displayLine > offset+visibleHeight {
		t.Errorf("display line %d not visible in viewport (offset=%d, visibleHeight=%d)",
			displayLine, offset, visibleHeight)
	}
}

// TestJumpToMatchWithComments verifies that jumpToMatch scrolls to the correct
// display line when comments are inserted before the match.
func TestJumpToMatchWithComments(t *testing.T) {
	// Create a document with search-able content
	lines := []string{
		"Line 1",
		"Line 2",
		"Line 3 with keyword",
		"Line 4",
		"Line 5",
		"Line 6",
		"Line 7 with keyword",
		"Line 8",
	}
	content := strings.Join(lines, "\n")

	doc := Document{
		Path:          "/path/to/test.md",
		RelPath:       "test.md",
		Type:          DocTypePlan,
		ModTime:       time.Now(),
		Content:       content,
		RenderedLines: lines,
	}

	view := New([]Document{doc}, "", nil)
	view.SetSize(80, 10)
	view.fullScreen = true
	view.selectedDoc = &doc

	// Insert comments before the first search match (line 3)
	view.activeSession = &Session{
		ID:      "test-session",
		DocPath: doc.Path,
		Comments: []Comment{
			{
				ID:          "c1",
				SessionID:   "test-session",
				StartLine:   1,
				EndLine:     1,
				ContextText: lines[0],
				CommentText: "Comment on line 1",
				CreatedAt:   time.Now(),
			},
			{
				ID:          "c2",
				SessionID:   "test-session",
				StartLine:   2,
				EndLine:     2,
				ContextText: lines[1],
				CommentText: "Comment on line 2",
				CreatedAt:   time.Now(),
			},
		},
		CreatedAt:  time.Now(),
		ModifiedAt: time.Now(),
	}

	// Build line mapping
	rendered := content
	rendered, lineMapping := view.insertCommentsInline(rendered)
	view.lineMapping = lineMapping
	view.viewport.SetContent(rendered)

	// Set up search matches (lines 3 and 7)
	view.searchMatches = []int{3, 7}
	view.searchMatchIndex = 0

	// Jump to first match (doc line 3)
	view.jumpToMatch(3)

	// Doc line 3 should map to display line 5 (due to 2 comments inserted before it)
	displayLine := view.mapDocToDisplay(3, lineMapping)
	if displayLine != 5 {
		t.Fatalf("expected doc line 3 to map to display line 5, got %d", displayLine)
	}

	// Verify cursor is at the correct document line
	if view.cursorLine != 3 {
		t.Errorf("expected cursor at doc line 3, got %d", view.cursorLine)
	}

	// Verify viewport scrolled to make display line visible
	offset := view.viewport.YOffset()
	visibleHeight := view.viewport.VisibleLineCount() - 1 // -1 for status bar

	if displayLine < offset+1 || displayLine > offset+visibleHeight {
		t.Errorf("display line %d not visible after jumpToMatch (offset=%d, visibleHeight=%d)",
			displayLine, offset, visibleHeight)
	}
}

// TestBuildDisplayToDocMap verifies the reverse mapping helper function.
func TestBuildDisplayToDocMap(t *testing.T) {
	// Create a line mapping: doc -> display
	lineMapping := map[int]int{
		1: 1,
		2: 2,
		3: 4, // Comment inserted at line 3 (display line 3)
		4: 5,
		5: 7, // Comment inserted at line 5 (display line 6)
	}

	// Build reverse map
	displayToDoc := buildDisplayToDocMap(lineMapping)

	// Verify reverse mapping
	expectedReverse := map[int]int{
		1: 1,
		2: 2,
		4: 3,
		5: 4,
		7: 5,
	}

	if len(displayToDoc) != len(expectedReverse) {
		t.Fatalf("expected %d entries in reverse map, got %d", len(expectedReverse), len(displayToDoc))
	}

	for displayLine, expectedDocLine := range expectedReverse {
		if docLine, ok := displayToDoc[displayLine]; !ok {
			t.Errorf("display line %d missing from reverse map", displayLine)
		} else if docLine != expectedDocLine {
			t.Errorf("displayToDoc[%d] = %d, want %d", displayLine, docLine, expectedDocLine)
		}
	}

	// Verify comment lines (3, 6) are NOT in the reverse map
	commentDisplayLines := []int{3, 6}
	for _, displayLine := range commentDisplayLines {
		if _, ok := displayToDoc[displayLine]; ok {
			t.Errorf("comment display line %d should not be in reverse map", displayLine)
		}
	}
}

// TestReverseMappingCorrectness verifies that the reverse mapping correctly
// identifies comment lines vs document lines.
func TestReverseMappingCorrectness(t *testing.T) {
	doc := Document{
		Path:    "/path/to/test.md",
		RelPath: "test.md",
		Type:    DocTypePlan,
		ModTime: time.Now(),
		Content: "Line 1\nLine 2\nLine 3\nLine 4\nLine 5",
	}

	view := New([]Document{doc}, "", nil)
	view.selectedDoc = &doc

	// Create session with comments
	view.activeSession = &Session{
		ID:      "test-session",
		DocPath: doc.Path,
		Comments: []Comment{
			{
				ID:          "c1",
				SessionID:   "test-session",
				StartLine:   2,
				EndLine:     2,
				ContextText: "Line 2",
				CommentText: "First comment",
				CreatedAt:   time.Now(),
			},
			{
				ID:          "c2",
				SessionID:   "test-session",
				StartLine:   4,
				EndLine:     4,
				ContextText: "Line 4",
				CommentText: "Second comment",
				CreatedAt:   time.Now(),
			},
		},
		CreatedAt:  time.Now(),
		ModifiedAt: time.Now(),
	}

	// Build line mapping
	content := "Line 1\nLine 2\nLine 3\nLine 4\nLine 5"
	_, lineMapping := view.insertCommentsInline(content)

	// Build reverse map
	displayToDoc := buildDisplayToDocMap(lineMapping)

	// Test document lines map correctly
	tests := []struct {
		displayLine int
		wantDocLine int
		isComment   bool
	}{
		{1, 1, false}, // Doc line 1
		{2, 2, false}, // Doc line 2
		{3, 0, true},  // Comment after line 2
		{4, 3, false}, // Doc line 3
		{5, 4, false}, // Doc line 4
		{6, 0, true},  // Comment after line 4
		{7, 5, false}, // Doc line 5
	}

	for _, tt := range tests {
		docLine, ok := displayToDoc[tt.displayLine]
		if tt.isComment {
			if ok {
				t.Errorf("display line %d should be comment (not in map), but got doc line %d",
					tt.displayLine, docLine)
			}
		} else {
			if !ok {
				t.Errorf("display line %d should map to doc line %d, but not found in map",
					tt.displayLine, tt.wantDocLine)
			} else if docLine != tt.wantDocLine {
				t.Errorf("displayToDoc[%d] = %d, want %d",
					tt.displayLine, docLine, tt.wantDocLine)
			}
		}
	}

	// Verify nil lineMapping returns nil
	nilResult := buildDisplayToDocMap(nil)
	if nilResult != nil {
		t.Error("buildDisplayToDocMap(nil) should return nil")
	}
}

// TestFinalizedSessionsNotReloaded verifies that finalized sessions are not
// loaded as active sessions when opening a document.
func TestFinalizedSessionsNotReloaded(t *testing.T) {
	// This test requires a real store with database persistence
	// to test the session finalization behavior
	tmpDir := t.TempDir()

	// Create a test database
	database, err := db.Open(tmpDir, db.DefaultOpenOptions())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			t.Errorf("failed to close database: %v", err)
		}
	}()

	// Create a test store
	store := stores.NewReviewStore(database)

	// Create a test document
	docPath := filepath.Join(tmpDir, "test.md")
	content := "Line 1\nLine 2\nLine 3"
	if err := os.WriteFile(docPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	doc := Document{
		Path:    docPath,
		RelPath: "test.md",
		Type:    DocTypePlan,
		ModTime: time.Now(),
		Content: content,
	}

	// Create review view with the store
	view := New([]Document{doc}, tmpDir, store)
	view.SetSize(80, 24)

	// Load document and create a session with a comment
	view.loadDocument(&doc)

	// Create a comment (which will create a session)
	view.selectionStart = 1
	view.cursorLine = 2
	view.selectionMode = true
	view.addComment("Test comment before finalization")

	if view.activeSession == nil {
		t.Fatal("expected active session after adding comment")
	}

	sessionID := view.activeSession.ID

	// Finalize the session
	ctx := context.Background()
	if err := store.FinalizeSession(ctx, sessionID); err != nil {
		t.Fatalf("failed to finalize session: %v", err)
	}

	// Clear active session (simulating what happens after finalization)
	view.activeSession = nil

	// Reload the document - should NOT reload the finalized session
	view.loadDocument(&doc)

	// Verify the finalized session was NOT loaded
	if view.activeSession != nil {
		t.Errorf("expected activeSession to be nil after reloading with finalized session, but got session ID: %s",
			view.activeSession.ID)
	}
}

// TestCtrlDUWithComments verifies that ctrl+d and ctrl+u correctly handle
// display-to-document coordinate mapping when comments are inserted inline.
func TestCtrlDUWithComments(t *testing.T) {
	// Create a document with many lines
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = fmt.Sprintf("Line %d", i+1)
	}
	content := strings.Join(lines, "\n")

	doc := Document{
		Path:          "/path/to/test.md",
		RelPath:       "test.md",
		Type:          DocTypePlan,
		ModTime:       time.Now(),
		Content:       content,
		RenderedLines: lines,
	}

	view := New([]Document{doc}, "", nil)
	view.SetSize(80, 10) // Small height to force scrolling
	view.fullScreen = true
	view.selectedDoc = &doc

	// Insert comments before several lines
	view.activeSession = &Session{
		ID:      "test-session",
		DocPath: doc.Path,
		Comments: []Comment{
			{
				ID:          "c1",
				SessionID:   "test-session",
				StartLine:   5,
				EndLine:     5,
				ContextText: lines[4],
				CommentText: "Comment 1",
				CreatedAt:   time.Now(),
			},
			{
				ID:          "c2",
				SessionID:   "test-session",
				StartLine:   10,
				EndLine:     10,
				ContextText: lines[9],
				CommentText: "Comment 2",
				CreatedAt:   time.Now(),
			},
			{
				ID:          "c3",
				SessionID:   "test-session",
				StartLine:   15,
				EndLine:     15,
				ContextText: lines[14],
				CommentText: "Comment 3",
				CreatedAt:   time.Now(),
			},
		},
		CreatedAt:  time.Now(),
		ModifiedAt: time.Now(),
	}

	// Build line mapping by rendering with comments
	rendered := content
	rendered, lineMapping := view.insertCommentsInline(rendered)
	view.lineMapping = lineMapping
	view.viewport.SetContent(rendered)

	// Set initial cursor position
	view.cursorLine = 1

	// Simulate ctrl+d (half page down)
	view.viewport.HalfPageDown()
	displayLine := view.viewport.YOffset() + view.viewport.VisibleLineCount()/2
	view.cursorLine = view.mapDisplayToDoc(displayLine, view.lineMapping)
	if view.cursorLine == 0 && view.selectedDoc != nil {
		view.cursorLine = 1
	}

	// Verify cursor is at a valid document line (not 0 or beyond doc length)
	if view.cursorLine == 0 {
		t.Errorf("ctrl+d mapped to comment line (0), should map to valid document line")
	}
	if view.cursorLine > len(lines) {
		t.Errorf("ctrl+d set cursor beyond document length: %d > %d", view.cursorLine, len(lines))
	}

	// Store cursor position after ctrl+d
	cursorAfterDown := view.cursorLine

	// Simulate ctrl+u (half page up)
	view.viewport.HalfPageUp()
	displayLine = view.viewport.YOffset() + view.viewport.VisibleLineCount()/2
	view.cursorLine = view.mapDisplayToDoc(displayLine, view.lineMapping)
	if view.cursorLine == 0 && view.selectedDoc != nil {
		view.cursorLine = 1
	}

	// Verify cursor is at a valid document line
	if view.cursorLine == 0 {
		t.Errorf("ctrl+u mapped to comment line (0), should map to valid document line")
	}
	if view.cursorLine > len(lines) {
		t.Errorf("ctrl+u set cursor beyond document length: %d > %d", view.cursorLine, len(lines))
	}

	// Cursor should have moved up from the previous position
	if view.cursorLine >= cursorAfterDown {
		t.Errorf("ctrl+u should move cursor up: before=%d, after=%d", cursorAfterDown, view.cursorLine)
	}
}

func TestFinalizationModal_IntegrationWithView(t *testing.T) {
	// Create a view with a finalization modal active
	docs := []Document{
		{
			Path:    "/test/doc.md",
			RelPath: "doc.md",
			Type:    DocTypePlan,
		},
	}
	v := New(docs, "/test", nil)
	v.SetSize(100, 40)

	// Manually set up the finalization modal state (simulating pressing 'f')
	v.fullScreen = true
	v.selectedDoc = &docs[0]
	modal := NewFinalizationModal("test feedback", 100, 40)
	v.finalizationModal = &modal

	// Verify initial state
	if v.finalizationModal.selectedIdx != 0 {
		t.Errorf("Initial selectedIdx should be 0, got %d", v.finalizationModal.selectedIdx)
	}

	// Send 'j' key to view - should be forwarded to modal (stays at 0 with single option)
	jKey := keyMsg("j")
	v, _ = v.Update(jKey)

	// Verify modal received the key
	if v.finalizationModal == nil {
		t.Fatal("finalizationModal should not be nil")
	}
	// With single option, selectedIdx stays at 0
	if v.finalizationModal.selectedIdx != 0 {
		t.Errorf("With single option, selectedIdx should remain 0, got %d", v.finalizationModal.selectedIdx)
	}

	// Test enter key confirms the modal
	enterKey := keyMsg("enter")
	v, _ = v.Update(enterKey)
	// Note: After confirmation, finalizationModal might be nil (handled by view)
}
