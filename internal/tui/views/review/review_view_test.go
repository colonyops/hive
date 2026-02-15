package review

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/data/db"
	"github.com/colonyops/hive/internal/data/stores"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.Len(t, items, expectedCount, "expected %d items, got %d", expectedCount, len(items))

	// First item should be Plans header
	item0, ok := items[0].(TreeItem)
	require.True(t, ok, "item 0 is not a TreeItem")
	assert.True(t, item0.IsHeader)
	assert.Equal(t, "Plan", item0.HeaderName, "expected Plans header, got: IsHeader=%v, HeaderName=%s", item0.IsHeader, item0.HeaderName)

	// Second item should be first plan document
	item1, ok := items[1].(TreeItem)
	require.True(t, ok, "item 1 is not a TreeItem")
	assert.False(t, item1.IsHeader, "expected document, got header")
	assert.Equal(t, "plans/plan1.md", item1.Document.RelPath)
	assert.False(t, item1.IsLastInType, "expected IsLastInType=false for first plan")

	// Third item should be second plan document (last in type)
	item2, ok := items[2].(TreeItem)
	require.True(t, ok, "item 2 is not a TreeItem")
	assert.True(t, item2.IsLastInType, "expected IsLastInType=true for second plan")
}

func TestTreeItemFilterValue(t *testing.T) {
	header := TreeItem{
		IsHeader:   true,
		HeaderName: "Plans",
	}

	assert.Empty(t, header.FilterValue(), "expected empty filter value for header, got %s", header.FilterValue())

	doc := TreeItem{
		IsHeader: false,
		Document: Document{
			RelPath: "plans/implementation.md",
		},
	}

	filterValue := doc.FilterValue()
	assert.Equal(t, "plans/implementation.md", filterValue)
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

	view := New(docs, "", nil, 0)

	// Should not panic and should have a list
	require.NotNil(t, view.list.Items(), "expected list items to be initialized")

	// Should be able to set size
	view.SetSize(80, 24)
}

func TestDocumentWatcherIntegration(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Create review view with watcher
	view := New([]Document{}, tmpDir, nil, 0)

	// View should have a watcher
	require.NotNil(t, view.watcher, "expected watcher to be initialized")

	// Init should return a command
	cmd := view.Init()
	require.NotNil(t, cmd, "expected Init to return a watch command")
}

func TestCommentDeletionWithConfirmation(t *testing.T) {
	doc := Document{
		Path:    "/path/to/test.md",
		RelPath: "plans/test.md",
		Type:    DocTypePlan,
		ModTime: time.Now(),
		Content: "Line 1\nLine 2\nLine 3\nLine 4\nLine 5",
	}

	view := New([]Document{doc}, "", nil, 0)
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

	require.NotNil(t, view.confirmModal, "expected confirmation modal to be shown")
	assert.Equal(t, 2, view.pendingDeleteLine, "expected pendingDeleteLine=2, got %d", view.pendingDeleteLine)

	// Press 'y' to confirm deletion
	view, _ = view.Update(keyMsg("y"))

	assert.Nil(t, view.confirmModal, "expected confirmation modal to be closed")
	assert.Equal(t, 0, view.pendingDeleteLine, "expected pendingDeleteLine to be cleared, got %d", view.pendingDeleteLine)

	// Comment should be deleted (session cleared when all comments removed)
	assert.Nil(t, view.activeSession, "expected session to be cleared")
}

func TestCommentDeletionCancellation(t *testing.T) {
	doc := Document{
		Path:    "/path/to/test.md",
		RelPath: "plans/test.md",
		Type:    DocTypePlan,
		ModTime: time.Now(),
		Content: "Line 1\nLine 2\nLine 3\nLine 4\nLine 5",
	}

	view := New([]Document{doc}, "", nil, 0)
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

	assert.Nil(t, view.confirmModal, "expected confirmation modal to be closed")
	assert.Equal(t, 0, view.pendingDeleteLine, "expected pendingDeleteLine to be cleared")

	// Comment should still exist
	assert.Len(t, view.activeSession.Comments, 1, "expected 1 comment, got %d", len(view.activeSession.Comments))
}

