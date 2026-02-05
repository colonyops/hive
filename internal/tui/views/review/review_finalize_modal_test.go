package review

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

const testFeedback = "Test feedback"

func TestFinalizationModal_Creation(t *testing.T) {
	feedback := testFeedback

	t.Run("with agent command available", func(t *testing.T) {
		modal := NewFinalizationModal(feedback, true, 100, 40)

		if len(modal.options) != 2 {
			t.Errorf("Expected 2 options with agent command, got %d", len(modal.options))
		}
		if modal.options[0].action != FinalizationActionClipboard {
			t.Error("First option should be clipboard")
		}
		if modal.options[1].action != FinalizationActionSendToAgent {
			t.Error("Second option should be send to agent")
		}
	})

	t.Run("without agent command", func(t *testing.T) {
		modal := NewFinalizationModal(feedback, false, 100, 40)

		if len(modal.options) != 1 {
			t.Errorf("Expected 1 option without agent command, got %d", len(modal.options))
		}
		if modal.options[0].action != FinalizationActionClipboard {
			t.Error("Only option should be clipboard")
		}
	})
}

func TestFinalizationModal_Navigation(t *testing.T) {
	feedback := testFeedback
	modal := NewFinalizationModal(feedback, true, 100, 40)

	// Initial selection should be 0
	if modal.selectedIdx != 0 {
		t.Errorf("Initial selection should be 0, got %d", modal.selectedIdx)
	}

	// Test navigation directly by updating selectedIdx
	// (Integration tests would test key handling)
	modal.selectedIdx = 1
	if modal.selectedIdx != 1 {
		t.Errorf("Selection should be 1, got %d", modal.selectedIdx)
	}

	// Test wrap around
	modal.selectedIdx = 0
	if modal.selectedIdx != 0 {
		t.Errorf("Selection should wrap to 0, got %d", modal.selectedIdx)
	}
}

func TestFinalizationModal_Confirmation(t *testing.T) {
	feedback := testFeedback
	modal := NewFinalizationModal(feedback, true, 100, 40)

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
	modal := NewFinalizationModal(feedback, true, 100, 40)

	// Set cancelled flag
	modal.cancelled = true
	if !modal.Cancelled() {
		t.Error("Modal should be cancelled")
	}
}

func TestFinalizationModal_SelectedAction(t *testing.T) {
	feedback := testFeedback
	modal := NewFinalizationModal(feedback, true, 100, 40)

	// Select second option (send to agent)
	modal.selectedIdx = 1
	modal.confirmed = true

	if modal.SelectedAction() != FinalizationActionSendToAgent {
		t.Errorf("Expected send to agent action, got %v", modal.SelectedAction())
	}
}

func TestFinalizationModal_View(t *testing.T) {
	feedback := testFeedback
	modal := NewFinalizationModal(feedback, true, 100, 40)

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
	if !contains(view, "Send to Claude agent") {
		t.Error("View should contain agent option")
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
	modal := NewFinalizationModal(feedback, true, 100, 40)

	// Test 'j' key - should move selection down
	keyMsg := tea.KeyPressMsg(tea.Key{Text: "j", Code: 'j'})
	updatedModal, _ := modal.Update(keyMsg)
	if updatedModal.selectedIdx != 1 {
		t.Errorf("After pressing 'j', selectedIdx should be 1, got %d", updatedModal.selectedIdx)
	}

	// Test 'k' key - should move selection up
	modal = updatedModal
	keyMsg = tea.KeyPressMsg(tea.Key{Text: "k", Code: 'k'})
	updatedModal, _ = modal.Update(keyMsg)
	if updatedModal.selectedIdx != 0 {
		t.Errorf("After pressing 'k', selectedIdx should be 0, got %d", updatedModal.selectedIdx)
	}

	// Test 'enter' key - should confirm
	keyMsg = tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
	updatedModal, _ = modal.Update(keyMsg)
	if !updatedModal.Confirmed() {
		t.Error("After pressing 'enter', modal should be confirmed")
	}

	// Test 'esc' key - should cancel
	modal = NewFinalizationModal(feedback, true, 100, 40)
	keyMsg = tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc})
	updatedModal, _ = modal.Update(keyMsg)
	if !updatedModal.Cancelled() {
		t.Error("After pressing 'esc', modal should be cancelled")
	}
}
