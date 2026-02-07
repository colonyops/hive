package tui

import (
	"strings"
	"testing"

	"github.com/hay-kot/hive/internal/core/session"
)

func TestMaybeOverrideWindowDelete(t *testing.T) {
	m := Model{}

	deleteAction := Action{
		Type:      ActionTypeDelete,
		SessionID: "sess-1",
	}
	shellAction := Action{
		Type:      ActionTypeShell,
		SessionID: "sess-1",
		ShellCmd:  "echo hello",
	}

	t.Run("nil treeItem returns action unchanged", func(t *testing.T) {
		got := m.maybeOverrideWindowDelete(deleteAction, nil)
		if got.Type != ActionTypeDelete {
			t.Errorf("Type = %v, want %v", got.Type, ActionTypeDelete)
		}
	})

	t.Run("non-window item returns action unchanged", func(t *testing.T) {
		ti := &TreeItem{IsWindowItem: false}
		got := m.maybeOverrideWindowDelete(deleteAction, ti)
		if got.Type != ActionTypeDelete {
			t.Errorf("Type = %v, want %v", got.Type, ActionTypeDelete)
		}
	})

	t.Run("non-delete action on window returns action unchanged", func(t *testing.T) {
		ti := &TreeItem{
			IsWindowItem:  true,
			WindowIndex:   "1",
			WindowName:    "claude",
			ParentSession: session.Session{Slug: "my-slug"},
		}
		got := m.maybeOverrideWindowDelete(shellAction, ti)
		if got.Type != ActionTypeShell {
			t.Errorf("Type = %v, want %v", got.Type, ActionTypeShell)
		}
	})

	t.Run("delete on window converts to tmux kill-window shell command", func(t *testing.T) {
		ti := &TreeItem{
			IsWindowItem:  true,
			WindowIndex:   "2",
			WindowName:    "aider",
			ParentSession: session.Session{Slug: "my-slug"},
		}
		got := m.maybeOverrideWindowDelete(deleteAction, ti)
		if got.Type != ActionTypeShell {
			t.Errorf("Type = %v, want %v", got.Type, ActionTypeShell)
		}
		if !strings.Contains(got.ShellCmd, "tmux kill-window") {
			t.Errorf("ShellCmd = %q, expected to contain 'tmux kill-window'", got.ShellCmd)
		}
		if !strings.Contains(got.ShellCmd, "my-slug:2") {
			t.Errorf("ShellCmd = %q, expected to contain target 'my-slug:2'", got.ShellCmd)
		}
		if !strings.Contains(got.Confirm, "aider") {
			t.Errorf("Confirm = %q, expected to contain window name 'aider'", got.Confirm)
		}
	})

	t.Run("uses MetaTmuxSession when available", func(t *testing.T) {
		ti := &TreeItem{
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
		got := m.maybeOverrideWindowDelete(deleteAction, ti)
		if !strings.Contains(got.ShellCmd, "explicit-sess:1") {
			t.Errorf("ShellCmd = %q, expected target 'explicit-sess:1'", got.ShellCmd)
		}
	})

	t.Run("falls back to Name when Slug empty", func(t *testing.T) {
		ti := &TreeItem{
			IsWindowItem:  true,
			WindowIndex:   "0",
			WindowName:    "bash",
			ParentSession: session.Session{Name: "my-name"},
		}
		got := m.maybeOverrideWindowDelete(deleteAction, ti)
		if !strings.Contains(got.ShellCmd, "my-name:0") {
			t.Errorf("ShellCmd = %q, expected target 'my-name:0'", got.ShellCmd)
		}
	})

	t.Run("errors when session and window index are empty", func(t *testing.T) {
		ti := &TreeItem{
			IsWindowItem:  true,
			WindowIndex:   "",
			ParentSession: session.Session{},
		}
		got := m.maybeOverrideWindowDelete(deleteAction, ti)
		if got.Err == nil {
			t.Error("expected Err to be non-nil when session and window index are empty")
		}
	})

	t.Run("no window name uses generic confirm message", func(t *testing.T) {
		ti := &TreeItem{
			IsWindowItem:  true,
			WindowIndex:   "0",
			WindowName:    "",
			ParentSession: session.Session{Slug: "my-slug"},
		}
		got := m.maybeOverrideWindowDelete(deleteAction, ti)
		if got.Confirm != "Kill tmux window?" {
			t.Errorf("Confirm = %q, want %q", got.Confirm, "Kill tmux window?")
		}
	})
}