func TestReviewDiscardWithConfirmation(t *testing.T) {
	doc := Document{
		Path:    "/path/to/test.md",
		RelPath: "plans/test.md",
		Type:    DocTypePlan,
		ModTime: time.Now(),
		Content: "Line 1\nLine 2\nLine 3\nLine 4\nLine 5",
	}

	view := New([]Document{doc}, "", nil, 0)
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

	require.NotNil(t, view.confirmModal, "expected confirmation modal to be shown")
	assert.True(t, view.pendingDiscard, "expected pendingDiscard to be true")

	// Press 'y' to confirm discard
	view, cmd := view.Update(keyMsg("y"))

	assert.Nil(t, view.confirmModal, "expected confirmation modal to be closed")
	assert.False(t, view.pendingDiscard, "expected pendingDiscard to be cleared")

	// Execute the discard command
	if cmd != nil {
		msg := cmd()
		assert.IsType(t, reviewDiscardedMsg{}, msg, "expected reviewDiscardedMsg, got %T", msg)

		// Process the discard message
		view, _ = view.Update(msg)
	}

	// Session should be cleared
	assert.Nil(t, view.activeSession, "expected session to be cleared after discard")
}

func TestReviewDiscardCancellation(t *testing.T) {
	doc := Document{
		Path:    "/path/to/test.md",
		RelPath: "plans/test.md",
		Type:    DocTypePlan,
		ModTime: time.Now(),
		Content: "Line 1\nLine 2\nLine 3\nLine 4\nLine 5",
	}

	view := New([]Document{doc}, "", nil, 0)
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

	assert.Nil(t, view.confirmModal, "expected confirmation modal to be closed")
	assert.False(t, view.pendingDiscard, "expected pendingDiscard to be cleared")

	// Session should still exist
	require.NotNil(t, view.activeSession, "expected session to still exist after cancellation")
	assert.Len(t, view.activeSession.Comments, 1, "expected 1 comment, got %d", len(view.activeSession.Comments))
}

func TestReviewDiscardWithNoComments(t *testing.T) {
	doc := Document{
		Path:    "/path/to/test.md",
		RelPath: "plans/test.md",
		Type:    DocTypePlan,
		ModTime: time.Now(),
		Content: "Line 1\nLine 2\nLine 3",
	}

	view := New([]Document{doc}, "", nil, 0)
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

	assert.Nil(t, view.confirmModal, "expected no confirmation modal when there are no comments")
	assert.False(t, view.pendingDiscard, "expected pendingDiscard to remain false")
}

func TestCommentVisualStyling(t *testing.T) {
	doc := Document{
		Path:    "/path/to/test.md",
		RelPath: "plans/test.md",
		Type:    DocTypePlan,
		ModTime: time.Now(),
		Content: "Line 1\nLine 2\nLine 3",
	}

	view := New([]Document{doc}, "", nil, 0)
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

	// Check that the rendered output contains the comment icon
	assert.Contains(t, rendered, styles.IconComment, "expected rendered output to contain '%s' icon", styles.IconComment)

	// Check that the comment text is present
	assert.Contains(t, rendered, "This is a test comment", "expected rendered output to contain comment text")

	// Check that there's increased indentation (at least 4 spaces before the styled content)
	lines := strings.Split(rendered, "\n")
	var commentLineFound bool
	for _, line := range lines {
		if strings.Contains(line, styles.IconComment) {
			// Strip ANSI codes and check for leading spaces (indentation)
			cleanLine := ansiStripPattern.ReplaceAllString(line, "")
			assert.True(t, strings.HasPrefix(cleanLine, "    "), "expected comment line to have increased indentation (at least 4 spaces)")
			commentLineFound = true
			break
		}
	}

	assert.True(t, commentLineFound, "expected to find a comment line in rendered output")
}

