package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestModel_hasEditorFocus(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func(*Model)
		want      bool
	}{
		{
			name: "no editor focus initially",
			setupFunc: func(m *Model) {
				// Default state
			},
			want: false,
		},
		{
			name: "command palette has editor focus",
			setupFunc: func(m *Model) {
				m.state = UIStateCommandPalette
			},
			want: true,
		},
		{
			name: "creating session has editor focus",
			setupFunc: func(m *Model) {
				m.state = UIStateCreatingSession
			},
			want: true,
		},
		{
			name: "normal state has no editor focus",
			setupFunc: func(m *Model) {
				m.state = UIStateNormal
			},
			want: false,
		},
		{
			name: "confirming modal has no editor focus",
			setupFunc: func(m *Model) {
				m.state = UIStateConfirming
			},
			want: false,
		},
		{
			name: "loading state has no editor focus",
			setupFunc: func(m *Model) {
				m.state = UIStateLoading
			},
			want: false,
		},
		{
			name: "renaming state has editor focus",
			setupFunc: func(m *Model) {
				m.state = UIStateRenaming
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Model{}
			tt.setupFunc(m)
			got := m.hasEditorFocus()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestModel_hasEditorFocus_Integration(t *testing.T) {
	t.Run("blocking keybindings when in command palette", func(t *testing.T) {
		m := &Model{
			state: UIStateCommandPalette,
		}

		// Should have editor focus
		assert.True(t, m.hasEditorFocus())

		// This means 'q' and other keys should be delegated, not trigger quit
	})

	t.Run("blocking keybindings when creating session", func(t *testing.T) {
		m := &Model{
			state: UIStateCreatingSession,
		}

		// Should have editor focus
		assert.True(t, m.hasEditorFocus())
	})

	t.Run("allowing keybindings in normal state", func(t *testing.T) {
		m := &Model{
			state: UIStateNormal,
		}

		// Should NOT have editor focus
		assert.False(t, m.hasEditorFocus())

		// This means 'q' should trigger quit
	})
}
