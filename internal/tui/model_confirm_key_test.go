package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	act "github.com/colonyops/hive/internal/core/action"
	"github.com/stretchr/testify/assert"
)

// keyPress creates a tea.KeyPressMsg with the given text representation.
func keyPressMsg(text string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Text: text}
}

// minimalConfirmModel creates a Model with only the fields needed for confirm key tests.
func minimalConfirmModel() Model {
	return Model{
		state:  stateConfirming,
		modals: NewModalCoordinator(),
	}
}

func TestHandleConfirmModalKey_Esc_ClearsState(t *testing.T) {
	m := minimalConfirmModel()
	m.modals.Confirm = NewModal("Confirm", "Are you sure?")
	m.modals.Pending = Action{Type: act.TypeShell, ShellCmd: "echo test"}

	result, _ := m.handleConfirmModalKey("esc")
	rm := result.(Model)

	assert.Equal(t, stateNormal, rm.state, "esc should return to normal state")
	assert.False(t, rm.modals.Confirm.Visible(), "confirm modal must be dismissed after esc")
	assert.Equal(t, Action{}, rm.modals.Pending, "pending action must be cleared after esc")
}

func TestHandleConfirmModalKey_Enter_NotConfirmed_ClearsState(t *testing.T) {
	m := minimalConfirmModel()
	m.modals.Confirm = NewModal("Confirm", "Are you sure?")
	// Toggle to Cancel button so ConfirmSelected() returns false
	m.modals.Confirm.ToggleSelection()
	m.modals.Pending = Action{Type: act.TypeShell, ShellCmd: "echo test"}

	result, cmd := m.handleConfirmModalKey(keyEnter)
	rm := result.(Model)

	assert.Equal(t, stateNormal, rm.state, "enter (cancel) should return to normal state")
	assert.False(t, rm.modals.Confirm.Visible(), "confirm modal must be dismissed after cancel")
	assert.Equal(t, Action{}, rm.modals.Pending, "pending action must be cleared after cancel")
	assert.Nil(t, cmd, "no command should be emitted when cancelled")
}

func TestHandleConfirmModalKey_Enter_Confirmed_ClearsModal(t *testing.T) {
	m := minimalConfirmModel()
	m.modals.Confirm = NewModal("Confirm", "Are you sure?")
	m.modals.Pending = Action{Type: act.TypeShell, ShellCmd: "echo test"}

	result, cmd := m.handleConfirmModalKey(keyEnter)
	rm := result.(Model)

	assert.Equal(t, stateNormal, rm.state, "enter (confirm) should return to normal state")
	assert.False(t, rm.modals.Confirm.Visible(), "confirm modal must be dismissed after confirm")
	assert.Equal(t, Action{}, rm.modals.Pending, "pending action must be cleared after confirm")
	assert.NotNil(t, cmd, "a command should be returned to execute the action")
}

func TestHandleKey_Loading_BlocksKeys(t *testing.T) {
	m := Model{
		state:  stateLoading,
		modals: NewModalCoordinator(),
	}

	tests := []string{"enter", "q", "esc", "tab", "j"}
	for _, key := range tests {
		t.Run(key+" should be blocked", func(t *testing.T) {
			msg := keyPressMsg(key)
			result, cmd := m.handleKey(msg)
			rm := result.(Model)
			assert.Equal(t, stateLoading, rm.state, "state must stay loading when %q pressed", key)
			assert.Nil(t, cmd, "no command should be emitted for blocked key %q", key)
		})
	}
}

func TestHandleKey_Loading_CtrlC_Quits(t *testing.T) {
	m := Model{
		state:  stateLoading,
		modals: NewModalCoordinator(),
	}

	msg := tea.KeyPressMsg{Text: "ctrl+c", Code: 3}
	result, _ := m.handleKey(msg)
	rm := result.(Model)
	assert.True(t, rm.quitting, "ctrl+c should set quitting=true during loading")
}
