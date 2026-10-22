package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/hive/internal/config"
	"github.com/colonyops/hive/cmd/hive/internal/plugins"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/hive/events/testbus"
	"github.com/colonyops/hive/internal/store/db"
	"github.com/colonyops/hive/pkg/executil/executiltest"
)

type reloadMux struct{ Multiplexer }

func TestReloadConfigReplacesServicesAndCommandsOnlyOnSuccess(t *testing.T) {
	t.Setenv("HIVE_DEFAULT_AGENT", "")
	dir := t.TempDir()
	cfg, err := config.Load("", dir)
	require.NoError(t, err)
	database, err := db.Open(dir, db.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	engine, err := hive.New(&cfg.Config, hive.Ports{
		DB: database, Bus: testbus.New(t).EventBus, Executor: &executiltest.Exec{}, Mux: reloadMux{}, DataDir: dir,
	})
	require.NoError(t, err)
	commands := plugins.NewCommandSet(config.DefaultUserCommands(), map[string]config.UserCommand{"old": {Sh: "echo old"}})
	commands.SetPlugin("test", map[string]config.UserCommand{"plugin": {Sh: "echo plugin"}})
	app := NewApp(zerolog.Nop(), engine, cfg, nil, nil, commands, nil)
	original := app.Sessions()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("database:\n  max_open_conns: 4"), 0o600))
	require.ErrorContains(t, app.ReloadConfig(path), "restart required")
	assert.Same(t, original, app.Sessions())
	assert.Same(t, cfg, app.Config)
	_, ok := commands.Lookup("old")
	assert.True(t, ok)
	require.NoError(t, os.WriteFile(path, []byte("usercommands:\n  fresh: echo fresh\nrules:\n  - pattern: acme\n    max_recycled: 7"), 0o600))
	require.NoError(t, app.ReloadConfig(path))
	assert.NotSame(t, original, app.Sessions())
	assert.Same(t, &app.Config.Config, app.Engine.Config())
	assert.Equal(t, 7, app.Config.GetMaxRecycled("acme/widgets"))
	_, ok = commands.Lookup("old")
	assert.False(t, ok)
	_, ok = commands.Lookup("fresh")
	assert.True(t, ok)
	_, ok = commands.Lookup("plugin")
	assert.True(t, ok)
	assert.NotNil(t, app.Doctor)
	assert.NotNil(t, app.Sources)
}