func TestLineMappingWithComments(t *testing.T) {
	doc := Document{
		Path:    "/path/to/test.md",
		RelPath: "plans/test.md",
		Type:    DocTypePlan,
		ModTime: time.Now(),
		Content: "Line 1\nLine 2\nLine 3\nLine 4\nLine 5",
	}

	view := New([]Document{doc}, "", nil, 0)
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
		assert.Equal(t, tt.displayLine, got, "lineMapping[%d] = %d, want %d", tt.docLine, got, tt.displayLine)
	}

	// Test mapDocToDisplay helper
	for _, tt := range tests {
		got := view.mapDocToDisplay(tt.docLine, lineMapping)
		assert.Equal(t, tt.displayLine, got, "mapDocToDisplay(%d) = %d, want %d", tt.docLine, got, tt.displayLine)
	}

	// Test mapDisplayToDoc helper (reverse mapping)
	for _, tt := range tests {
		got := view.mapDisplayToDoc(tt.displayLine, lineMapping)
		assert.Equal(t, tt.docLine, got, "mapDisplayToDoc(%d) = %d, want %d", tt.displayLine, got, tt.docLine)
	}

	// Test that comment lines map back to 0 (not a document line)
	commentDisplayLines := []int{3, 6} // Lines where comments are inserted
	for _, displayLine := range commentDisplayLines {
		got := view.mapDisplayToDoc(displayLine, lineMapping)
		assert.Equal(t, 0, got, "mapDisplayToDoc(%d) = %d, want 0 (comment line, not a doc line)", displayLine, got)
	}
}

func TestView_WithPickerModal(t *testing.T) {
	docs := []Document{
		{RelPath: "doc1.md", Type: DocTypePlan},
	}

	reviewView := New(docs, "/test", nil, 0)
	reviewView.SetSize(100, 40)

	// Show picker
	_ = reviewView.ShowDocumentPicker()

	require.NotNil(t, reviewView.pickerModal, "Expected picker modal to be created")

	// Verify picker has documents
	assert.Len(t, reviewView.pickerModal.documents, 1, "Expected 1 document in picker, got %d", len(reviewView.pickerModal.documents))
}

func TestView_HasPickerModalField(t *testing.T) {
	reviewView := New(nil, "", nil, 0)

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

	view := New([]Document{doc}, "", nil, 0)
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
	require.Equal(t, 7, displayLine, "expected doc line 5 to map to display line 7, got %d", displayLine)

	// Call ensureCursorVisible
	view.ensureCursorVisible()

	// Verify viewport scrolled to show the display line, not the document line
	offset := view.viewport.YOffset()
	visibleHeight := view.viewport.VisibleLineCount() - 1 // -1 for status bar

	// Display line 7 should be visible within the viewport
	assert.True(t, displayLine >= offset+1 && displayLine <= offset+visibleHeight,
		"display line %d not visible in viewport (offset=%d, visibleHeight=%d)",
		displayLine, offset, visibleHeight)
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

	view := New([]Document{doc}, "", nil, 0)
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
	require.Equal(t, 5, displayLine, "expected doc line 3 to map to display line 5, got %d", displayLine)

	// Verify cursor is at the correct document line
	assert.Equal(t, 3, view.cursorLine, "expected cursor at doc line 3, got %d", view.cursorLine)

	// Verify viewport scrolled to make display line visible
	offset := view.viewport.YOffset()
	visibleHeight := view.viewport.VisibleLineCount() - 1 // -1 for status bar

	assert.True(t, displayLine >= offset+1 && displayLine <= offset+visibleHeight,
		"display line %d not visible after jumpToMatch (offset=%d, visibleHeight=%d)",
		displayLine, offset, visibleHeight)
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

	require.Len(t, displayToDoc, len(expectedReverse), "expected %d entries in reverse map, got %d", len(expectedReverse), len(displayToDoc))

	for displayLine, expectedDocLine := range expectedReverse {
		docLine, ok := displayToDoc[displayLine]
		assert.True(t, ok, "display line %d missing from reverse map", displayLine)
		if ok {
			assert.Equal(t, expectedDocLine, docLine, "displayToDoc[%d] = %d, want %d", displayLine, docLine, expectedDocLine)
		}
	}

	// Verify comment lines (3, 6) are NOT in the reverse map
	commentDisplayLines := []int{3, 6}
	for _, displayLine := range commentDisplayLines {
		_, ok := displayToDoc[displayLine]
		assert.False(t, ok, "comment display line %d should not be in reverse map", displayLine)
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

	view := New([]Document{doc}, "", nil, 0)
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
			assert.False(t, ok, "display line %d should be comment (not in map), but got doc line %d", tt.displayLine, docLine)
		} else {
			assert.True(t, ok, "display line %d should map to doc line %d, but not found in map", tt.displayLine, tt.wantDocLine)
			if ok {
				assert.Equal(t, tt.wantDocLine, docLine, "displayToDoc[%d] = %d, want %d", tt.displayLine, docLine, tt.wantDocLine)
			}
		}
	}

	// Verify nil lineMapping returns nil
	nilResult := buildDisplayToDocMap(nil)
	assert.Nil(t, nilResult, "buildDisplayToDocMap(nil) should return nil")
}

