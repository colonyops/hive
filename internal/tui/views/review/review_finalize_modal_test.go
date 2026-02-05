package review

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

const testFeedback = "Test feedback"

func TestFinalizationModal_Creation(t *testing.T) {
	feedback := testFeedback
	modal := NewFinalizationModal(feedback, 100, 40)

	if len(modal.options) != 1 {
		t.Errorf("Expected 1 option, got %d", len(modal.options))
	}
	if modal.options[0].action != FinalizationActionClipboard {
		t.Error("Option should be clipboard")
	}
}

func TestFinalizationModal_Navigation(t *testing.T) {
	feedback := testFeedback
	modal := NewFinalizationModal(feedback, 100, 40)

	// Initial selection should be 0
	if modal.selectedIdx != 0 {
		t.Errorf("Initial selection should be 0, got %d", modal.selectedIdx)
	}

	// With single option, navigation stays at 0
	modal.selectedIdx = 0
	if modal.selectedIdx != 0 {
		t.Errorf("Selection should be 0, got %d", modal.selectedIdx)
	}
}

func TestFinalizationModal_Confirmation(t *testing.T) {
	feedback := testFeedback
	modal := NewFinalizationModal(feedback, 100, 40)

	// Set confirmed flag
	modal.confirmed = true
	if !modal.Confirmed() {
		t.Error("Modal should be confirmed")
	}

	// Selected action should be clipboard (index 0)
	if modal.SelectedAction() != FinalizationActionClipboard {
		t.Errorf("Expected clipboard action, got %v", modal.SelectedAction())
	}
}

func TestFinalizationModal_Cancellation(t *testing.T) {
	feedback := testFeedback
	modal := NewFinalizationModal(feedback, 100, 40)

	// Set cancelled flag
	modal.cancelled = true
	if !modal.Cancelled() {
		t.Error("Modal should be cancelled")
	}
}

func TestFinalizationModal_View(t *testing.T) {
	feedback := testFeedback
	modal := NewFinalizationModal(feedback, 100, 40)

	view := modal.View()
	if view == "" {
		t.Error("View should not be empty")
	}

	// Check that the view contains expected text
	if !contains(view, "Finalize Review") {
		t.Error("View should contain title")
	}
	if !contains(view, "Copy to clipboard") {
		t.Error("View should contain clipboard option")
	}
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
	if updatedModal.selectedIdx != 0 {
		t.Errorf("With single option, selectedIdx should remain 0, got %d", updatedModal.selectedIdx)
	}

	// Test 'enter' key - should confirm
	keyMsg = tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
	updatedModal, _ = modal.Update(keyMsg)
	if !updatedModal.Confirmed() {
		t.Error("After pressing 'enter', modal should be confirmed")
	}

	// Test 'esc' key - should cancel
	modal = NewFinalizationModal(feedback, 100, 40)
	keyMsg = tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc})
	updatedModal, _ = modal.Update(keyMsg)
	if !updatedModal.Cancelled() {
		t.Error("After pressing 'esc', modal should be cancelled")
	}
}
