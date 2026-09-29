package tasks

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/colonyops/hive/internal/core/action"
	"github.com/colonyops/hive/internal/core/hc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubKeyResolver is a minimal KeyResolver for testing action passthrough.
type stubKeyResolver struct {
	actions map[string]action.Action
}

func (s *stubKeyResolver) IsAction(key string, t action.Type) bool {
	a, ok := s.actions[key]
	return ok && a.Type == t
}

func (s *stubKeyResolver) ResolveAction(key string) (action.Action, bool) {
	a, ok := s.actions[key]
	return a, ok
}

func (s *stubKeyResolver) HelpEntries() []string { return nil }

// newDetailView creates a minimal View focused on the detail pane for key-handling tests.
func newDetailView(handler KeyResolver) *View {
	return &View{
		handler:  handler,
		comments: make(map[string][]hc.Comment),
		focus:    paneDetail,
	}
}

func pressKey(key string) tea.KeyPressMsg {
	if key == "esc" {
		return tea.KeyPressMsg{Code: tea.KeyEsc}
	}
	r := []rune(key)
	return tea.KeyPressMsg{Text: key, Code: r[0]}
}

func TestHandleDetailKey_ActionPassthrough(t *testing.T) {
	tests := []struct {
		name       string
		key        string
		actionType action.Type
	}{
		{"done key dispatches TypeTasksSetDone", "d", action.TypeTasksSetDone},
		{"open key dispatches TypeTasksSetOpen", "o", action.TypeTasksSetOpen},
		{"in-progress key dispatches TypeTasksSetInProgress", "i", action.TypeTasksSetInProgress},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &stubKeyResolver{
				actions: map[string]action.Action{
					tt.key: {Type: tt.actionType},
				},
			}
			v := newDetailView(handler)

			cmd := v.handleDetailKey(pressKey(tt.key))
			require.NotNil(t, cmd, "expected a command for action key %q", tt.key)

			msg := cmd()
			actionMsg, ok := msg.(ActionRequestMsg)
			require.True(t, ok, "expected ActionRequestMsg, got %T", msg)
			assert.Equal(t, tt.actionType, actionMsg.Action.Type)

			// Focus remains on detail pane after action dispatch
			assert.Equal(t, paneDetail, v.focus)
		})
	}
}

func TestHandleDetailKey_EscapeReturnsFocusToTree(t *testing.T) {
	v := newDetailView(nil)

	cmd := v.handleDetailKey(pressKey("esc"))
	assert.Nil(t, cmd)
	assert.Equal(t, paneTree, v.focus)
}

func TestHandleDetailKey_UnmappedKeyDoesNotDispatch(t *testing.T) {
	handler := &stubKeyResolver{
		actions: map[string]action.Action{
			"d": {Type: action.TypeTasksSetDone},
		},
	}
	v := newDetailView(handler)

	// "z" is not in the handler map — must not produce an ActionRequestMsg
	cmd := v.handleDetailKey(pressKey("z"))
	if cmd != nil {
		result := cmd()
		_, isAction := result.(ActionRequestMsg)
		assert.False(t, isAction, "unmapped key should not produce ActionRequestMsg")
	}
}

func TestHandleDetailKey_NilHandlerDoesNotPanic(t *testing.T) {
	v := newDetailView(nil)

	assert.NotPanics(t, func() {
		v.handleDetailKey(pressKey("d"))
	})
}