// TestFinalizedSessionsNotReloaded verifies that finalized sessions are not
// loaded as active sessions when opening a document.
func TestFinalizedSessionsNotReloaded(t *testing.T) {
	// This test requires a real store with database persistence
	// to test the session finalization behavior
	tmpDir := t.TempDir()

	// Create a test database
	database, err := db.Open(tmpDir, db.DefaultOpenOptions())
	require.NoError(t, err, "failed to open database")
	defer func() {
		assert.NoError(t, database.Close(), "failed to close database")
	}()

	// Create a test store
	store := stores.NewReviewStore(database)

	// Create a test document
	docPath := filepath.Join(tmpDir, "test.md")
	content := "Line 1\nLine 2\nLine 3"
	require.NoError(t, os.WriteFile(docPath, []byte(content), 0o644), "failed to write test file")

	doc := Document{
		Path:    docPath,
		RelPath: "test.md",
		Type:    DocTypePlan,
		ModTime: time.Now(),
		Content: content,
	}

	// Create review view with the store
	view := New([]Document{doc}, tmpDir, store, 0)
	view.SetSize(80, 24)

	// Load document and create a session with a comment
	view.loadDocument(&doc)

	// Create a comment (which will create a session)
	view.selectionStart = 1
	view.cursorLine = 2
	view.selectionMode = true
	view.addComment("Test comment before finalization")

	require.NotNil(t, view.activeSession, "expected active session after adding comment")

	sessionID := view.activeSession.ID

	// Finalize the session
	ctx := context.Background()
	require.NoError(t, store.FinalizeSession(ctx, sessionID), "failed to finalize session")

	// Clear active session (simulating what happens after finalization)
	view.activeSession = nil

	// Reload the document - should NOT reload the finalized session
	view.loadDocument(&doc)

	// Verify the finalized session was NOT loaded
	assert.Nil(t, view.activeSession, "expected activeSession to be nil after reloading with finalized session")
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

	view := New([]Document{doc}, "", nil, 0)
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
	assert.NotZero(t, view.cursorLine, "ctrl+d mapped to comment line (0), should map to valid document line")
	assert.LessOrEqual(t, view.cursorLine, len(lines), "ctrl+d set cursor beyond document length: %d > %d", view.cursorLine, len(lines))

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
	assert.NotZero(t, view.cursorLine, "ctrl+u mapped to comment line (0), should map to valid document line")
	assert.LessOrEqual(t, view.cursorLine, len(lines), "ctrl+u set cursor beyond document length: %d > %d", view.cursorLine, len(lines))

	// Cursor should have moved up from the previous position
	assert.Less(t, view.cursorLine, cursorAfterDown, "ctrl+u should move cursor up: before=%d, after=%d", cursorAfterDown, view.cursorLine)
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
	v := New(docs, "/test", nil, 0)
	v.SetSize(100, 40)

	// Manually set up the finalization modal state (simulating pressing 'f')
	v.fullScreen = true
	v.selectedDoc = &docs[0]
	modal := NewFinalizationModal("test feedback", 100, 40)
	v.finalizationModal = &modal

	// Verify initial state
	assert.Equal(t, 0, v.finalizationModal.selectedIdx, "Initial selectedIdx should be 0, got %d", v.finalizationModal.selectedIdx)

	// Send 'j' key to view - should be forwarded to modal (stays at 0 with single option)
	jKey := keyMsg("j")
	v, _ = v.Update(jKey)

	// Verify modal received the key
	require.NotNil(t, v.finalizationModal, "finalizationModal should not be nil")
	// With single option, selectedIdx stays at 0
	assert.Equal(t, 0, v.finalizationModal.selectedIdx, "With single option, selectedIdx should remain 0, got %d", v.finalizationModal.selectedIdx)

	// Test enter key confirms the modal
	enterKey := keyMsg("enter")
	v, _ = v.Update(enterKey)
	// Note: After confirmation, finalizationModal might be nil (handled by view)
}

