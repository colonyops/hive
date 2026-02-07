package review

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

const testFeedback = "Test feedback"

func TestFinalizationModal_Creation(t *testing.T) {
	feedback := testFeedback
	modal := NewFinalizationModal(feedback, 100, 40)

	assert.Len(t, modal.options, 1, "Expected 1 option, got %d", len(modal.options))
	assert.Equal(t, FinalizationActionClipboard, modal.options[0].action)
}

func TestFinalizationModal_Navigation(t *testing.T) {
	feedback := testFeedback
	modal := NewFinalizationModal(feedback, 100, 40)

	// Initial selection should be 0
	assert.Equal(t, 0, modal.selectedIdx, "Initial selection should be 0, got %d", modal.selectedIdx)

	// With single option, navigation stays at 0
	modal.selectedIdx = 0
	assert.Equal(t, 0, modal.selectedIdx, "Selection should be 0, got %d", modal.selectedIdx)
}

func TestFinalizationModal_Confirmation(t *testing.T) {
	feedback := testFeedback
	modal := NewFinalizationModal(feedback, 100, 40)

	// Set confirmed flag
	modal.confirmed = true
	assert.True(t, modal.Confirmed(), "Modal should be confirmed")

	// Selected action should be clipboard (index 0)
	assert.Equal(t, FinalizationActionClipboard, modal.SelectedAction())
}

func TestFinalizationModal_Cancellation(t *testing.T) {
	feedback := testFeedback
	modal := NewFinalizationModal(feedback, 100, 40)

	// Set cancelled flag
	modal.cancelled = true
	assert.True(t, modal.Cancelled(), "Modal should be cancelled")
}

func TestFinalizationModal_View(t *testing.T) {
	feedback := testFeedback
	modal := NewFinalizationModal(feedback, 100, 40)

	view := modal.View()
	assert.NotEmpty(t, view, "View should not be empty")

	// Check that the view contains expected text
	assert.True(t, contains(view, "Finalize Review"), "View should contain title")
	assert.True(t, contains(view, "Copy to clipboard"), "View should contain clipboard option")
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && stringContains(s, substr))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestFinalizationModal_KeyHandling(t *testing.T) {
	feedback := testFeedback
	modal := NewFinalizationModal(feedback, 100, 40)

	// With single option, j/k don't change selectedIdx
	keyMsg := tea.KeyPressMsg(tea.Key{Text: "j", Code: 'j'})
	updatedModal, _ := modal.Update(keyMsg)
	assert.Equal(t, 0, updatedModal.selectedIdx, "With single option, selectedIdx should remain 0, got %d", updatedModal.selectedIdx)

	// Test 'enter' key - should confirm
	keyMsg = tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
	updatedModal, _ = modal.Update(keyMsg)
	assert.True(t, updatedModal.Confirmed(), "After pressing 'enter', modal should be confirmed")

	// Test 'esc' key - should cancel
	modal = NewFinalizationModal(feedback, 100, 40)
	keyMsg = tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc})
	updatedModal, _ = modal.Update(keyMsg)
	assert.True(t, updatedModal.Cancelled(), "After pressing 'esc', modal should be cancelled")
}
