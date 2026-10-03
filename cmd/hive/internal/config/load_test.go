package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/hive/internal/action"
	"github.com/colonyops/hive/cmd/hive/internal/theme"
)

func TestLoadMergesDefaultsUnderUserKeybindingsAndCommands(t *testing.T) {
	t.Setenv("HIVE_DEFAULT_AGENT", "")
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`usercommands:
  open: open {{ .Path }}
  Recycle:
    action: recycle
    help: my recycle
views:
  sessions:
    keybindings:
      o: {cmd: open}
      r: {cmd: Delete}
rules:
  - pattern: acme/.*
    max_recycled: 1
`), 0o644))

	cfg, err := Load(path, t.TempDir())
	require.NoError(t, err)

	commands := cfg.MergedUserCommands()
	assert.Equal(t, "open {{ .Path }}", commands["open"].Sh)
	assert.Equal(t, "my recycle", commands["Recycle"].Help, "a user command overrides the default of the same name")
	assert.Equal(t, action.TypeDelete, commands["Delete"].Action, "defaults the user did not override stay")

	sessions := cfg.Views.Sessions.Keybindings
	assert.Equal(t, "open", sessions["o"].Cmd)
	assert.Equal(t, "Delete", sessions["r"].Cmd, "a user key overrides the default binding")
	assert.Equal(t, "NewSession", sessions["n"].Cmd, "default bindings stay")
	assert.Equal(t, "Quit", cfg.Keybindings["q"].Cmd, "the flat map includes global defaults")

	assert.Equal(t, theme.Default, cfg.TUI.Theme)
	assert.Equal(t, 1, cfg.GetMaxRecycled("acme/widgets"), "engine sections decode through the inline embed")
	assert.Equal(t, 2, cfg.Database.MaxOpenConns, "engine defaults apply")
}
