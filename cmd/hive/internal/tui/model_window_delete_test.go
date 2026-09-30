package tui

import (
	"testing"

	"github.com/colonyops/hive/cmd/hive/internal/tui/views/sessions"
	"github.com/colonyops/hive/internal/core/action"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/stretchr/testify/assert"
)

func TestMaybeOverrideWindowDelete(t *testing.T) {
	deleteAction := Action{
		Type:      action.TypeDelete,
		SessionID: "sess-1",
	}
	shellAction := Action{
		Type:      action.TypeShell,
		SessionID: "sess-1",
		ShellCmd:  "echo hello",
	}

	t.Run("nil treeItem returns action unchanged", func(t *testing.T) {
		got := sessions.MaybeOverrideWindowDelete(deleteAction, nil)
		assert.Equal(t, action.TypeDelete, got.Type)
	})

	t.Run("non-window item returns action unchanged", func(t *testing.T) {
		ti := &sessions.TreeItem{IsWindowItem: false}
		got := sessions.MaybeOverrideWindowDelete(deleteAction, ti)
		assert.Equal(t, action.TypeDelete, got.Type)
	})

	t.Run("non-delete action on window returns action unchanged", func(t *testing.T) {
		ti := &sessions.TreeItem{
			IsWindowItem:  true,
			WindowIndex:   "1",
			WindowName:    "claude",
			ParentSession: session.Session{Slug: "my-slug"},
		}
		got := sessions.MaybeOverrideWindowDelete(shellAction, ti)
		assert.Equal(t, action.TypeShell, got.Type)
	})

	t.Run("delete on window creates a typed kill request", func(t *testing.T) {
		ti := &sessions.TreeItem{
			IsWindowItem:  true,
			WindowIndex:   "2",
			WindowName:    "aider",
			ParentSession: session.Session{Slug: "my-slug"},
		}
		got := sessions.MaybeOverrideWindowDelete(deleteAction, ti)
		assert.Equal(t, action.TypeKillWindow, got.Type)
		assert.NotNil(t, got.WindowTarget)
		assert.Equal(t, "my-slug", got.WindowTarget.Session)
		assert.Equal(t, "2", got.WindowTarget.Window)
		assert.Contains(t, got.Confirm, "aider")
	})

	t.Run("uses MetaTmuxSession when available", func(t *testing.T) {
		ti := &sessions.TreeItem{
			IsWindowItem: true,
			WindowIndex:  "1",
			WindowName:   "claude",
			ParentSession: session.Session{
				Slug: "my-slug",
				Metadata: map[string]string{
					session.MetaTmuxSession: "explicit-sess",
				},
			},
		}
		got := sessions.MaybeOverrideWindowDelete(deleteAction, ti)
		assert.Equal(t, "explicit-sess", got.WindowTarget.Session)
		assert.Equal(t, "1", got.WindowTarget.Window)
	})

	t.Run("errors when session and window index are empty", func(t *testing.T) {
		ti := &sessions.TreeItem{
			IsWindowItem:  true,
			WindowIndex:   "",
			ParentSession: session.Session{},
		}
		got := sessions.MaybeOverrideWindowDelete(deleteAction, ti)
		assert.Error(t, got.Err, "expected Err to be non-nil when session and window index are empty")
	})

	t.Run("no window name uses generic confirm message", func(t *testing.T) {
		ti := &sessions.TreeItem{
			IsWindowItem:  true,
			WindowIndex:   "0",
			WindowName:    "",
			ParentSession: session.Session{Slug: "my-slug"},
		}
		got := sessions.MaybeOverrideWindowDelete(deleteAction, ti)
		assert.Equal(t, "Kill tmux window?", got.Confirm)
	})
}
