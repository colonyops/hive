package actions

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const launcherYAML = `version: 1
actions:
  - id: run-tests
    label: Run tests
    type: shell
    targets: [session]
    command_template: 'mise run test'
launchers:
  - id: lazygit
    label: lazygit
    icon: git-branch
    command: lazygit
  - id: dotfiles
    label: Edit dotfiles
    cwd: ~/.dotfiles
    command: $EDITOR .
`

func TestLaunchers_ParseBesideTheActionsInOneFile(t *testing.T) {
	got, err := parseCatalog([]byte(launcherYAML))
	require.NoError(t, err)

	assert.Len(t, got.Actions, 1)
	assert.Equal(t, []Launcher{
		{ID: "lazygit", Label: "lazygit", Command: "lazygit", Icon: "git-branch"},
		{ID: "dotfiles", Label: "Edit dotfiles", Command: "$EDITOR .", Cwd: "~/.dotfiles"},
	}, got.Launchers)
}

// Launcher ids and action ids are separate namespaces: nothing resolves a
// launcher against the action catalog, so sharing an id is unambiguous.
func TestLaunchers_MayShareAnIDWithAnAction(t *testing.T) {
	_, err := parseCatalog([]byte(`version: 1
actions:
  - id: lazygit
    label: Open lazygit output
    type: shell
    command_template: 'lazygit log'
launchers:
  - id: lazygit
    label: lazygit
    command: lazygit
`))
	require.NoError(t, err)
}

func TestLaunchers_RejectMalformedEntries(t *testing.T) {
	for name, body := range map[string]string{
		"no id":          "  - label: lazygit\n    command: lazygit\n",
		"bad slug":       "  - id: Lazy_Git\n    label: lazygit\n    command: lazygit\n",
		"no label":       "  - id: lazygit\n    command: lazygit\n",
		"no command":     "  - id: lazygit\n    label: lazygit\n",
		"unknown icon":   "  - id: lazygit\n    label: lazygit\n    icon: nope\n    command: lazygit\n",
		"duplicate id":   "  - id: lazygit\n    label: lazygit\n    command: lazygit\n  - id: lazygit\n    label: Again\n    command: lazygit\n",
		"unknown field":  "  - id: lazygit\n    label: lazygit\n    command: lazygit\n    targets: [session]\n",
		"action's field": "  - id: lazygit\n    label: lazygit\n    command_template: lazygit\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseCatalog([]byte("version: 1\nlaunchers:\n" + body))
			require.Error(t, err)
		})
	}
}

// The store owns the whole file, so a launcher written through it must land in
// its own list and leave the actions — and their comments — untouched.
func TestActionStore_LauncherCRUDLeavesTheActionsAlone(t *testing.T) {
	path := writeActionsFile(t, t.TempDir(), "actions.yml", `version: 1
# a comment a hand edit put here
actions:
  - id: run-tests
    label: Run tests
    type: shell
    command_template: 'mise run test'
`)
	s := NewActionStore(path)
	require.NoError(t, s.Reload())

	created, err := s.CreateLauncher(Launcher{ID: "lazygit", Label: "lazygit", Command: "lazygit", Icon: "git-branch"})
	require.NoError(t, err)
	assert.Equal(t, "lazygit", created.ID)
	assert.Len(t, s.List(), 1, "writing a launcher must not disturb the action catalog")
	assert.Contains(t, readActionsFile(t, path), "a comment a hand edit put here")

	_, err = s.CreateLauncher(Launcher{ID: "lazygit", Label: "again", Command: "lazygit"})
	require.ErrorContains(t, err, "already exists")

	_, err = s.UpdateLauncher("lazygit", Launcher{ID: "lazygit", Label: "Git", Command: "lazygit", Cwd: "~/src"})
	require.NoError(t, err)
	updated, ok := s.Launcher("lazygit")
	require.True(t, ok)
	assert.Equal(t, Launcher{ID: "lazygit", Label: "Git", Command: "lazygit", Cwd: "~/src"}, updated)

	_, err = s.UpdateLauncher("lazygit", Launcher{ID: "renamed", Label: "Git", Command: "lazygit"})
	require.ErrorContains(t, err, "immutable")

	require.NoError(t, s.DeleteLauncher("lazygit"))
	assert.Empty(t, s.Launchers())
	require.ErrorContains(t, s.DeleteLauncher("lazygit"), "not found")
	assert.Len(t, s.List(), 1)
}

// An invalid launcher must never reach disk, the same guarantee action CRUD
// gives: the store validates the candidate before it writes.
func TestActionStore_RefusesAnInvalidLauncher(t *testing.T) {
	path := filepath.Join(t.TempDir(), "actions.yml")
	s := NewActionStore(path)

	_, err := s.CreateLauncher(Launcher{ID: "lazygit", Label: "lazygit"})
	require.ErrorContains(t, err, "command is required")
	assert.NoFileExists(t, path)
}

// A file with no launchers list gains one on the first write, and a file with
// no actions list is still a valid place to put a launcher.
func TestActionStore_CreatesTheLauncherListOnDemand(t *testing.T) {
	path := filepath.Join(t.TempDir(), "actions.yml")
	s := NewActionStore(path)

	_, err := s.CreateLauncher(Launcher{ID: "lazygit", Label: "lazygit", Command: "lazygit"})
	require.NoError(t, err)

	reloaded, err := LoadCatalog(path)
	require.NoError(t, err)
	assert.Equal(t, []Launcher{{ID: "lazygit", Label: "lazygit", Command: "lazygit"}}, reloaded.Launchers)
	assert.Empty(t, reloaded.Actions)
}

func readActionsFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}
