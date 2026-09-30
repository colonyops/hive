package actions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func launchEditable(id string) EditableAction {
	return EditableAction{ID: id, Label: id, Type: "launch-session", ShowInDetail: true, AppliesTo: []string{"pr"}, Launch: &EditableLaunchConfig{PromptTemplate: "Review"}}
}

func TestActionStoreEditableCRUDPreservesUnrelatedCommentsAndRejectsBadLatestDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "actions.yml")
	original := "# catalog header\nversion: 1\nactions:\n  # keep this comment\n  - id: existing\n    label: Existing\n    type: shell\n    command_template: true\n"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o600))
	s := NewActionStore(path)
	created, err := s.Create(launchEditable("new-action"))
	require.NoError(t, err)
	assert.Equal(t, "new-action", created.ID)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(got), "# catalog header")
	assert.Less(t, strings.Index(string(got), "id: existing"), strings.Index(string(got), "id: new-action"))

	_, err = s.Create(launchEditable("new-action"))
	require.Error(t, err)
	changed := launchEditable("renamed")
	_, err = s.Update(t.Context(), "new-action", changed)
	require.Error(t, err)

	before, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, []byte("version: nope"), 0o600))
	_, err = s.Create(launchEditable("later"))
	require.Error(t, err)
	assert.Equal(t, []byte("version: nope"), mustRead(t, path))
	// Repairing disk permits mutation again; no stale candidate is retained.
	require.NoError(t, os.WriteFile(path, before, 0o600))
	_, err = s.Update(t.Context(), "new-action", launchEditable("new-action"))
	require.NoError(t, err)
}

func TestActionStoreListPreservesDetailFlagAndAppliesToIsCaseInsensitive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "actions.yml")
	require.NoError(t, os.WriteFile(path, []byte("version: 1\nactions:\n  - id: visible\n    label: Visible\n    type: shell\n    show_in_detail: true\n    applies_to: [PR]\n    command_template: true\n  - id: hidden\n    label: Hidden\n    type: shell\n    applies_to: [pr]\n    command_template: true\n"), 0o600))
	s := NewActionStore(path)
	require.Len(t, s.List(), 2)

	visible, ok := s.Get("visible")
	require.True(t, ok)
	assert.True(t, visible.ShowInDetail)
	assert.True(t, AppliesTo(visible, "pr"), "applies_to matching is case-insensitive")
	assert.False(t, AppliesTo(visible, "issue"))

	hidden, ok := s.Get("hidden")
	require.True(t, ok)
	assert.False(t, hidden.ShowInDetail)
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	return got
}
