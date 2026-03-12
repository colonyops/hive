package review

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

const testFeedback = "Test feedback"

func TestFinalizationModal_Creation(t *testing.T) {
	modal := NewFinalizationModal(testFeedback, 100, 40)
	assert.False(t, modal.confirmed)
	assert.False(t, modal.cancelled)
}

func TestFinalizationModal_Confirmation(t *testing.T) {
	modal := NewFinalizationModal(testFeedback, 100, 40)
	modal.confirmed = true
	assert.True(t, modal.Confirmed())
}

func TestFinalizationModal_Cancellation(t *testing.T) {
	modal := NewFinalizationModal(testFeedback, 100, 40)
	modal.cancelled = true
	assert.True(t, modal.Cancelled())
}

func TestFinalizationModal_View(t *testing.T) {
	modal := NewFinalizationModal(testFeedback, 100, 40)
	view := modal.View()
	assert.NotEmpty(t, view)
	assert.True(t, contains(view, "Finalize Review"))
	assert.True(t, contains(view, "ctrl+s"))
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
	// ctrl+s confirms
	modal := NewFinalizationModal(testFeedback, 100, 40)
	updated, _ := modal.Update(tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
	assert.True(t, updated.Confirmed(), "ctrl+s should confirm")

	// esc cancels
	modal = NewFinalizationModal(testFeedback, 100, 40)
	updated, _ = modal.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	assert.True(t, updated.Cancelled(), "esc should cancel")
}

func TestFinalizationModal_GeneralComment(t *testing.T) {
	modal := NewFinalizationModal(testFeedback, 100, 40)
	assert.Empty(t, modal.GeneralComment())
}
