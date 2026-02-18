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
				m.state = stateCommandPalette
			},
			want: true,
		},
		{
			name: "creating session has editor focus",
			setupFunc: func(m *Model) {
				m.state = stateCreatingSession
			},
			want: true,
		},
		{
			name: "normal state has no editor focus",
			setupFunc: func(m *Model) {
				m.state = stateNormal
			},
			want: false,
		},
		{
			name: "confirming modal has no editor focus",
			setupFunc: func(m *Model) {
				m.state = stateConfirming
			},
			want: false,
		},
		{
			name: "loading state has no editor focus",
			setupFunc: func(m *Model) {
				m.state = stateLoading
			},
			want: false,
		},
		{
			name: "renaming state has editor focus",
			setupFunc: func(m *Model) {
				m.state = stateRenaming
			},
			want: true,
		},
		{
			name: "form input has editor focus",
			setupFunc: func(m *Model) {
				m.state = stateFormInput
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Model{modals: NewModalCoordinator()}
			tt.setupFunc(m)
			got := m.hasEditorFocus()
			assert.Equal(t, tt.want, got)
		})
	}
}
