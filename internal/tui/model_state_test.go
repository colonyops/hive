package tui

import (
	"testing"

	"github.com/hay-kot/hive/internal/core/session"
	"github.com/hay-kot/hive/internal/integration/terminal"
	"github.com/stretchr/testify/assert"
)

func TestHandleFilterAction(t *testing.T) {
	tests := []struct {
		name       string
		actionType ActionType
		want       bool
		wantFilter terminal.Status
	}{
		{"filter all clears filter", ActionTypeFilterAll, true, ""},
		{"filter active", ActionTypeFilterActive, true, terminal.StatusActive},
		{"filter approval", ActionTypeFilterApproval, true, terminal.StatusApproval},
		{"filter ready", ActionTypeFilterReady, true, terminal.StatusReady},
		{"none is not filter", ActionTypeNone, false, ""},
		{"recycle is not filter", ActionTypeRecycle, false, ""},
		{"delete is not filter", ActionTypeDelete, false, ""},
		{"shell is not filter", ActionTypeShell, false, ""},
		{"doc review is not filter", ActionTypeDocReview, false, ""},
		{"delete batch is not filter", ActionTypeDeleteRecycledBatch, false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Model{}
			got := m.handleFilterAction(tt.actionType)
			assert.Equal(t, tt.want, got)
			if tt.want {
				assert.Equal(t, tt.wantFilter, m.statusFilter)
			}
		})
	}
}

func TestIsModalActive(t *testing.T) {
	tests := []struct {
		name  string
		state UIState
		want  bool
	}{
		{"normal is not modal", stateNormal, false},
		{"confirming is modal", stateConfirming, true},
		{"loading is modal", stateLoading, true},
		{"running recycle is modal", stateRunningRecycle, true},
		{"previewing message is modal", statePreviewingMessage, true},
		{"creating session is modal", stateCreatingSession, true},
		{"command palette is modal", stateCommandPalette, true},
		{"showing help is modal", stateShowingHelp, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Model{state: tt.state}
			assert.Equal(t, tt.want, m.isModalActive())
		})
	}
}

func TestIsSessionsFocused(t *testing.T) {
	tests := []struct {
		name string
		view ViewType
		want bool
	}{
		{"sessions focused", ViewSessions, true},
		{"messages not focused", ViewMessages, false},
		{"review not focused", ViewReview, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Model{activeView: tt.view}
			assert.Equal(t, tt.want, m.isSessionsFocused())
		})
	}
}

func TestIsCurrentTmuxSession(t *testing.T) {
	tests := []struct {
		name        string
		currentTmux string
		sess        session.Session
		want        bool
	}{
		{
			name:        "empty current returns false",
			currentTmux: "",
			sess:        session.Session{Slug: "my-session"},
			want:        false,
		},
		{
			name:        "exact slug match",
			currentTmux: "my-session",
			sess:        session.Session{Slug: "my-session"},
			want:        true,
		},
		{
			name:        "prefix with underscore",
			currentTmux: "my-session_123",
			sess:        session.Session{Slug: "my-session"},
			want:        true,
		},
		{
			name:        "prefix with dash",
			currentTmux: "my-session-extra",
			sess:        session.Session{Slug: "my-session"},
			want:        true,
		},
		{
			name:        "metadata tmux session match",
			currentTmux: "custom-name",
			sess: session.Session{
				Slug:     "other-slug",
				Metadata: map[string]string{session.MetaTmuxSession: "custom-name"},
			},
			want: true,
		},
		{
			name:        "no match",
			currentTmux: "different",
			sess:        session.Session{Slug: "my-session"},
			want:        false,
		},
		{
			name:        "partial slug without separator no match",
			currentTmux: "my-sessionextra",
			sess:        session.Session{Slug: "my-session"},
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Model{currentTmuxSession: tt.currentTmux}
			assert.Equal(t, tt.want, m.isCurrentTmuxSession(&tt.sess))
		})
	}
}