func TestHasActiveEditor(t *testing.T) {
	doc := Document{
		Path:    "/path/to/test.md",
		RelPath: "test.md",
		Type:    DocTypePlan,
		ModTime: time.Now(),
		Content: "Line 1\nLine 2\nLine 3",
	}

	view := New([]Document{doc}, "", nil, 0)
	view.SetSize(80, 24)
	view.fullScreen = true
	view.selectedDoc = &doc

	// Initially no active editor
	assert.False(t, view.HasActiveEditor(), "expected HasActiveEditor to be false initially")

	// Enable search mode - should have active editor
	view.searchMode = true
	assert.True(t, view.HasActiveEditor(), "expected HasActiveEditor to be true when searchMode is active")

	// Disable search mode
	view.searchMode = false
	assert.False(t, view.HasActiveEditor(), "expected HasActiveEditor to be false after disabling search mode")

	// Open comment modal - should have active editor
	modal := NewCommentModal(1, 2, "context", 80, 24, 80)
	view.commentModal = &modal
	assert.True(t, view.HasActiveEditor(), "expected HasActiveEditor to be true when comment modal is active")

	// Close comment modal
	view.commentModal = nil
	assert.False(t, view.HasActiveEditor(), "expected HasActiveEditor to be false after closing comment modal")

	// Both search mode and comment modal active
	view.searchMode = true
	view.commentModal = &modal
	assert.True(t, view.HasActiveEditor(), "expected HasActiveEditor to be true when both are active")
}

func TestHighlightLineNumber(t *testing.T) {
	view := View{
		width:  80,
		height: 24,
	}

	// Test with actual format: " n  content" (no pipe, two spaces)
	tests := []struct {
		name     string
		input    string
		wantText string // What text should be in the gutter
	}{
		{
			name:     "single digit line number",
			input:    " 1  This is content",
			wantText: " 1",
		},
		{
			name:     "double digit line number",
			input:    " 10  This is content",
			wantText: " 10",
		},
		{
			name:     "triple digit line number",
			input:    " 100  This is content",
			wantText: " 100",
		},
		{
			name:     "with padding (right-aligned)",
			input:    "   5  This is content",
			wantText: "   5",
		},
		{
			name:     "empty content",
			input:    " 1  ",
			wantText: " 1",
		},
		{
			name:     "real world example from output",
			input:    " 6    ## Quality Gates",
			wantText: " 6",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use a test style that adds markers we can detect
			testStyle := styles.ReviewCommentedLineNumStyle

			result := view.highlightLineNumber(tt.input, testStyle)

			// Strip ANSI codes to verify structure
			cleanResult := ansiStripPattern.ReplaceAllString(result, "")
			assert.Equal(t, tt.input, cleanResult, "stripped result should match input")

			// Verify the result contains ANSI codes (meaning styling was applied)
			assert.NotEqual(t, tt.input, result, "result should contain ANSI styling codes")

			// Verify the gutter portion was extracted correctly
			// Use regex to find the gutter in the clean result
			gutterPattern := regexp.MustCompile(`^( *\d+)  `)
			matches := gutterPattern.FindStringSubmatch(cleanResult)
			if len(matches) >= 2 {
				gutterPart := matches[1]
				assert.Equal(t, tt.wantText, gutterPart, "gutter should be correct")
			}
		})
	}
}

func TestHighlightLineNumber_NoSeparator(t *testing.T) {
	view := View{
		width:  80,
		height: 24,
	}

	// Test line without separator
	input := "No separator here"
	testStyle := styles.ReviewCommentedLineNumStyle

	result := view.highlightLineNumber(input, testStyle)

	// Should return unchanged
	assert.Equal(t, input, result, "line without separator should be returned unchanged")
}
