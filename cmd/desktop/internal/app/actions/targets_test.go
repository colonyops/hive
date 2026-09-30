package actions

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every action written before terminal mode existed declares no targets, and
// must keep meaning exactly what it meant then.
func TestHasTarget_NoDeclarationIsAnItemAction(t *testing.T) {
	a := Action{ID: "x", Type: "shell", Config: &ShellConfig{CommandTemplate: "true"}}
	assert.True(t, a.HasTarget(TargetItem))
	assert.False(t, a.HasTarget(TargetSession))
	assert.False(t, a.HasTarget(TargetWindow))
	assert.False(t, a.TargetsTerminal())
}

func TestHasTarget_DeclaringTerminalDropsTheItemSurface(t *testing.T) {
	a, err := decodeAction(t, `id: open
label: Open
type: shell
targets: [session]
command_template: "true"
`)
	require.NoError(t, err)
	assert.False(t, a.HasTarget(TargetItem))
	assert.True(t, a.HasTarget(TargetSession))
	assert.True(t, a.TargetsTerminal())
}

func TestDecode_TargetsAreCaseInsensitiveAndItemOnlyCollapses(t *testing.T) {
	a, err := decodeAction(t, `id: open
label: Open
type: shell
targets: [" Session ", WINDOW]
command_template: "true"
`)
	require.NoError(t, err)
	assert.Equal(t, []string{TargetSession, TargetWindow}, a.Targets)

	// `targets: [item]` is the default spelled out, so it collapses and never
	// reaches the YAML writer as a redundant key.
	itemOnly, err := decodeAction(t, `id: open
label: Open
type: shell
targets: [item]
command_template: "true"
`)
	require.NoError(t, err)
	assert.Nil(t, itemOnly.Targets)
	assert.True(t, itemOnly.HasTarget(TargetItem))
}

func TestValidateActions_RejectsAnUnknownTarget(t *testing.T) {
	a, err := decodeAction(t, `id: open
label: Open
type: shell
targets: [pane]
command_template: "true"
`)
	require.NoError(t, err)
	err = validateActions([]Action{a})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown target")
}

func TestValidateActions_RejectsADuplicateTarget(t *testing.T) {
	a, err := decodeAction(t, `id: open
label: Open
type: shell
targets: [session, session]
command_template: "true"
`)
	require.NoError(t, err)
	require.ErrorContains(t, validateActions([]Action{a}), "duplicate target")
}

// The refusal lands when the catalog is parsed rather than when the menu entry
// is clicked, so a launch-session action that cannot serve a terminal target
// never appears in one.
func TestValidateActions_RejectsALaunchSessionTerminalTarget(t *testing.T) {
	for _, target := range []string{TargetSession, TargetWindow} {
		t.Run(target, func(t *testing.T) {
			a, err := decodeAction(t, `id: spawn
label: Spawn
type: launch-session
targets: [`+target+`]
prompt_template: "go"
`)
			require.NoError(t, err)
			require.ErrorContains(t, validateActions([]Action{a}), "cannot run against a terminal")
		})
	}
}

func TestTerminalCapable(t *testing.T) {
	assert.False(t, Action{Config: &LaunchSessionConfig{}}.TerminalCapable())
	assert.True(t, Action{Config: &ShellConfig{}}.TerminalCapable())
	assert.True(t, Action{Config: &ClipboardConfig{}}.TerminalCapable())
	assert.True(t, Action{Config: &PublishMessageConfig{}}.TerminalCapable())
}

func TestEditableCatalog_SpellsOutTheDefaultTarget(t *testing.T) {
	e, err := editableFromAction(Action{ID: "x", Label: "X", Type: "shell", Config: &ShellConfig{CommandTemplate: "true"}})
	require.NoError(t, err)
	assert.Equal(t, []string{TargetItem}, e.Targets)

	// …and the editor sending that default back collapses again, so saving an
	// untouched action does not add a `targets:` key to actions.yml.
	a, err := actionFromEditable(e)
	require.NoError(t, err)
	assert.Nil(t, a.Targets)
}

func TestYAMLWriter_RoundTripsTerminalTargets(t *testing.T) {
	store := NewActionStore(filepath.Join(t.TempDir(), "actions.yml"))
	original := Action{
		ID: "open", Label: "Open", Type: "shell",
		Targets: []string{TargetSession, TargetWindow},
		Config:  &ShellConfig{CommandTemplate: "true"},
	}
	store.mu.Lock()
	_, err := store.mutateLocked("create", original.ID, original)
	store.mu.Unlock()
	require.NoError(t, err)

	reloaded, ok := store.Get("open")
	require.True(t, ok)
	assert.Equal(t, []string{TargetSession, TargetWindow}, reloaded.Targets)
}

func TestEditableCatalog_RoundTripsTerminalTargets(t *testing.T) {
	e, err := editableFromAction(Action{
		ID: "x", Label: "X", Type: "shell",
		Targets: []string{TargetSession, TargetWindow},
		Config:  &ShellConfig{CommandTemplate: "true"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{TargetSession, TargetWindow}, e.Targets)

	a, err := actionFromEditable(e)
	require.NoError(t, err)
	assert.Equal(t, []string{TargetSession, TargetWindow}, a.Targets)
}
