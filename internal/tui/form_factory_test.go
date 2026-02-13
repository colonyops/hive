package tui

import (
	"testing"

	"github.com/hay-kot/hive/internal/core/config"
	"github.com/hay-kot/hive/internal/core/session"
	"github.com/hay-kot/hive/internal/core/terminal"
	"github.com/hay-kot/hive/pkg/kv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFormDialog(t *testing.T) {
	sessions := []session.Session{
		{ID: "s1", Name: "alpha", State: session.StateActive},
		{ID: "s2", Name: "beta", State: session.StateActive},
	}
	repos := []DiscoveredRepo{
		{Name: "hive", Path: "/tmp/hive", Remote: "git@github.com:org/hive.git"},
	}

	t.Run("text field", func(t *testing.T) {
		fields := []config.FormField{
			{Variable: "msg", Type: config.FormTypeText, Label: "Message", Placeholder: "enter text"},
		}
		d, err := newFormDialog("Test", fields, sessions, repos, nil)
		require.NoError(t, err)
		vals := d.FormValues()
		assert.Contains(t, vals, "msg")
		assert.Empty(t, vals["msg"])
	})

	t.Run("text field with default", func(t *testing.T) {
		fields := []config.FormField{
			{Variable: "msg", Type: config.FormTypeText, Label: "Message", Default: "hello"},
		}
		d, err := newFormDialog("Test", fields, sessions, repos, nil)
		require.NoError(t, err)
		vals := d.FormValues()
		assert.Equal(t, "hello", vals["msg"])
	})

	t.Run("textarea field", func(t *testing.T) {
		fields := []config.FormField{
			{Variable: "body", Type: config.FormTypeTextArea, Label: "Body"},
		}
		d, err := newFormDialog("Test", fields, sessions, repos, nil)
		require.NoError(t, err)
		vals := d.FormValues()
		assert.Contains(t, vals, "body")
	})

	t.Run("select field", func(t *testing.T) {
		fields := []config.FormField{
			{Variable: "env", Type: config.FormTypeSelect, Label: "Env", Options: []string{"dev", "prod"}},
		}
		d, err := newFormDialog("Test", fields, sessions, repos, nil)
		require.NoError(t, err)
		vals := d.FormValues()
		// First option is selected by default
		assert.Equal(t, "dev", vals["env"])
	})

	t.Run("multi-select field", func(t *testing.T) {
		fields := []config.FormField{
			{Variable: "tags", Type: config.FormTypeMultiSelect, Label: "Tags", Options: []string{"a", "b", "c"}},
		}
		d, err := newFormDialog("Test", fields, sessions, repos, nil)
		require.NoError(t, err)
		vals := d.FormValues()
		selected, ok := vals["tags"].([]string)
		require.True(t, ok)
		assert.Empty(t, selected)
	})

	t.Run("session selector single", func(t *testing.T) {
		fields := []config.FormField{
			{Variable: "target", Preset: config.FormPresetSessionSelector, Label: "Target"},
		}
		d, err := newFormDialog("Test", fields, sessions, repos, nil)
		require.NoError(t, err)
		vals := d.FormValues()
		sess, ok := vals["target"].(session.Session)
		require.True(t, ok, "expected session.Session, got %T", vals["target"])
		assert.Equal(t, "s1", sess.ID)
	})

	t.Run("session selector multi", func(t *testing.T) {
		fields := []config.FormField{
			{Variable: "targets", Preset: config.FormPresetSessionSelector, Multi: true, Label: "Targets"},
		}
		d, err := newFormDialog("Test", fields, sessions, repos, nil)
		require.NoError(t, err)
		vals := d.FormValues()
		sessList, ok := vals["targets"].([]session.Session)
		require.True(t, ok, "expected []session.Session, got %T", vals["targets"])
		assert.Empty(t, sessList)
	})

	t.Run("project selector single", func(t *testing.T) {
		fields := []config.FormField{
			{Variable: "repo", Preset: config.FormPresetProjectSelector, Label: "Repo"},
		}
		d, err := newFormDialog("Test", fields, sessions, repos, nil)
		require.NoError(t, err)
		vals := d.FormValues()
		assert.Contains(t, vals, "repo")
	})

	t.Run("project selector multi", func(t *testing.T) {
		fields := []config.FormField{
			{Variable: "repos", Preset: config.FormPresetProjectSelector, Multi: true, Label: "Repos"},
		}
		d, err := newFormDialog("Test", fields, sessions, repos, nil)
		require.NoError(t, err)
		vals := d.FormValues()
		assert.Contains(t, vals, "repos")
	})

	t.Run("multiple fields", func(t *testing.T) {
		fields := []config.FormField{
			{Variable: "targets", Preset: config.FormPresetSessionSelector, Multi: true, Label: "Recipients"},
			{Variable: "message", Type: config.FormTypeText, Label: "Message", Placeholder: "Type here..."},
		}
		d, err := newFormDialog("SendBatch", fields, sessions, repos, nil)
		require.NoError(t, err)
		vals := d.FormValues()
		assert.Contains(t, vals, "targets")
		assert.Contains(t, vals, "message")
	})

	t.Run("unknown type returns error", func(t *testing.T) {
		fields := []config.FormField{
			{Variable: "x", Type: "unknown", Label: "X"},
		}
		_, err := newFormDialog("Test", fields, sessions, repos, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown form field type/preset")
	})

	t.Run("session selector filters active by default", func(t *testing.T) {
		mixed := []session.Session{
			{ID: "s1", Name: "alpha", State: session.StateActive},
			{ID: "s2", Name: "beta", State: session.StateRecycled},
			{ID: "s3", Name: "gamma", State: session.StateActive},
		}
		fields := []config.FormField{
			{Variable: "targets", Preset: config.FormPresetSessionSelector, Multi: true, Label: "Targets"},
		}
		d, err := newFormDialog("Test", fields, mixed, repos, nil)
		require.NoError(t, err)
		vals := d.FormValues()
		_, ok := vals["targets"].([]session.Session)
		require.True(t, ok)
	})

	t.Run("session selector filters by terminal status", func(t *testing.T) {
		mixed := []session.Session{
			{ID: "s1", Name: "alpha", State: session.StateActive},
			{ID: "s2", Name: "beta", State: session.StateActive},
			{ID: "s3", Name: "gamma", State: session.StateActive},
		}
		ts := kv.New[string, TerminalStatus]()
		ts.Set("s1", TerminalStatus{Status: terminal.StatusActive})
		ts.Set("s2", TerminalStatus{Status: terminal.StatusMissing})
		ts.Set("s3", TerminalStatus{Status: terminal.StatusReady})

		fields := []config.FormField{
			{Variable: "target", Preset: config.FormPresetSessionSelector, Label: "Target"},
		}
		d, err := newFormDialog("Test", fields, mixed, repos, ts)
		require.NoError(t, err)
		// s2 is missing, so only s1 (alpha) and s3 (gamma) should be present.
		// First selected item should be s1.
		vals := d.FormValues()
		sess, ok := vals["target"].(session.Session)
		require.True(t, ok)
		assert.Equal(t, "s1", sess.ID)
	})

	t.Run("session selector filter all includes all states", func(t *testing.T) {
		mixed := []session.Session{
			{ID: "s1", Name: "alpha", State: session.StateActive},
			{ID: "s2", Name: "beta", State: session.StateRecycled},
		}
		fields := []config.FormField{
			{Variable: "target", Preset: config.FormPresetSessionSelector, Label: "Target", Filter: config.FormFilterAll},
		}
		d, err := newFormDialog("Test", fields, mixed, repos, nil)
		require.NoError(t, err)
		vals := d.FormValues()
		// With filter=all, first item is "alpha" (all sessions included)
		sess, ok := vals["target"].(session.Session)
		require.True(t, ok)
		assert.Equal(t, "s1", sess.ID)
	})

	t.Run("empty fields", func(t *testing.T) {
		d, err := newFormDialog("Test", []config.FormField{}, sessions, repos, nil)
		require.NoError(t, err)
		assert.Empty(t, d.FormValues())
	})
}
