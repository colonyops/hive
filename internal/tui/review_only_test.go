package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/colonyops/hive/internal/data/db"
	"github.com/colonyops/hive/internal/tui/views/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReviewOnly_QKeyWithActiveEditor verifies that pressing "q" doesn't quit
// when the tree search input is active.
func TestReviewOnly_QKeyWithActiveEditor(t *testing.T) {
	tmpDir := t.TempDir()
	dbConn, err := db.Open(tmpDir, db.DefaultOpenOptions())
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, dbConn.Close())
	}()

	doc := review.Document{
		Path:    "/test/doc.md",
		RelPath: "doc.md",
		Type:    review.DocTypePlan,
		Content: "# Test Document\n\nSome content",
	}

	m := NewReviewOnly(ReviewOnlyOptions{
		Documents:   []review.Document{doc},
		InitialDoc:  nil,
		ContextDir:  "",
		DB:          dbConn,
		CopyCommand: "",
	})

	m.Init()
	model, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = model.(ReviewOnlyModel)

	// Activate tree search input by pressing "/"
	slashMsg := tea.KeyPressMsg{Text: "/", Code: '/'}
	model, _ = m.Update(slashMsg)
	m = model.(ReviewOnlyModel)

	// Press "q" - should NOT quit because search input is active
	msg := tea.KeyPressMsg{Text: "q", Code: 'q'}
	model, _ = m.Update(msg)
	m = model.(ReviewOnlyModel)

	assert.False(t, m.quitting, "Should not quit when tree search is active")

	// Close search with esc and press "q" - should quit
	escMsg := tea.KeyPressMsg{Text: "esc", Code: 27}
	model, _ = m.Update(escMsg)
	m = model.(ReviewOnlyModel)

	model, _ = m.Update(msg)
	m = model.(ReviewOnlyModel)

	assert.True(t, m.quitting, "Should quit when no active editor")
}

// TestReviewOnly_CtrlCAlwaysQuits verifies that Ctrl+C quits.
func TestReviewOnly_CtrlCAlwaysQuits(t *testing.T) {
	tmpDir := t.TempDir()
	dbConn, err := db.Open(tmpDir, db.DefaultOpenOptions())
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, dbConn.Close())
	}()

	doc := review.Document{
		Path:    "/test/doc.md",
		RelPath: "doc.md",
		Type:    review.DocTypePlan,
		Content: "# Test Document\n\nSome content",
	}

	m := NewReviewOnly(ReviewOnlyOptions{
		Documents:   []review.Document{doc},
		InitialDoc:  nil,
		ContextDir:  "",
		DB:          dbConn,
		CopyCommand: "",
	})

	m.Init()
	model, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = model.(ReviewOnlyModel)

	msg := tea.KeyPressMsg{Text: "ctrl+c", Code: 3}
	model, _ = m.Update(msg)
	m = model.(ReviewOnlyModel)

	assert.True(t, m.quitting, "Ctrl+C should quit")
}

// TestReviewView_HasActiveEditor verifies the HasActiveEditor method.
func TestReviewView_HasActiveEditor(t *testing.T) {
	v := review.New([]review.Document{}, "", nil, nil, 0)

	assert.False(t, v.HasActiveEditor(), "Should have no active editor initially")
}
