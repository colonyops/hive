package tui

import (
	"testing"

	"github.com/colonyops/hive/internal/core/action"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/tui/views/sessions"
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
		got := sessions.MaybeOverrideWindowDelete(deleteAction, nil, testRenderer)
		assert.Equal(t, action.TypeDelete, got.Type)
	})

	t.Run("non-window item returns action unchanged", func(t *testing.T) {
		ti := &sessions.TreeItem{IsWindowItem: false}
		got := sessions.MaybeOverrideWindowDelete(deleteAction, ti, testRenderer)
		assert.Equal(t, action.TypeDelete, got.Type)
	})

	t.Run("non-delete action on window returns action unchanged", func(t *testing.T) {
		ti := &sessions.TreeItem{
			IsWindowItem:  true,
			WindowIndex:   "1",
			WindowName:    "claude",
			ParentSession: session.Session{Slug: "my-slug"},
		}
		got := sessions.MaybeOverrideWindowDelete(shellAction, ti, testRenderer)
		assert.Equal(t, action.TypeShell, got.Type)
	})

	t.Run("delete on window converts to tmux kill-window shell command", func(t *testing.T) {
		ti := &sessions.TreeItem{
			IsWindowItem:  true,
			WindowIndex:   "2",
			WindowName:    "aider",
			ParentSession: session.Session{Slug: "my-slug"},
		}
		got := sessions.MaybeOverrideWindowDelete(deleteAction, ti, testRenderer)
		assert.Equal(t, action.TypeShell, got.Type)
		assert.Contains(t, got.ShellCmd, "tmux kill-window")
		assert.Contains(t, got.ShellCmd, "my-slug:2")
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
		got := sessions.MaybeOverrideWindowDelete(deleteAction, ti, testRenderer)
		assert.Contains(t, got.ShellCmd, "explicit-sess:1")
	})

	t.Run("falls back to Name when Slug empty", func(t *testing.T) {
		ti := &sessions.TreeItem{
			IsWindowItem:  true,
			WindowIndex:   "0",
			WindowName:    "bash",
			ParentSession: session.Session{Name: "my-name"},
		}
		got := sessions.MaybeOverrideWindowDelete(deleteAction, ti, testRenderer)
		assert.Contains(t, got.ShellCmd, "my-name:0")
	})

	t.Run("errors when session and window index are empty", func(t *testing.T) {
		ti := &sessions.TreeItem{
			IsWindowItem:  true,
			WindowIndex:   "",
			ParentSession: session.Session{},
		}
		got := sessions.MaybeOverrideWindowDelete(deleteAction, ti, testRenderer)
		assert.Error(t, got.Err, "expected Err to be non-nil when session and window index are empty")
	})

	t.Run("no window name uses generic confirm message", func(t *testing.T) {
		ti := &sessions.TreeItem{
			IsWindowItem:  true,
			WindowIndex:   "0",
			WindowName:    "",
			ParentSession: session.Session{Slug: "my-slug"},
		}
		got := sessions.MaybeOverrideWindowDelete(deleteAction, ti, testRenderer)
		assert.Equal(t, "Kill tmux window?", got.Confirm)
	})
}
